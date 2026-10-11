package server

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/lucasew/ts-proxy/internal/ctxwait"
	"github.com/lucasew/ts-proxy/pkg/config"
	"github.com/lucasew/ts-proxy/pkg/status"
	"github.com/lucasew/ts-proxy/pkg/tsproxy"
)

// restartDelay is how long runWithRestart waits before starting a failed
// server again when StopOnFail is false.
const restartDelay = 5 * time.Second

// Supervisor manages multiple servers and their lifecycles.
type Supervisor struct {
	cfg     *config.Config
	servers []*Server
}

// NewSupervisor creates a supervisor from a validated config.
func NewSupervisor(cfg *config.Config) *Supervisor {
	var servers []*Server
	for _, name := range cfg.ServerNames() {
		scfg := cfg.Servers[name]
		authKey := ""
		if scfg.Token != "" {
			if tok, ok := cfg.Tokens[scfg.Token]; ok {
				authKey = tok.AuthKey
			}
		}
		stateDir := filepath.Join(cfg.StateDir, name)
		srv := NewServer(name, Options{
			Hostname: scfg.Hostname,
			StateDir: stateDir,
			AuthKey:  authKey,
			Forward:  scfg.Forward,
			Handlers: scfg.Handlers,
		})
		servers = append(servers, srv)
	}
	return &Supervisor{
		cfg:     cfg,
		servers: servers,
	}
}

// Servers returns all managed servers.
func (s *Supervisor) Servers() []*Server {
	return s.servers
}

// StartAll authenticates all servers without serving traffic.
// Used for dry-run mode.
// If any server fails to start, servers that already authenticated are closed
// before the error is returned so tsnet nodes are not left running.
func (s *Supervisor) StartAll(ctx context.Context) error {
	for i, srv := range s.servers {
		if err := srv.Start(ctx); err != nil {
			for j := 0; j < i; j++ {
				if cerr := s.servers[j].Close(); cerr != nil {
					tsproxy.ReportError(cerr, "context", "close error after start failure", "server", s.servers[j].Name())
				}
			}
			return fmt.Errorf("server %s: %w", srv.Name(), err)
		}
	}
	return nil
}

// CloseAll shuts down all servers.
func (s *Supervisor) CloseAll() {
	for _, srv := range s.servers {
		if err := srv.Close(); err != nil {
			tsproxy.ReportError(err, "context", "close error", "server", srv.Name())
		}
	}
}

// DisplayAuthenticated prints server info including FQDN (after authentication).
func (s *Supervisor) DisplayAuthenticated() string {
	sections := make([]config.HandlerSection, 0, len(s.servers))
	for _, srv := range s.servers {
		scfg := s.cfg.Servers[srv.Name()]
		header := fmt.Sprintf("%s (%s)", srv.Name(), srv.FQDN())
		if scfg.Forward != "" {
			header += fmt.Sprintf(" [forward: %s]", scfg.Forward)
		}
		header += "\n"
		sections = append(sections, config.HandlerSection{
			Header:   header,
			Handlers: scfg.Handlers,
		})
	}
	return config.FormatHandlerSections(sections)
}

// Snapshot copies the current server state for the status listener.
func (s *Supervisor) Snapshot() []status.Server {
	out := make([]status.Server, 0, len(s.servers))
	for _, srv := range s.servers {
		handlers := make([]status.Handler, len(srv.opts.Handlers))
		for i, h := range srv.opts.Handlers {
			handlers[i] = status.Handler{
				Type:     h.Type,
				Listen:   h.Listen,
				Upstream: h.UpstreamAddress,
				Network:  h.UpstreamNetwork,
			}
		}
		out = append(out, status.Server{
			Name:     srv.Name(),
			State:    string(srv.State()),
			FQDN:     srv.FQDN(),
			Forward:  srv.opts.Forward,
			Handlers: handlers,
		})
	}
	return out
}

// Run starts all servers and supervises them.
// If StopOnFail is true, any server failure stops all servers.
// Otherwise, failed servers are restarted with a delay.
// When StatusListen is set, Run also serves the status listener until ctx ends.
// The listener is bound before any server starts. A bind error returns without
// leaving nodes up. The status goroutine exits when ctx is canceled.
func (s *Supervisor) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once
	fail := func(err error) {
		errOnce.Do(func() {
			firstErr = err
			cancel()
		})
	}

	if addr := s.cfg.StatusListen; addr != "" {
		ln, err := status.Listen(addr)
		if err != nil {
			return err
		}
		slog.Info("status listener", "addr", ln.Addr().String())
		wg.Go(func() {
			// Serve closes ln. A canceled ctx is a clean stop.
			err := status.Serve(ctx, ln, s.Snapshot)
			if err != nil && ctx.Err() == nil {
				fail(err)
			}
		})
	}

	for _, srv := range s.servers {
		wg.Go(func() {
			err := s.runWithRestart(ctx, srv)
			if err != nil {
				fail(err)
			}
		})
	}

	wg.Wait()
	return firstErr
}

func (s *Supervisor) runWithRestart(ctx context.Context, srv *Server) error {
	for {
		slog.Info("starting server", "name", srv.Name())
		err := srv.Run(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			tsproxy.ReportError(err, "context", "server failed", "name", srv.Name())
			if s.cfg.StopOnFail {
				return fmt.Errorf("server %s: %w", srv.Name(), err)
			}
			slog.Info("restarting server", "name", srv.Name(), "delay", restartDelay.String())
			srv.ResetState()
			if !ctxwait.Delay(ctx, restartDelay) {
				return nil
			}
			continue
		}
		return nil
	}
}
