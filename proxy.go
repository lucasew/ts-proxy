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

// Server defines the interface for protocol-specific proxy handlers (e.g., TCP, HTTP)
// that serve connections on a given listener.
type Server interface {
	Serve(ln net.Listener) error
}

// ListenerFunction is a signature for functions that establish network listeners,
// allowing the orchestrator to dynamically switch between standard, TLS, or Funnel backends.
type ListenerFunction func(network string, addr string) (net.Listener, error)

// TailscaleProxyServer is the core orchestrator managing the tsnet server lifecycle.
// It acts as the central hub for routing, identity resolution, and provisioning
// child proxy servers (HTTP/TCP) based on configuration.
type TailscaleProxyServer struct {
	ctx     context.Context
	cancel  func()
	options TailscaleProxyServerOptions
	server  *tsnet.Server
}

// TailscaleProxyServerOptions holds configuration parameters for the orchestrator,
// dictating which proxy features, network protocols, and Tailscale integrations are enabled.
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

// NewTailscaleProxyServer initializes the tsnet server and sets up context cancellation
// for graceful shutdown. It configures the internal state directory but does not begin
// listening for connections.
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

// Hostname prioritizes TLS-provisioned domains over the statically configured node name.
// This ensures correct routing and redirection when Funnel or TLS certificates are active.
func (tps *TailscaleProxyServer) Hostname() string {
	for _, domain := range tps.server.CertDomains() {
		return domain
	}
	return tps.options.Hostname
}

// GetListenerFunction determines the appropriate network binding method based on configuration.
// It falls back from Funnel (public) to TLS (secure internal) to standard TCP (internal).
func (tps *TailscaleProxyServer) GetListenerFunction() ListenerFunction {
	if tps.options.EnableFunnel {
		return tps.listenFunnel
	}
	if tps.options.EnableTLS {
		return tps.server.ListenTLS
	}
	return tps.server.Listen
}

// GetListener invokes the selected listener function specifically for a TCP port,
// returning the active network listener for incoming proxy connections.
func (tps *TailscaleProxyServer) GetListener() (net.Listener, error) {
	return tps.GetListenerFunction()("tcp", tps.options.Listen)
}

// Dial routes outbound traffic directly through the host network to the upstream target,
// bypassing the tailnet for forwarding operations.
func (tps *TailscaleProxyServer) Dial(network string, addr string) (net.Conn, error) {
	dialNetwork := tps.options.Network
	dialHost := tps.options.Address
	return net.Dial(dialNetwork, dialHost)
}

// WhoIs queries the tsnet LocalClient to resolve caller identities (like usernames or profile pics)
// from their Tailscale IP addresses. This is critical for HTTP header enrichment and authorization.
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

// Run starts the orchestrator blockingly. It binds the listener, instantiates the required
// TCP or HTTP child handlers based on the configuration, and serves incoming connections
// until the context is cancelled.
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
