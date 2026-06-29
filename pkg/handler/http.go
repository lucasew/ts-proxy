package handler

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/lucasew/ts-proxy/pkg/tsproxy"
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
			tsproxy.ReportError("http server shutdown", err)
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
			tsproxy.ReportError("http whois error", err)
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
	if r.URL.Hostname() != "" && r.URL.Hostname() != h.opts.Hostname {
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
	r.Header.Del(HeaderXForwardedProto)
	if h.opts.EnableTLS {
		r.Header.Set(HeaderXForwardedProto, SchemeHTTPS)
	} else {
		r.Header.Set(HeaderXForwardedProto, SchemeHTTP)
	}
	r.Header.Del(HeaderXForwardedHost)
	r.Header.Set(HeaderXForwardedHost, h.opts.Hostname)
	slog.Info("request",
		"method", r.Method,
		"user", userInfo.UserProfile.LoginName,
		"host", r.Host,
		"url", r.URL.String(),
	)
	SetTailscaleHeaders(r, userInfo)
}

// SetTailscaleHeaders sanitizes and sets Tailscale user identity headers.
func SetTailscaleHeaders(r *http.Request, userInfo *apitype.WhoIsResponse) {
	for k := range r.Header {
		normalized := strings.ReplaceAll(k, "_", "-")
		if strings.EqualFold(normalized, TailscaleUserLoginHeader) ||
			strings.EqualFold(normalized, TailscaleUserNameHeader) ||
			strings.EqualFold(normalized, TailscaleUserProfilePicHeader) ||
			strings.EqualFold(normalized, TailscaleHeadersInfoHeader) {
			delete(r.Header, k)
		}
	}
	r.Header.Set(TailscaleUserLoginHeader, userInfo.UserProfile.LoginName)
	r.Header.Set(TailscaleUserNameHeader, userInfo.UserProfile.DisplayName)
	r.Header.Set(TailscaleUserProfilePicHeader, userInfo.UserProfile.ProfilePicURL)
	r.Header.Set(TailscaleHeadersInfoHeader, "https://tailscale.com/s/serve-headers")
}
