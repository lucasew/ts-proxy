package tsproxy

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"

	"os"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tsnet"
)

type TailscaleProxyServer struct {
	ctx     context.Context
	cancel  func()
	proxy   *httputil.ReverseProxy
	mux     *http.ServeMux
	options TailscaleProxyServerOptions
	server  *tsnet.Server
}

type TailscaleProxyServerOptions struct {
	// context
	Context context.Context
	// where to forward requests
	Upstream *url.URL
	// node name in tailscale panel
	Hostname string
	// wether to enable Tailscale Funnel, will crash if no permissions
	EnableFunnel bool
	// where to store tailscale data
	StateDir string
	// address to bind the server
	Addr string
}

var (
	ErrInvalidUpstream = errors.New("invalid upstream")
)

func NewTailscaleProxyServer(options TailscaleProxyServerOptions) (*TailscaleProxyServer, error) {
	if options.Context == nil {
		options.Context = context.Background()
	}
	ctx, cancel := context.WithCancel(options.Context)
	s := new(tsnet.Server)
	s.Hostname = options.Hostname
	if options.Hostname == "" {
		s.Hostname = "tsproxy"
	}
	if options.Upstream == nil {
		return nil, ErrInvalidUpstream
	}
	if options.StateDir != "" {
		err := os.MkdirAll(options.StateDir, 0700)
		if err != nil {
			return nil, err
		}
		s.Dir = options.StateDir
	}

	proxy := httputil.NewSingleHostReverseProxy(options.Upstream)
	mux := http.NewServeMux()
	ret := &TailscaleProxyServer{
		ctx:     ctx,
		cancel:  cancel,
		proxy:   proxy,
		mux:     mux,
		options: options,
		server:  s,
	}
	mux.HandleFunc("/", ret.ServeHTTP)
	return ret, nil
}

func (tps *TailscaleProxyServer) handleServer(ln net.Listener) {
	server := http.Server{
		Handler: tps,
		BaseContext: func(_ net.Listener) context.Context {
			return tps.ctx
		},
	}
	err := server.Serve(ln)
	tps.handleError(err)
}

func (tps *TailscaleProxyServer) handleError(err error) bool {
	if err != nil {
		log.Printf("FATAL ERROR: %s\n", err.Error())
		tps.cancel()
	}
	return err != nil
}

func (tps *TailscaleProxyServer) Run() {
	var ln net.Listener
	var err error
	if tps.options.EnableFunnel {
		ln, err = tps.server.ListenFunnel("tcp", tps.options.Addr)
	} else {
		ln, err = tps.server.Listen("tcp", tps.options.Addr)
	}
	if tps.handleError(err) {
		return
	}
	go tps.handleServer(ln)
	<-tps.ctx.Done()
}

func (tps *TailscaleProxyServer) WhoIs(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error) {
	lc, err := tps.server.LocalClient()
	if err != nil {
		return nil, err
	}
	return lc.WhoIs(ctx, remoteAddr)
}

func (tps *TailscaleProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	userInfo, err := tps.WhoIs(r.Context(), r.RemoteAddr)
	if err != nil {
		w.WriteHeader(500)
		return
	}
	r.Header.Set("Tailscale-User-Login", userInfo.UserProfile.LoginName)
	r.Header.Set("Tailscale-User-Name", userInfo.UserProfile.DisplayName)
	r.Header.Set("Tailscale-User-Profile-Pic", userInfo.UserProfile.ProfilePicURL)
	r.Header.Set("Tailscale-Headers-Info", "https://tailscale.com/s/serve-headers")
	tps.proxy.ServeHTTP(w, r)
}
