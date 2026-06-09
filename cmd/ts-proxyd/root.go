package main

import (
	"fmt"
	"log/slog"
	"os"
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
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./ts-proxy.yaml, /etc/ts-proxy/ts-proxy.yaml)")
	rootCmd.PersistentFlags().String("state-dir", "", "base state directory (default /var/lib/ts-proxy)")
	rootCmd.PersistentFlags().Bool("stop-on-fail", false, "stop all servers if any one fails")

	viper.BindPFlag("state_dir", rootCmd.PersistentFlags().Lookup("state-dir"))
	viper.BindPFlag("stop_on_fail", rootCmd.PersistentFlags().Lookup("stop-on-fail"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("ts-proxy")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("/etc/ts-proxy")
		viper.AddConfigPath("$HOME/.config/ts-proxy")
		viper.AddConfigPath(".")
	}

	viper.SetEnvPrefix("TS_PROXY")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			slog.Error("reading config file", "err", err)
			os.Exit(1)
		}
	} else {
		slog.Info("using config file", "path", viper.ConfigFileUsed())
	}
}

func loadConfig() (*config.Config, error) {
	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg.SetDefaults()
	cfg.ExpandEnv()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	return &cfg, nil
}
