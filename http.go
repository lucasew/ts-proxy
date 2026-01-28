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

/**
 * TailscaleHTTPProxyServer is an L7 HTTP proxy implementation.
 * It wraps the generic httputil.ReverseProxy to add Tailscale-specific features:
 * - Authentication (WhoIs)
 * - Header injection (Identity, X-Forwarded-*)
 * - Canonical hostname enforcement
 */
type TailscaleHTTPProxyServer struct {
	server *TailscaleProxyServer
	proxy  *httputil.ReverseProxy
}

/**
 * NewTailscaleHTTPProxyServer creates a new HTTP proxy instance.
 * It configures a standard SingleHostReverseProxy but overrides the Transport
 * to use the TailscaleProxyServer's Dial method, ensuring all traffic
 * is routed to the configured upstream address.
 */
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

/**
 * Serve starts the HTTP server on the provided listener.
 * It configures conservative timeouts to protect against Slowloris-style attacks.
 */
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

/**
 * ServeHTTP handles individual HTTP requests.
 * The flow is as follows:
 * 1. Authenticate the request using Tailscale's WhoIs (maps IP to user identity).
 * 2. Enforce canonical hostname via redirection if necessary.
 * 3. Sanitize and inject identity headers.
 * 4. Forward the request to the upstream service.
 */
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

/**
 * handleRedirect enforces that requests are made to the canonical hostname.
 * If a request comes in for a different hostname (e.g., via a CNAME or IP),
 * it redirects the client to the configured Tailscale hostname.
 *
 * @returns true if a redirect was sent, false otherwise.
 */
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

/**
 * enrichHeaders prepares the request headers for the upstream service.
 * It handles standard proxy headers (X-Forwarded-Proto, X-Forwarded-Host)
 * and delegates Tailscale-specific header injection.
 */
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

/**
 * setTailscaleHeaders securely injects Tailscale identity headers.
 *
 * CRITICAL SECURITY:
 * Before setting headers, it iterates through ALL existing request headers
 * to delete any that might be spoofing Tailscale headers (including underscore variations).
 * Only after sanitization are the authoritative headers set.
 */
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
