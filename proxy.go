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

// TailscaleProxyServer is the central orchestrator that wraps a tsnet.Server.
// It manages the connection lifecycle, provisions state directories, and
// delegates incoming Tailscale traffic to either a TCP or HTTP proxy layer.
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
	// whether to enable Tailscale Funnel, exposing the node to the public internet.
	// This will crash if the Tailnet's Access Controls (ACLs) do not explicitly permit Funnel for this node.
	EnableFunnel bool
	// whether to enable automatic provisioning of TLS certificates (requires HTTPS enabled in Tailscale admin).
	EnableTLS bool
	// whether to use the HTTP proxy implementation, which enriches requests with Tailscale identity headers.
	// If false, it falls back to a raw TCP byte-copying proxy.
	EnableHTTP bool
	// directory to store Tailscale machine state. This enables the node to persist its Tailnet identity
	// across restarts instead of acting as an ephemeral node.
	StateDir string
	// network protocol for outbound upstream connections (e.g., "tcp", "udp"), passed to net.Dial.
	Network string
	// the upstream destination address (host:port) to forward all proxied traffic.
	Address string
	// the local bind address (e.g., ":80", ":443") for the Tailscale listener.
	Listen string
}

// NewTailscaleProxyServer initializes a new Tailscale orchestrator with the provided options.
// It creates a new tsnet.Server, sets up the machine hostname, provisions the persistent state
// directory if specified, and wraps the provided context to handle lifecycle cancellation on fatal errors.
func NewTailscaleProxyServer(options TailscaleProxyServerOptions) (_ *TailscaleProxyServer, err error) {
	if options.Context == nil {
		options.Context = context.Background()
	}
	ctx, cancel := context.WithCancel(options.Context)
	defer func() {
		if err != nil {
			cancel()
		}
	}()
	s := new(tsnet.Server)
	if options.Hostname == "" {
		options.Hostname = "tsproxy"
	}
	s.Hostname = options.Hostname
	if options.Address == "" {
		return nil, ErrInvalidUpstream
	}
	if options.StateDir != "" {
		err := os.MkdirAll(options.StateDir, 0700)
		if err != nil {
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

// GetListenerFunction dictates the listener strategy based on configuration.
// It returns the appropriate Tailscale listener mechanism: Funnel (public ingress),
// TLS (encrypted internal Tailnet traffic), or standard TCP (unencrypted Tailnet traffic).
func (tps *TailscaleProxyServer) GetListenerFunction() ListenerFunction {
	if tps.options.EnableFunnel {
		return tps.listenFunnel
	}
	if tps.options.EnableTLS {
		return tps.server.ListenTLS
	}
	return tps.server.Listen
}

// GetListener invokes the chosen listener strategy (Funnel, TLS, or TCP)
// to bind to the address specified in the configuration options.
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

// Run executes the core server lifecycle. It retrieves the configured listener,
// initializes the underlying proxy strategy (TCP or HTTP), and begins serving
// traffic, blocking until the context is cancelled or a fatal error occurs.
func (tps *TailscaleProxyServer) Run() {
	ln, err := tps.GetListener()
	if tps.handleError(err) {
		return
	}
	defer ln.Close()
	server := NewTailscaleTCPProxyServer(tps)
	if tps.options.EnableHTTP {
		server, err = NewTailscaleHTTPProxyServer(tps)
		if tps.handleError(err) {
			return
		}
	}
	server.Serve(ln)
}
