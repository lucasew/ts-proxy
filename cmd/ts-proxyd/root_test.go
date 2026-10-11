package main

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lucasew/ts-proxy/pkg/config"
)

func TestDefaultConfigPathsPreferWorkingDirectory(t *testing.T) {
	paths := defaultConfigPaths()
	require.GreaterOrEqual(t, len(paths), 2)
	assert.Equal(t, ".", paths[0])

	for _, p := range paths {
		assert.NotContains(t, p, "$HOME")
		assert.NotContains(t, p, "$XDG_")
	}
	etcIdx := slices.Index(paths, "/etc/ts-proxy")
	require.NotEqual(t, -1, etcIdx, "paths = %v", paths)

	userCfg, err := os.UserConfigDir()
	if err != nil || userCfg == "" {
		require.Len(t, paths, 2)
		return
	}
	wantUser := filepath.Join(userCfg, "ts-proxy")
	userIdx := slices.Index(paths, wantUser)
	require.NotEqual(t, -1, userIdx, "paths = %v, want %q", paths, wantUser)
	assert.LessOrEqual(t, userIdx, etcIdx, "user config should win over system")
	require.True(t, filepath.IsAbs(wantUser))
}

func TestDefaultConfigPathsHonorsXDGConfigHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	paths := defaultConfigPaths()
	want := filepath.Join(xdg, "ts-proxy")
	require.Contains(t, paths, want)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return
	}
	stale := filepath.Join(home, ".config", "ts-proxy")
	if stale == want {
		return
	}
	assert.NotContains(t, paths, stale)
}

func TestLoadConfigEmptyEnvExpansionReappliesDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ts-proxy.yaml")
	content := `
state_dir: "${TS_PROXY_TEST_EMPTY_STATE}"
status_listen: "${TS_PROXY_TEST_EMPTY_STATUS}"
servers:
  web:
    hostname: "${TS_PROXY_TEST_EMPTY_HOST}"
    handlers:
      - type: http
        upstream_address: "127.0.0.1:8080"
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))
	t.Setenv("TS_PROXY_TEST_EMPTY_STATE", "")
	t.Setenv("TS_PROXY_TEST_EMPTY_HOST", "")
	t.Setenv("TS_PROXY_TEST_EMPTY_STATUS", "")

	got, err := cmd.Parse[root]("--config", cfgPath)
	require.NoError(t, err)
	cfg, err := loadConfig(got.Flags)
	require.NoError(t, err)
	assert.Equal(t, "/var/lib/ts-proxy", cfg.StateDir)
	assert.Equal(t, config.DefaultStatusListen, cfg.StatusListen)
	srv, ok := cfg.Servers["web"]
	require.True(t, ok, "missing server web")
	assert.Equal(t, "web", srv.Hostname)
	if assert.Len(t, srv.Handlers, 1) {
		assert.Equal(t, ":80", srv.Handlers[0].Listen)
	}
}

func TestLoadConfigFlagAndEnvOverrideFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ts-proxy.yaml")
	content := `
state_dir: /from/file
stop_on_fail: true
status_listen: "127.0.0.1:1"
servers:
  web:
    handlers:
      - type: tcp
        listen: ":22"
        upstream_address: "127.0.0.1:22"
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))

	t.Setenv("TS_PROXY_STATE_DIR", "/from/env")
	t.Setenv("TS_PROXY_STOP_ON_FAIL", "true")
	t.Setenv("TS_PROXY_STATUS_LISTEN", "127.0.0.1:9")

	fromEnv, err := cmd.Parse[root]("--config", cfgPath)
	require.NoError(t, err)
	cfg, err := loadConfig(fromEnv.Flags)
	require.NoError(t, err)
	require.Equal(t, "/from/env", cfg.StateDir)
	require.True(t, cfg.StopOnFail)
	require.Equal(t, "127.0.0.1:9", cfg.StatusListen)

	t.Setenv("TS_PROXY_STOP_ON_FAIL", "false")
	stopped, err := cmd.Parse[root]("--config", cfgPath, "--state-dir", "/from/flag", "--status-listen", "127.0.0.1:8")
	require.NoError(t, err)
	cfg, err = loadConfig(stopped.Flags)
	require.NoError(t, err)
	assert.Equal(t, "/from/flag", cfg.StateDir)
	assert.False(t, cfg.StopOnFail)
	assert.Equal(t, "127.0.0.1:8", cfg.StatusListen)
}

func TestStatusListenAddrArg(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ts-proxy.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("status_listen: \"127.0.0.1:1\"\n"), 0o644))
	got, err := cmd.Parse[root]("--config", cfgPath, "--status-listen", "8090")
	require.NoError(t, err)
	cfg, err := loadConfig(got.Flags)
	require.NoError(t, err)
	require.Equal(t, ":8090", cfg.StatusListen)

	_, err = cmd.Parse[root]("--status-listen", "nope")
	require.ErrorIs(t, err, cmd.ErrInvalidArgument)
}

func TestLoadConfigRejectsBadStatusListen(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ts-proxy.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("status_listen: nope\n"), 0o644))
	got, err := cmd.Parse[root]("--config", cfgPath)
	require.NoError(t, err)
	_, err = loadConfig(got.Flags)
	require.ErrorIs(t, err, config.ErrStatusListen)
}

func TestParseServerFlags(t *testing.T) {
	got, err := cmd.Parse[root]("server", "--config", "ts-proxy.yaml", "--dry-run")
	require.NoError(t, err)
	require.NotNil(t, got.Server)
	require.True(t, got.Server.DryRun.Value())
	assert.Equal(t, "ts-proxy.yaml", got.Flags.ConfigFile.Value())
}

func TestUsageListsCommands(t *testing.T) {
	text, err := cmd.Usage[cmd.App[root]]("ts-proxyd")
	require.NoError(t, err)
	for _, want := range []string{"server", "config", "--state-dir", "--stop-on-fail", "--status-listen", "--config", "127.0.0.1:8090"} {
		assert.Contains(t, text, want)
	}
}

func TestConfigCommandPrintsResolvedStatusListen(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ts-proxy.yaml")
	content := `
servers:
  web:
    handlers:
      - type: tcp
        listen: ":22"
        upstream_address: "127.0.0.1:22"
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))
	app, err := cmd.Parse[cmd.App[root]]("--config", cfgPath, "config")
	require.NoError(t, err)

	r, w, err := os.Pipe()
	require.NoError(t, err)
	old := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	runErr := app.Run(t.Context())
	require.NoError(t, w.Close())
	out, readErr := io.ReadAll(r)
	require.NoError(t, readErr)
	require.NoError(t, runErr)
	require.Contains(t, string(out), "status_listen: 127.0.0.1:8090")
}
