package handler

import (
	"context"
	"github.com/lucasew/ts-proxy/pkg/tsproxy"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"tailscale.com/client/tailscale/apitype"
)

const (
	TailscaleUserLoginHeader      = "Tailscale-User-Login"
	TailscaleUserNameHeader       = "Tailscale-User-Name"
	TailscaleUserProfilePicHeader = "Tailscale-User-Profile-Pic"
	TailscaleHeadersInfoHeader    = "Tailscale-Headers-Info"

	HeaderXForwardedProto = "X-Forwarded-Proto"
	HeaderXForwardedHost  = "X-Forwarded-Host"

	SchemeHTTP  = "http"
	SchemeHTTPS = "https"
)

// DefaultHTTPDialTimeout is how long the reverse proxy waits when dialing
// upstream before giving up. Unlimited dials can pin a request goroutine
// forever against a blackholed or slow peer (same class of bug as
// DefaultTCPDialTimeout).
const DefaultHTTPDialTimeout = 10 * time.Second

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
	opts        HTTPOptions
	proxy       *httputil.ReverseProxy
	dialTimeout time.Duration
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
	h := &HTTPHandler{
		opts:        opts,
		dialTimeout: DefaultHTTPDialTimeout,
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.Transport = &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			timeout := h.dialTimeout
			if timeout <= 0 {
				timeout = DefaultHTTPDialTimeout
			}
			d := net.Dialer{Timeout: timeout}
			return d.DialContext(ctx, opts.UpstreamNetwork, opts.UpstreamAddress)
		},
	}
	h.proxy = proxy
	return h
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
			tsproxy.ReportError(err, "context", "http whois error")
			w.WriteHeader(http.StatusInternalServerError)
			return
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
	deleteHeaderVariants(r.Header, HeaderXForwardedProto, HeaderXForwardedHost)
	if h.opts.EnableTLS {
		r.Header.Set(HeaderXForwardedProto, SchemeHTTPS)
	} else {
		r.Header.Set(HeaderXForwardedProto, SchemeHTTP)
	}
	r.Header.Set(HeaderXForwardedHost, h.opts.Hostname)
	slog.Info("request",
		"method", r.Method,
		"user", userInfo.UserProfile.LoginName,
		"host", r.Host,
		"url", r.URL.String(),
	)
	SetTailscaleHeaders(r, userInfo)
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
