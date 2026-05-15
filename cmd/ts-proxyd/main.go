package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sync"

	tsproxy "github.com/lucasew/ts-proxy"
	"github.com/lucasew/ts-proxy/internal/configsources"
	"github.com/lucasew/ts-proxy/internal/configsources/envvar"
	"github.com/lucasew/ts-proxy/internal/configsources/flagsource"
)

func main() {
	var (
		stateDir string
		dryRun   bool
	)
	flag.StringVar(&stateDir, "state-dir", "", "Base state directory for all servers")
	flag.BoolVar(&dryRun, "dry-run", false, "Validate configuration and list servers without starting them")

	// Register configuration sources.
	configsources.Register(flagsource.New()) // adds its own flags
	configsources.Register(envvar.New())

	flag.Parse()

	// Environment overrides for global options.
	if v := os.Getenv("TSPROXY_STATEDIR"); v != "" && stateDir == "" {
		stateDir = v
	}
	if os.Getenv("TSPROXY_DRY_RUN") != "" {
		dryRun = true
	}

	// Discover servers from all registered sources.
	configs, err := configsources.DiscoverAll()
	if err != nil {
		slog.Error("configuration discovery failed", "err", err)
		os.Exit(1)
	}
	if len(configs) == 0 {
		fmt.Fprintln(os.Stderr, "no servers configured — provide flags or TSPROXY_<name>_ADDRESS environment variables")
		os.Exit(1)
	}

	configsources.ApplyDefaults(configs, stateDir)

	// Display discovered servers.
	slog.Info("discovered servers", "count", len(configs))
	for _, c := range configs {
		slog.Info("server",
			"name", c.Name,
			"network", c.Network,
			"address", c.Address,
			"hostname", c.Hostname,
			"funnel", c.EnableFunnel,
			"tls", c.EnableTLS,
			"http", c.EnableHTTP,
			"statedir", c.StateDir,
			"listen", c.Listen,
		)
	}

	if dryRun {
		slog.Info("dry run mode — exiting without starting servers")
		return
	}

	// Launch all servers in parallel.
	var wg sync.WaitGroup
	for _, c := range configs {
		wg.Add(1)
		go func(cfg configsources.ServerConfig) {
			defer wg.Done()
			runServer(cfg)
		}(c)
	}
	wg.Wait()
}

func runServer(cfg configsources.ServerConfig) {
	opts := tsproxy.TailscaleProxyServerOptions{
		Network:      cfg.Network,
		Address:      cfg.Address,
		Hostname:     cfg.Hostname,
		EnableFunnel: cfg.EnableFunnel,
		EnableTLS:    cfg.EnableTLS,
		EnableHTTP:   cfg.EnableHTTP,
		StateDir:     cfg.StateDir,
		Listen:       cfg.Listen,
	}

	server, err := tsproxy.NewTailscaleProxyServer(opts)
	if err != nil {
		slog.Error("failed to create server", "name", cfg.Name, "err", err)
		return
	}
	slog.Info("starting server", "name", cfg.Name)
	server.Run()
	slog.Info("server stopped", "name", cfg.Name)
}
