package server

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lucasew/ts-proxy/pkg/config"
)

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
func (s *Supervisor) StartAll(ctx context.Context) error {
	for _, srv := range s.servers {
		if err := srv.Start(ctx); err != nil {
			return fmt.Errorf("server %s: %w", srv.Name(), err)
		}
	}
	return nil
}

// CloseAll shuts down all servers.
func (s *Supervisor) CloseAll() {
	for _, srv := range s.servers {
		if err := srv.Close(); err != nil {
			slog.Error("close error", "server", srv.Name(), "err", err)
		}
	}
}

// DisplayAuthenticated prints server info including FQDN (after authentication).
func (s *Supervisor) DisplayAuthenticated() string {
	var b strings.Builder

	// Compute global max widths for handler columns (common vertical alignment across all servers)
	maxListen := 0
	maxTypeFlags := 0
	for _, srv := range s.servers {
		scfg := s.cfg.Servers[srv.Name()]
		for _, h := range scfg.Handlers {
			if len(h.Listen) > maxListen {
				maxListen = len(h.Listen)
			}
			flags := ""
			if h.TLS {
				flags += " TLS"
			}
			if h.Funnel {
				flags += " Funnel"
			}
			if flags != "" {
				flags = " [" + flags[1:] + "]"
			}
			tf := h.Type + flags
			if len(tf) > maxTypeFlags {
				maxTypeFlags = len(tf)
			}
		}
	}

	for _, srv := range s.servers {
		scfg := s.cfg.Servers[srv.Name()]
		fmt.Fprintf(&b, "%s (%s)\n", srv.Name(), srv.FQDN())
		for _, h := range scfg.Handlers {
			flags := ""
			if h.TLS {
				flags += " TLS"
			}
			if h.Funnel {
				flags += " Funnel"
			}
			if flags != "" {
				flags = " [" + flags[1:] + "]"
			}
			typeFlags := h.Type + flags
			fmt.Fprintf(&b, "  %-*s %-*s -> %s\n",
				maxListen, h.Listen,
				maxTypeFlags, typeFlags,
				h.UpstreamAddress)
		}
	}
	return b.String()
}

// Run starts all servers and supervises them.
// If StopOnFail is true, any server failure stops all servers.
// Otherwise, failed servers are restarted with a delay.
func (s *Supervisor) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once

	for _, srv := range s.servers {
		srv := srv
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := s.runWithRestart(ctx, srv)
			if err != nil {
				errOnce.Do(func() {
					firstErr = err
					cancel()
				})
			}
		}()
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
			slog.Error("server failed", "name", srv.Name(), "error", err)
			if s.cfg.StopOnFail {
				return fmt.Errorf("server %s: %w", srv.Name(), err)
			}
			slog.Info("restarting server", "name", srv.Name(), "delay", "5s")
			srv.ResetState()
			select {
			case <-time.After(5 * time.Second):
			case <-ctx.Done():
				return nil
			}
			continue
		}
		return nil
	}
}
