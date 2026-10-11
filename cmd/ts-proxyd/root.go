package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lucasew/ts-proxy/pkg/config"
	"gopkg.in/yaml.v3"
)

var (
	// ErrNoServers is returned when the config has no servers to run.
	ErrNoServers = errors.New("no servers configured")
	// ErrCLIFlags is returned when a command runs without the parsed flag bag.
	ErrCLIFlags = errors.New("missing cli flags")
)

func flagsFrom(ctx context.Context) (cliFlags, error) {
	flags, ok := cmd.Lookup[cliFlags](ctx, "cli")
	if !ok {
		return cliFlags{}, ErrCLIFlags
	}
	return flags, nil
}

func loadConfig(flags cliFlags) (*config.Config, error) {
	path, err := resolveConfigPath(flags.ConfigFile.Value())
	if err != nil {
		return nil, err
	}

	var cfg config.Config
	if path != "" {
		slog.Info("using config file", "path", path)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing config: %w", err)
		}
	}

	if flags.StateDir.ArgSet() {
		cfg.StateDir = flags.StateDir.Value()
	}
	if flags.StopOnFail.ArgSet() {
		cfg.StopOnFail = flags.StopOnFail.Value()
	}
	if flags.StatusListen.ArgSet() {
		cfg.StatusListen = flags.StatusListen.Value()
	}

	// Defaults first so missing fields are filled before expansion runs.
	cfg.SetDefaults()
	if err := cfg.ExpandEnv(); err != nil {
		return nil, fmt.Errorf("expanding environment variables: %w", err)
	}
	// ExpandEnv treats a set-but-empty env var as success and substitutes "".
	// Re-apply defaults so empty expansions fall back instead of shipping a
	// blank state_dir / hostname / listen into Validate and runtime.
	cfg.SetDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	return &cfg, nil
}

// resolveConfigPath returns the YAML file to read.
// An empty explicit path searches the default directories. No file is not an
// error: loadConfig then applies defaults. An explicit path is returned as
// given; loadConfig reports a missing file or a directory.
func resolveConfigPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	for _, dir := range defaultConfigPaths() {
		path := filepath.Join(dir, "ts-proxy.yaml")
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return "", fmt.Errorf("reading config file: %w", err)
		}
		if info.IsDir() {
			continue
		}
		return path, nil
	}
	return "", nil
}

// defaultConfigPaths is the search order when --config is not set.
// The working directory is first so a local file wins over /etc and the user
// config dir.
//
// The user config dir comes from os.UserConfigDir (XDG_CONFIG_HOME on Unix,
// %AppData% on Windows). A hard-coded "$HOME/.config" path does not expand on
// Windows and ignores XDG_CONFIG_HOME.
func defaultConfigPaths() []string {
	paths := []string{"."}
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		paths = append(paths, filepath.Join(dir, "ts-proxy"))
	}
	paths = append(paths, "/etc/ts-proxy")
	return paths
}
