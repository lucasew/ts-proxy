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

type TailscaleHTTPProxyServer struct {
	server *TailscaleProxyServer
	proxy  *httputil.ReverseProxy
}

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
