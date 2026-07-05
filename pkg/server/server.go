package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"

	"github.com/lucasew/ts-proxy/pkg/config"
	"github.com/lucasew/ts-proxy/pkg/handler"
	"github.com/lucasew/ts-proxy/pkg/tsproxy"
	"golang.org/x/sync/errgroup"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tsnet"
)

// Options for creating a Server.
type Options struct {
	Hostname string
	StateDir string
	AuthKey  string
	Handlers []config.HandlerConfig
}

// Server manages a single Tailscale node and its handlers.
type Server struct {
	name string
	opts Options
	sm   *StateMachine
	ts   *tsnet.Server
}

// NewServer creates a server from options.
func NewServer(name string, opts Options) *Server {
	return &Server{
		name: name,
		opts: opts,
		sm:   NewStateMachine(),
	}
}

// Name returns the server's slug name.
func (s *Server) Name() string {
	return s.name
}

// State returns the current lifecycle state.
func (s *Server) State() State {
	return s.sm.Current()
}

// FQDN returns the fully qualified domain name after authentication,
// falling back to the configured hostname.
func (s *Server) FQDN() string {
	if s.ts != nil {
		for _, domain := range s.ts.CertDomains() {
			return domain
		}
	}
	return s.opts.Hostname
}

// mustTransition records a lifecycle change; invalid transitions are logged and ignored
// so callers keep working even if the state machine lags behind reality.
func (s *Server) mustTransition(to State) {
	if err := s.sm.Transition(to); err != nil {
		slog.Warn("state transition rejected", "server", s.name, "to", to, "current", s.sm.Current(), "err", err)
	}
}

// Start initializes the Tailscale node and authenticates.
func (s *Server) Start(ctx context.Context) error {
	s.mustTransition(StateStarting)

	if err := os.MkdirAll(s.opts.StateDir, 0700); err != nil {
		s.mustTransition(StateFailed)
		return fmt.Errorf("create state dir %s: %w", s.opts.StateDir, err)
	}

	s.ts = &tsnet.Server{
		Hostname: s.opts.Hostname,
		Dir:      s.opts.StateDir,
	}
	if s.opts.AuthKey != "" {
		s.ts.AuthKey = s.opts.AuthKey
	}

	s.mustTransition(StateAuthenticating)
	slog.Info("authenticating", "server", s.name)

	_, err := s.ts.Up(ctx)
	if err != nil {
		s.mustTransition(StateFailed)
		if cerr := s.ts.Close(); cerr != nil {
			tsproxy.ReportError(cerr, "context", "tailscale close error")
		}
		s.ts = nil
		return fmt.Errorf("tailscale up: %w", err)
	}

	slog.Info("authenticated", "server", s.name, "fqdn", s.FQDN())
	return nil
}

// Serve starts all handlers. Must be called after Start.
func (s *Server) Serve(ctx context.Context) error {
	if s.ts == nil {
		return fmt.Errorf("server not started")
	}

	lc, err := s.ts.LocalClient()
	if err != nil {
		s.mustTransition(StateFailed)
		return fmt.Errorf("local client: %w", err)
	}

	whoIs := func(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error) {
		return lc.WhoIs(ctx, remoteAddr)
	}

	fqdn := s.FQDN()
	s.mustTransition(StateRunning)

	g, gCtx := errgroup.WithContext(ctx)
	for _, hc := range s.opts.Handlers {
		hc := hc
		g.Go(func() error {
			h, err := s.createHandler(hc, fqdn, whoIs)
			if err != nil {
				return fmt.Errorf("create handler %s %s: %w", hc.Type, hc.Listen, err)
			}

			lf := s.listenerFunc(hc.TLS, hc.Funnel)
			ln, err := lf("tcp", hc.Listen)
			if err != nil {
				return fmt.Errorf("listen %s: %w", hc.Listen, err)
			}
			defer func() {
				if cerr := ln.Close(); cerr != nil {
					tsproxy.ReportError(cerr, "context", "listener close error")
				}
			}()

			slog.Info("handler listening",
				"server", s.name,
				"type", hc.Type,
				"listen", hc.Listen,
				"upstream", hc.UpstreamAddress,
			)
			return h.Serve(gCtx, ln)
		})
	}

	err = g.Wait()
	if err != nil {
		s.mustTransition(StateFailed)
	} else {
		s.mustTransition(StateStopped)
	}
	return err
}

// Run performs the full lifecycle: start, serve, close.
func (s *Server) Run(ctx context.Context) error {
	if err := s.Start(ctx); err != nil {
		return err
	}
	defer func() {
		if err := s.Close(); err != nil {
			tsproxy.ReportError(err, "context", "server close error")
		}
	}()
	return s.Serve(ctx)
}

// Close shuts down the Tailscale node.
func (s *Server) Close() error {
	if s.ts != nil {
		err := s.ts.Close()
		s.ts = nil
		return err
	}
	return nil
}

// ResetState prepares the server for a restart by resetting the state machine.
func (s *Server) ResetState() {
	s.sm = NewStateMachine()
}

func (s *Server) createHandler(hc config.HandlerConfig, fqdn string, whoIs handler.WhoIsFunc) (handler.Handler, error) {
	switch hc.Type {
	case "tcp":
		return handler.NewTCP(hc.UpstreamNetwork, hc.UpstreamAddress), nil
	case "http":
		return handler.NewHTTP(handler.HTTPOptions{
			Hostname:        fqdn,
			EnableTLS:       hc.TLS,
			UpstreamAddress: hc.UpstreamAddress,
			UpstreamNetwork: hc.UpstreamNetwork,
			WhoIs:           whoIs,
		}), nil
	default:
		return nil, fmt.Errorf("unknown handler type: %s", hc.Type)
	}
}

func (s *Server) listenerFunc(tls, funnel bool) func(string, string) (net.Listener, error) {
	if funnel {
		return func(network, addr string) (net.Listener, error) {
			return s.ts.ListenFunnel(network, addr)
		}
	}
	if tls {
		return s.ts.ListenTLS
	}
	return s.ts.Listen
}
