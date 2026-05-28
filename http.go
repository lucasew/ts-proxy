package tsproxy

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"tailscale.com/client/tailscale/apitype"
	"time"
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

// TailscaleHTTPProxyServer orchestrates HTTP-level proxying.
// It wraps a TailscaleProxyServer to intercept requests, inject
// identity information obtained from Tailscale WhoIs into HTTP headers,
// and forward requests to the upstream destination.
type TailscaleHTTPProxyServer struct {
	server *TailscaleProxyServer
	proxy  *httputil.ReverseProxy
}

// NewTailscaleHTTPProxyServer initializes a reverse proxy that uses
// the TailscaleProxyServer's custom dialer for upstream connections.
// It explicitly points the proxy destination to an internal HTTP URL,
// as TLS termination is handled at the proxy edge or upstream.
func NewTailscaleHTTPProxyServer(server *TailscaleProxyServer) (Server, error) {
	u := &url.URL{
		Scheme: "http",
		Host:   server.Hostname(),
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.Transport = &http.Transport{
		Dial: server.Dial,
	}
	return &TailscaleHTTPProxyServer{
		server: server,
		proxy:  proxy,
	}, nil
}

// Serve binds the proxy to a listener. It configures aggressive connection timeouts
// (Read/Write/Idle) to prevent resource exhaustion from slow clients or stale
// connections (e.g. Slowloris attacks).
func (tps *TailscaleHTTPProxyServer) Serve(l net.Listener) error {
	server := http.Server{
		Handler:           tps,
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return server.Serve(l)
}

// ServeHTTP acts as the main HTTP middleware. It authenticates the incoming
// connection via Tailscale's WhoIs to fetch user identity, handles any mandatory
// redirects, sanitizes and injects identity headers, and proxies the request.
func (tps *TailscaleHTTPProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userInfo, err := tps.server.WhoIs(r.Context(), r.RemoteAddr)
	if err != nil {
		slog.Error("error/http/ts-auth", "err", err)
		w.WriteHeader(500)
		return
	}
	if tps.handleRedirect(w, r) {
		return
	}
	tps.enrichHeaders(r, userInfo)
	tps.proxy.ServeHTTP(w, r)
}

// handleRedirect intercepts requests targeting external or mismatched hostnames.
// It forces clients to communicate via the proxy's canonical Tailscale hostname
// to prevent host header spoofing and ensure correct TLS/SNI routing.
func (tps *TailscaleHTTPProxyServer) handleRedirect(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Hostname() != "" && r.URL.Hostname() != tps.server.Hostname() {
		destinationURL := new(url.URL)
		*destinationURL = *r.URL
		destinationURL.Host = tps.server.Hostname() + tps.server.options.Listen
		if tps.server.options.EnableTLS {
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

// enrichHeaders injects trustable metadata about the original client into the request.
// It manages X-Forwarded-* headers to propagate the actual protocol and host seen
// by the proxy, while delegating Tailscale identity header injection to setTailscaleHeaders.
func (tps *TailscaleHTTPProxyServer) enrichHeaders(r *http.Request, userInfo *apitype.WhoIsResponse) {
	r.Header.Del(HeaderXForwardedProto)
	if tps.server.options.EnableTLS {
		r.Header.Set(HeaderXForwardedProto, SchemeHTTPS)
	} else {
		r.Header.Set(HeaderXForwardedProto, SchemeHTTP)
	}
	r.Header.Del(HeaderXForwardedHost)
	r.Header.Set(HeaderXForwardedHost, tps.server.Hostname())
	slog.Info("request", "method", r.Method, "user", userInfo.UserProfile.LoginName, "host", r.Host, "url", r.URL.String())
	setTailscaleHeaders(r, userInfo)
}

// setTailscaleHeaders strips any potentially spoofed identity headers from the incoming
// request, substituting them with cryptographically verified information from the WhoIs response.
// It iterates over the header map and uses delete() rather than Del() to ensure
// non-canonical variants (like those containing underscores) are successfully removed.
func setTailscaleHeaders(r *http.Request, userInfo *apitype.WhoIsResponse) {
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
