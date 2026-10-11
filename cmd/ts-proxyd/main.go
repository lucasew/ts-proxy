package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/lewtec/lewkit/x/cmd"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	app, err := cmd.Parse[cmd.App[root]](os.Args[1:]...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := app.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

type root struct {
	Flags  cliFlags   `flatten:"" ctx:"cli"`
	Server *serverCmd `cmd:"server"`
	Config *configCmd `cmd:"config"`
}

func (root) Description() string {
	return "Expose each configured server on a tailnet."
}

type cliFlags struct {
	ConfigFile cmd.StringArg `long:"config" help:"config file (default: ./ts-proxy.yaml, then the user config directory, then /etc/ts-proxy/)" default:""`
	StateDir   cmd.StringArg `long:"state-dir" help:"base state directory (default /var/lib/ts-proxy)" env:"TS_PROXY_STATE_DIR" default:""`
	StopOnFail cmd.Flag      `long:"stop-on-fail" help:"stop all servers if any one fails" env:"TS_PROXY_STOP_ON_FAIL"`
	// default matches config.DefaultStatusListen. A struct tag cannot name the const.
	StatusListen cmd.AddrArg `long:"status-listen" help:"status page and /health listen address" env:"TS_PROXY_STATUS_LISTEN" default:"127.0.0.1:8090"`
}
