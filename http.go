package tsproxy

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
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
	server := http.Server{Handler: tps}
	return server.Serve(l)
}

func (tps *TailscaleHTTPProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userInfo, err := tps.server.WhoIs(r.Context(), r.RemoteAddr)
	if err != nil {
		slog.Error("error/http/ts-auth", "err", err)
		w.WriteHeader(500)
		return
	}
	if r.URL.Hostname() != "" && r.URL.Hostname() != tps.server.Hostname() {
		destinationURL := new(url.URL)
		*destinationURL = *r.URL
		destinationURL.Host = tps.server.Hostname() + tps.server.options.Listen
		if tps.server.options.EnableTLS {
			destinationURL.Scheme = "https"
		} else {
			destinationURL.Scheme = "http"
		}
		slog.Info("redirect", "from", r.URL.String(), "to", destinationURL.String())
		http.Redirect(w, r, destinationURL.String(), http.StatusMovedPermanently)
		return
	}
	r.Header.Del("X-Forwarded-Proto")
	if tps.server.options.EnableTLS {
		r.Header.Set("X-Forwarded-Proto", "https")
	} else {
		r.Header.Set("X-Forwarded-Proto", "http")
	}
	slog.Info("request", "method", r.Method, "user", userInfo.UserProfile.LoginName, "host", r.Host, "url", r.URL.String())
	r.Header.Del("Tailscale-User-Login")
	r.Header.Del("Tailscale-User-Name")
	r.Header.Del("Tailscale-User-Profile-Pic")
	r.Header.Del("Tailscale-Headers-Info")
	r.Header.Set("Tailscale-User-Login", userInfo.UserProfile.LoginName)
	r.Header.Set("Tailscale-User-Name", userInfo.UserProfile.DisplayName)
	r.Header.Set("Tailscale-User-Profile-Pic", userInfo.UserProfile.ProfilePicURL)
	r.Header.Set("Tailscale-Headers-Info", "https://tailscale.com/s/serve-headers")
	tps.proxy.ServeHTTP(w, r)
}
