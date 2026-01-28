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

/**
 * Server defines the interface for a proxy server backend (TCP or HTTP).
 * Implementations are responsible for accepting connections from the listener
 * and handling the traffic according to their specific protocol logic.
 */
type Server interface {
	Serve(ln net.Listener) error
}

/**
 * ListenerFunction is a factory type for creating net.Listener instances.
 * It abstracts the difference between standard TCP listeners and Tailscale's
 * tsnet listeners (Funnel, TLS, or standard).
 */
type ListenerFunction func(network string, addr string) (net.Listener, error)

/**
 * TailscaleProxyServer is the core struct that manages the Tailscale connection
 * and acts as the bridge between the Tailnet and the upstream service.
 *
 * It embeds a tsnet.Server which provides the virtual network interface.
 * The server handles lifecycle management, configuration, and selecting
 * the appropriate listening mode (Standard, TLS, or Funnel).
 */
type TailscaleProxyServer struct {
	ctx     context.Context
	cancel  func()
	options TailscaleProxyServerOptions
	server  *tsnet.Server
}

/**
 * TailscaleProxyServerOptions configures the behavior of the TailscaleProxyServer.
 * It dictates the identity of the node on the Tailnet, how it listens for
 * traffic, and where that traffic is forwarded.
 */
type TailscaleProxyServerOptions struct {
	/**
	 * Context controls the lifecycle of the server.
	 * If nil, context.Background() is used.
	 */
	Context context.Context
	/**
	 * Hostname is the machine name as it appears in the Tailscale admin console.
	 * Defaults to "tsproxy" if not specified.
	 */
	Hostname string
	/**
	 * EnableFunnel toggles the exposure of the service to the public internet via Tailscale Funnel.
	 * Note: This requires the node to have the necessary ACL permissions on the Tailnet.
	 * If true, it attempts to listen on the public funnel address.
	 */
	EnableFunnel bool
	/**
	 * EnableTLS toggles automatic TLS certificate management via Tailscale.
	 * When enabled, the server will provision valid certificates for its TS hostname.
	 */
	EnableTLS bool
	/**
	 * EnableHTTP toggles the HTTP-specific proxy logic.
	 * If true, the server acts as an L7 HTTP proxy, enabling header injection
	 * and path-based routing. If false, it acts as a raw L4 TCP proxy.
	 */
	EnableHTTP bool
	/**
	 * StateDir specifies where the tailscale backend stores its state (keys, etc.).
	 * Ensuring persistence here is critical for maintaining the node's identity across restarts.
	 */
	StateDir string
	/**
	 * Network is the protocol network type used for dialing the upstream service.
	 * Typically "tcp", "tcp4", or "tcp6". Passed directly to net.Dial.
	 */
	Network string
	/**
	 * Address is the upstream destination to forward traffic to.
	 * E.g., "localhost:8080" or "192.168.1.50:3000".
	 */
	Address string
	/**
	 * Listen is the local bind address for the server.
	 * While tsnet handles the virtual listener, this might still be relevant
	 * for determining the port to request or bind to within the tsnet scope.
	 */
	Listen string
}

/**
 * NewTailscaleProxyServer initializes a new proxy server instance.
 * It validates the options, sets up the tsnet.Server configuration, and
 * prepares the execution context.
 *
 * @param options Configuration options for the server.
 * @returns A pointer to the initialized TailscaleProxyServer or an error.
 */
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

/**
 * Hostname retrieves the effective hostname of the proxy.
 * It attempts to return the full domain name from the TLS certificate if available,
 * falling back to the configured hostname.
 */
func (tps *TailscaleProxyServer) Hostname() string {
	for _, domain := range tps.server.CertDomains() {
		return domain
	}
	return tps.options.Hostname
}

/**
 * GetListenerFunction determines the appropriate listening strategy based on configuration.
 *
 * Strategy priority:
 * 1. Funnel (Public Internet)
 * 2. TLS (Tailnet HTTPS)
 * 3. Standard (Tailnet Raw TCP)
 *
 * @returns A ListenerFunction compatible with net.Listen signature.
 */
func (tps *TailscaleProxyServer) GetListenerFunction() ListenerFunction {
	if tps.options.EnableFunnel {
		return tps.listenFunnel
	}
	if tps.options.EnableTLS {
		return tps.server.ListenTLS
	}
	return tps.server.Listen
}

/**
 * GetListener creates and returns the network listener.
 * It invokes the strategy determined by GetListenerFunction.
 */
func (tps *TailscaleProxyServer) GetListener() (net.Listener, error) {
	return tps.GetListenerFunction()("tcp", tps.options.Listen)
}

/**
 * Dial opens a connection to the upstream service.
 * It ignores the provided network/addr arguments and uses the statically configured
 * upstream Network and Address from options. This ensures all traffic is forced
 * to the configured target.
 */
func (tps *TailscaleProxyServer) Dial(network string, addr string) (net.Conn, error) {
	dialNetwork := tps.options.Network
	dialHost := tps.options.Address
	return net.Dial(dialNetwork, dialHost)
}

/**
 * WhoIs queries the Tailscale local client to identify the user behind a remote address.
 * This is crucial for authentication and identity propagation.
 *
 * @param ctx The context for the request.
 * @param remoteAddr The remote address (IP:port) of the incoming connection.
 * @returns The Tailscale identity information or an error.
 */
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

/**
 * Run starts the proxy server loop.
 * It initializes the listener and hands it off to either the HTTP or TCP
 * proxy handler based on the EnableHTTP option.
 * This method blocks until the server stops or encounters a fatal error.
 */
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
