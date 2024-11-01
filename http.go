package tsproxy

import (
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/davecgh/go-spew/spew"
)

func init() {
	_ = spew.Dump
}

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
		log.Printf("error/http/ts-auth: %s", err.Error())
		w.WriteHeader(500)
		return
	}
	if r.URL.Hostname() != tps.server.Hostname() {
		destinationURL := new(url.URL)
		*destinationURL = *r.URL
		destinationURL.Host = tps.server.Hostname() + tps.server.options.Listen
		if tps.server.options.EnableTLS {
			destinationURL.Scheme = "https"
		} else {
			destinationURL.Scheme = "http"
		}
		http.Redirect(w, r, destinationURL.String(), http.StatusMovedPermanently)
		return
	}
	log.Printf("%s %s %s %s", r.Method, userInfo.UserProfile.LoginName, r.Host, r.URL.String())
	r.Header.Set("Tailscale-User-Login", userInfo.UserProfile.LoginName)
	r.Header.Set("Tailscale-User-Name", userInfo.UserProfile.DisplayName)
	r.Header.Set("Tailscale-User-Profile-Pic", userInfo.UserProfile.ProfilePicURL)
	r.Header.Set("Tailscale-Headers-Info", "https://tailscale.com/s/serve-headers")
	tps.proxy.ServeHTTP(w, r)
}
