package tsproxy

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"

	"github.com/davecgh/go-spew/spew"
)

func init() {
	_ = spew.Dump
}

type TailscaleHTTPProxyServer struct {
	server *TailscaleProxyServer
	client *http.Client
	scheme string
}

func NewTailscaleHTTPProxyServer(server *TailscaleProxyServer) (Server, error) {
	transport := &http.Transport{
		Dial: server.Dial,
	}
	client := &http.Client{
		Transport: transport,
	}
	parsedURL, err := url.Parse(server.options.Upstream)
	if err != nil {
		return nil, err
	}
	return &TailscaleHTTPProxyServer{
		server: server,
		client: client,
		scheme: parsedURL.Scheme,
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
	log.Printf("got http conn")
	defer log.Printf("http conn end")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	req := r.Clone(ctx)
	req.URL.Scheme = tps.scheme
	req.URL.Host = "whatever-would-be-ignored-anyway"
	req.RequestURI = ""
	req.Header.Set("Tailscale-User-Login", userInfo.UserProfile.LoginName)
	req.Header.Set("Tailscale-User-Name", userInfo.UserProfile.DisplayName)
	req.Header.Set("Tailscale-User-Profile-Pic", userInfo.UserProfile.ProfilePicURL)
	req.Header.Set("Tailscale-Headers-Info", "https://tailscale.com/s/serve-headers")
	resp, err := tps.client.Do(req)
	if err != nil {
		log.Printf("error/http/proxy: %s", err.Error())
		w.WriteHeader(500)
		return
	}
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	buf := bufferPool.Get().([]byte)
	defer bufferPool.Put(buf)
	io.CopyBuffer(w, resp.Body, buf)
}
