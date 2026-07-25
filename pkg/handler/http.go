package handler

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/lucasew/ts-proxy/pkg/tsproxy"
	"tailscale.com/client/local"
	"tailscale.com/client/tailscale/apitype"
)

const (
	TailscaleUserLoginHeader      = "Tailscale-User-Login"
	TailscaleUserNameHeader       = "Tailscale-User-Name"
	TailscaleUserProfilePicHeader = "Tailscale-User-Profile-Pic"
	TailscaleHeadersInfoHeader    = "Tailscale-Headers-Info"

	// taggedDevicesLogin is the LoginName WhoIs returns for tagged nodes.
	// Official Tailscale serve omits identity headers for tagged devices.
	taggedDevicesLogin = "tagged-devices"

	HeaderXForwardedProto = "X-Forwarded-Proto"
	HeaderXForwardedHost  = "X-Forwarded-Host"
	HeaderXForwardedFor   = "X-Forwarded-For"

	SchemeHTTP  = "http"
	SchemeHTTPS = "https"
)

// HTTPOptions configures an HTTP reverse proxy handler.
type HTTPOptions struct {
	Hostname        string
	EnableTLS       bool
	UpstreamAddress string
	UpstreamNetwork string
	WhoIs           WhoIsFunc
}

// HTTPHandler is an HTTP reverse proxy that enriches requests with Tailscale user headers.
type HTTPHandler struct {
	opts  HTTPOptions
	proxy *httputil.ReverseProxy
}

// NewHTTP creates an HTTP reverse proxy handler.
func NewHTTP(opts HTTPOptions) *HTTPHandler {
	if opts.UpstreamNetwork == "" {
		opts.UpstreamNetwork = "tcp"
	}
	u := &url.URL{
		Scheme: SchemeHTTP,
		Host:   opts.Hostname,
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.Transport = &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, opts.UpstreamNetwork, opts.UpstreamAddress)
		},
	}
	return &HTTPHandler{
		opts:  opts,
		proxy: proxy,
	}
}

func (h *HTTPHandler) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			tsproxy.ReportError(err, "context", "http server shutdown")
		}
	}()

	err := srv.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.opts.WhoIs != nil {
		userInfo, err := h.opts.WhoIs(r.Context(), r.RemoteAddr)
		if err != nil {
			// Public Funnel (and other non-tailnet) clients have no peer
			// identity. Match Tailscale serve: continue without identity
			// headers instead of failing the request.
			if !errors.Is(err, local.ErrPeerNotFound) {
				tsproxy.ReportError(err, "context", "http whois error")
				http.Error(w, "whois failed", http.StatusInternalServerError)
				return
			}
			userInfo = nil
		}
		if h.handleRedirect(w, r) {
			return
		}
		h.enrichHeaders(r, userInfo)
	}
	h.proxy.ServeHTTP(w, r)
}

func (h *HTTPHandler) handleRedirect(w http.ResponseWriter, r *http.Request) bool {
	// Server requests put the host in Request.Host; Request.URL.Host is empty.
	host := r.Host
	if hostname, _, err := net.SplitHostPort(host); err == nil {
		host = hostname
	}
	if host != "" && host != h.opts.Hostname {
		destinationURL := new(url.URL)
		*destinationURL = *r.URL
		destinationURL.Host = h.opts.Hostname
		if h.opts.EnableTLS {
			destinationURL.Scheme = SchemeHTTPS
		} else {
			destinationURL.Scheme = SchemeHTTP
		}
		slog.Info("redirect", "from", r.URL.String(), "to", destinationURL.String())
		http.Redirect(w, r, destinationURL.String(), http.StatusMovedPermanently)
		return true
	}
	return false
}

func (h *HTTPHandler) enrichHeaders(r *http.Request, userInfo *apitype.WhoIsResponse) {
	// Strip client-supplied X-Forwarded-* variants (including underscore
	// forms that http.Header.Del does not remove) before setting ours.
	// X-Forwarded-For is only stripped: httputil.ReverseProxy appends the
	// real client IP itself, so setting it here would duplicate the value.
	deleteHeaderVariants(r.Header, HeaderXForwardedProto, HeaderXForwardedHost, HeaderXForwardedFor)
	if h.opts.EnableTLS {
		r.Header.Set(HeaderXForwardedProto, SchemeHTTPS)
	} else {
		r.Header.Set(HeaderXForwardedProto, SchemeHTTP)
	}
	r.Header.Set(HeaderXForwardedHost, h.opts.Hostname)

	login := ""
	if userInfo != nil && userInfo.UserProfile != nil {
		login = userInfo.UserProfile.LoginName
	}
	slog.Info("request",
		"method", r.Method,
		"user", login,
		"host", r.Host,
		"url", r.URL.String(),
	)

	// Always strip client-supplied identity headers. Only set them when
	// WhoIs returned a real user (not funnel/public and not tagged devices).
	if hasTailscaleUserIdentity(userInfo) {
		SetTailscaleHeaders(r, userInfo)
	} else {
		deleteHeaderVariants(r.Header,
			TailscaleUserLoginHeader,
			TailscaleUserNameHeader,
			TailscaleUserProfilePicHeader,
			TailscaleHeadersInfoHeader,
		)
	}
}

// hasTailscaleUserIdentity reports whether WhoIs yielded a user identity we
// should forward. Matches Tailscale serve: no headers for missing peers,
// empty profiles, or tagged devices.
func hasTailscaleUserIdentity(userInfo *apitype.WhoIsResponse) bool {
	if userInfo == nil || userInfo.UserProfile == nil {
		return false
	}
	login := userInfo.UserProfile.LoginName
	return login != "" && login != taggedDevicesLogin
}

// deleteHeaderVariants removes every header key whose name matches any of
// names after normalizing "_" to "-" (case-insensitive). http.Header.Del only
// drops the canonical MIME key, so spoofed X_Forwarded_Proto-style keys would
// otherwise survive and confuse upstreams that treat "_" and "-" as equivalent.
func deleteHeaderVariants(h http.Header, names ...string) {
	for k := range h {
		normalized := strings.ReplaceAll(k, "_", "-")
		for _, name := range names {
			if strings.EqualFold(normalized, name) {
				delete(h, k)
				break
			}
		}
	}
}

// SetTailscaleHeaders sanitizes and sets Tailscale user identity headers.
// Caller must ensure userInfo has a non-nil UserProfile (see hasTailscaleUserIdentity).
func SetTailscaleHeaders(r *http.Request, userInfo *apitype.WhoIsResponse) {
	deleteHeaderVariants(r.Header,
		TailscaleUserLoginHeader,
		TailscaleUserNameHeader,
		TailscaleUserProfilePicHeader,
		TailscaleHeadersInfoHeader,
	)
	r.Header.Set(TailscaleUserLoginHeader, userInfo.UserProfile.LoginName)
	r.Header.Set(TailscaleUserNameHeader, userInfo.UserProfile.DisplayName)
	r.Header.Set(TailscaleUserProfilePicHeader, userInfo.UserProfile.ProfilePicURL)
	r.Header.Set(TailscaleHeadersInfoHeader, "https://tailscale.com/s/serve-headers")
}
