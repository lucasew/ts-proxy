package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lucasew/ts-proxy/pkg/server"
)

type serverCmd struct {
	DryRun cmd.Flag `long:"dry-run" help:"authenticate servers and display structure, then exit"`
}

func (serverCmd) Description() string {
	return "Start the proxy and supervise every configured server."
}

func (c *serverCmd) Run(ctx context.Context) error {
	flags, err := flagsFrom(ctx)
	if err != nil {
		return err
	}
	cfg, err := loadConfig(flags)
	if err != nil {
		return err
	}
	if len(cfg.Servers) == 0 {
		return ErrNoServers
	}

	fmt.Fprint(os.Stderr, "Configured servers:\n")
	fmt.Fprint(os.Stderr, cfg.DisplayString())
	fmt.Fprintln(os.Stderr)

	sup := server.NewSupervisor(cfg)
	if c.DryRun.Value() {
		slog.Info("dry-run mode: authenticating servers...")
		defer sup.CloseAll()
		if err := sup.StartAll(ctx); err != nil {
			return fmt.Errorf("dry-run auth: %w", err)
		}
		fmt.Fprintln(os.Stderr)
		fmt.Fprint(os.Stderr, "Authenticated servers:\n")
		fmt.Fprint(os.Stderr, sup.DisplayAuthenticated())
		return nil
	}
	return sup.Run(ctx)
}
