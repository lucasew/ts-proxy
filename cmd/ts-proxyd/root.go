package main

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/lucasew/ts-proxy/pkg/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "ts-proxyd",
	Short: "Tailscale reverse proxy server",
	Long:  "ts-proxyd exposes services to a Tailnet (and optionally the Internet via Funnel) using Tailscale tsnet.",
	// main prints a single "Error: …" line; avoid cobra duplicating it and
	// dumping full usage on routine config mistakes (use --help for usage).
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./ts-proxy.yaml, then $HOME/.config/ts-proxy/, then /etc/ts-proxy/)")
	rootCmd.PersistentFlags().String("state-dir", "", "base state directory (default /var/lib/ts-proxy)")
	rootCmd.PersistentFlags().Bool("stop-on-fail", false, "stop all servers if any one fails")

	if err := viper.BindPFlag("state_dir", rootCmd.PersistentFlags().Lookup("state-dir")); err != nil {
		panic(fmt.Errorf("binding state-dir flag: %w", err))
	}
	if err := viper.BindPFlag("stop_on_fail", rootCmd.PersistentFlags().Lookup("stop-on-fail")); err != nil {
		panic(fmt.Errorf("binding stop-on-fail flag: %w", err))
	}
}

// defaultConfigPaths is the search order when --config is not set.
// Viper uses the first ts-proxy.yaml it finds; put the working directory
// first so a local file is not shadowed by /etc or the home config dir
// (matches README and common CLI expectation).
func defaultConfigPaths() []string {
	return []string{
		".",
		"$HOME/.config/ts-proxy",
		"/etc/ts-proxy",
	}
}

// initConfig wires viper search paths and env bindings. Reading the file is
// deferred to loadConfig so failures surface as normal cobra command errors
// instead of os.Exit from OnInitialize.
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("ts-proxy")
		viper.SetConfigType("yaml")
		for _, p := range defaultConfigPaths() {
			viper.AddConfigPath(p)
		}
	}

	viper.SetEnvPrefix("TS_PROXY")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()
}

func loadConfig() (*config.Config, error) {
	if err := viper.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			// Explicit --config path missing, unreadable, or invalid YAML:
			// this is a user-facing configuration error, not an unexpected
			// runtime fault for ReportError.
			return nil, fmt.Errorf("reading config file: %w", err)
		}
	} else {
		slog.Info("using config file", "path", viper.ConfigFileUsed())
	}

	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.SetDefaults()
	if err := cfg.ExpandEnv(); err != nil {
		return nil, fmt.Errorf("expanding environment variables: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	return &cfg, nil
}
