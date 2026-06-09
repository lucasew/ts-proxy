package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/lucasew/ts-proxy/pkg/server"
	"github.com/spf13/cobra"
)

var dryRun bool

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the ts-proxy server",
	Long:  "Start the Tailscale proxy server that supervises all configured services.",
	RunE:  runServer,
}

func init() {
	serverCmd.Flags().BoolVar(&dryRun, "dry-run", false, "authenticate servers and display structure, then exit")
	rootCmd.AddCommand(serverCmd)
}

func runServer(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	if len(cfg.Servers) == 0 {
		return fmt.Errorf("no servers configured")
	}

	fmt.Fprint(os.Stderr, "Configured servers:\n")
	fmt.Fprint(os.Stderr, cfg.DisplayString())
	fmt.Fprintln(os.Stderr)

	sup := server.NewSupervisor(cfg)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if dryRun {
		slog.Info("dry-run mode: authenticating servers...")
		if err := sup.StartAll(ctx); err != nil {
			return fmt.Errorf("dry-run auth: %w", err)
		}
		defer sup.CloseAll()

		fmt.Fprintln(os.Stderr)
		fmt.Fprint(os.Stderr, "Authenticated servers:\n")
		fmt.Fprint(os.Stderr, sup.DisplayAuthenticated())
		return nil
	}

	return sup.Run(ctx)
}
