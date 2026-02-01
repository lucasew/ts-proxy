package tsproxy

import (
	"context"
	"errors"
	"log/slog"
	"net"

	"os"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tsnet"
)

var (
	ErrInvalidUpstream = errors.New("invalid upstream")
)

type Server interface {
	Serve(ln net.Listener) error
}

type ListenerFunction func(network string, addr string) (net.Listener, error)

type TailscaleProxyServer struct {
	ctx     context.Context
	cancel  func()
	options TailscaleProxyServerOptions
	server  *tsnet.Server
}

type TailscaleProxyServerOptions struct {
	// context
	Context context.Context
	// node name in tailscale panel
	Hostname string
	// wether to enable Tailscale Funnel, will crash if no permissions
	EnableFunnel bool
	// wether to enable provisioning of TLS Certificates for HTTPS
	EnableTLS bool
	// wether to enable HTTP proxy logic
	EnableHTTP bool
	// where to store tailscale data
	StateDir string
	// protocol to listen, passed to net.Dial
	Network string
	// where to forward requests
	Address string
	// address to bind the server, passed to net.Dial
	Listen string
}

func NewTailscaleProxyServer(options TailscaleProxyServerOptions) (*TailscaleProxyServer, error) {
	if options.Context == nil {
		options.Context = context.Background()
	}
	ctx, cancel := context.WithCancel(options.Context)
	s := new(tsnet.Server)
	if options.Hostname == "" {
		options.Hostname = "tsproxy"
	}
	s.Hostname = options.Hostname
	if options.Address == "" {
		cancel()
		return nil, ErrInvalidUpstream
	}
	if options.StateDir != "" {
		err := os.MkdirAll(options.StateDir, 0700)
		if err != nil {
			cancel()
			return nil, err
		}
		s.Dir = options.StateDir
	}
	return &TailscaleProxyServer{
		ctx:     ctx,
		cancel:  cancel,
		options: options,
		server:  s,
	}, nil
}

func (tps *TailscaleProxyServer) listenFunnel(network string, addr string) (net.Listener, error) {
	return tps.server.ListenFunnel(network, addr)
}

func (tps *TailscaleProxyServer) Hostname() string {
	for _, domain := range tps.server.CertDomains() {
		return domain
	}
	return tps.options.Hostname
}

func (tps *TailscaleProxyServer) GetListenerFunction() ListenerFunction {
	if tps.options.EnableFunnel {
		return tps.listenFunnel
	}
	if tps.options.EnableTLS {
		return tps.server.ListenTLS
	}
	return tps.server.Listen
}

func (tps *TailscaleProxyServer) GetListener() (net.Listener, error) {
	return tps.GetListenerFunction()("tcp", tps.options.Listen)
}

func (tps *TailscaleProxyServer) Dial(network string, addr string) (net.Conn, error) {
	dialNetwork := tps.options.Network
	dialHost := tps.options.Address
	return net.Dial(dialNetwork, dialHost)
}

func (tps *TailscaleProxyServer) WhoIs(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error) {
	lc, err := tps.server.LocalClient()
	if err != nil {
		return nil, err
	}
	return lc.WhoIs(ctx, remoteAddr)
}

func (tps *TailscaleProxyServer) handleError(err error) bool {
	if err != nil {
		slog.Error("FATAL ERROR", "err", err)
		tps.cancel()
	}
	return err != nil
}

func (tps *TailscaleProxyServer) Run() {
	ln, err := tps.GetListener()
	if tps.handleError(err) {
		return
	}
	defer func() { _ = ln.Close() }()
	server := NewTailscaleTCPProxyServer(tps)
	if tps.options.EnableHTTP {
		server, err = NewTailscaleHTTPProxyServer(tps)
		if tps.handleError(err) {
			return
		}
	}
	if err := server.Serve(ln); err != nil {
		slog.Error("server serve error", "err", err)
	}
}
