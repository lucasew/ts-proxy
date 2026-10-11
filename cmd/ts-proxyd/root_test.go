package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lewtec/lewkit/x/cmd"
	"github.com/lucasew/ts-proxy/pkg/config"
)

func TestDefaultConfigPathsPreferWorkingDirectory(t *testing.T) {
	paths := defaultConfigPaths()
	if len(paths) < 2 {
		t.Fatalf("defaultConfigPaths() len = %d, want at least cwd and /etc", len(paths))
	}
	if paths[0] != "." {
		t.Errorf("first search path = %q, want %q so cwd wins over system paths", paths[0], ".")
	}

	etcIdx := -1
	userIdx := -1
	for i, p := range paths {
		if p == "/etc/ts-proxy" {
			etcIdx = i
		}
		if strings.Contains(p, "$HOME") || strings.Contains(p, "$XDG_") {
			t.Errorf("path %q still contains literal env placeholder; want os.UserConfigDir resolution", p)
		}
	}
	if etcIdx < 0 {
		t.Fatalf("paths = %v, want /etc/ts-proxy", paths)
	}

	userCfg, err := os.UserConfigDir()
	if err != nil || userCfg == "" {
		if len(paths) != 2 {
			t.Fatalf("paths = %v, want [., /etc/ts-proxy] when UserConfigDir fails", paths)
		}
		return
	}
	wantUser := filepath.Join(userCfg, "ts-proxy")
	for i, p := range paths {
		if p == wantUser {
			userIdx = i
			break
		}
	}
	if userIdx < 0 {
		t.Fatalf("paths = %v, want user config dir %q", paths, wantUser)
	}
	if userIdx > etcIdx {
		t.Errorf("user path index %d after /etc index %d; user config should win over system", userIdx, etcIdx)
	}
	if !filepath.IsAbs(wantUser) {
		t.Fatalf("resolved user config path %q is not absolute", wantUser)
	}
}

func TestDefaultConfigPathsHonorsXDGConfigHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	paths := defaultConfigPaths()
	want := filepath.Join(xdg, "ts-proxy")
	found := false
	for _, p := range paths {
		if p == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("paths = %v, want entry %q from XDG_CONFIG_HOME", paths, want)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		stale := filepath.Join(home, ".config", "ts-proxy")
		if stale == want {
			return
		}
		for _, p := range paths {
			if p == stale {
				t.Errorf("paths include hard-coded home config %q while XDG_CONFIG_HOME=%q", stale, xdg)
			}
		}
	}
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
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("TS_PROXY_TEST_EMPTY_STATE", "")
	t.Setenv("TS_PROXY_TEST_EMPTY_HOST", "")
	t.Setenv("TS_PROXY_TEST_EMPTY_STATUS", "")

	got, err := cmd.Parse[root]("--config", cfgPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	cfg, err := loadConfig(got.Flags)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.StateDir != "/var/lib/ts-proxy" {
		t.Errorf("StateDir = %q, want default /var/lib/ts-proxy after empty env expansion", cfg.StateDir)
	}
	if cfg.StatusListen != config.DefaultStatusListen {
		t.Errorf("StatusListen = %q, want %s", cfg.StatusListen, config.DefaultStatusListen)
	}
	srv, ok := cfg.Servers["web"]
	if !ok {
		t.Fatal("missing server web")
	}
	if srv.Hostname != "web" {
		t.Errorf("Hostname = %q, want server name %q after empty env expansion", srv.Hostname, "web")
	}
	if len(srv.Handlers) != 1 || srv.Handlers[0].Listen != ":80" {
		t.Errorf("HTTP handler listen = %#v, want default :80", srv.Handlers)
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
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t.Setenv("TS_PROXY_STATE_DIR", "/from/env")
	t.Setenv("TS_PROXY_STOP_ON_FAIL", "true")
	t.Setenv("TS_PROXY_STATUS_LISTEN", "127.0.0.1:9")

	fromEnv, err := cmd.Parse[root]("--config", cfgPath)
	if err != nil {
		t.Fatalf("parse env: %v", err)
	}
	cfg, err := loadConfig(fromEnv.Flags)
	if err != nil {
		t.Fatalf("loadConfig env: %v", err)
	}
	if cfg.StateDir != "/from/env" || !cfg.StopOnFail || cfg.StatusListen != "127.0.0.1:9" {
		t.Fatalf("env override = state %q stop %v status %q", cfg.StateDir, cfg.StopOnFail, cfg.StatusListen)
	}

	t.Setenv("TS_PROXY_STOP_ON_FAIL", "false")
	stopped, err := cmd.Parse[root]("--config", cfgPath, "--state-dir", "/from/flag", "--status-listen", "127.0.0.1:8")
	if err != nil {
		t.Fatalf("parse flag: %v", err)
	}
	cfg, err = loadConfig(stopped.Flags)
	if err != nil {
		t.Fatalf("loadConfig flag: %v", err)
	}
	if cfg.StateDir != "/from/flag" {
		t.Errorf("StateDir = %q, want /from/flag", cfg.StateDir)
	}
	if cfg.StopOnFail {
		t.Error("StopOnFail = true, want false from env")
	}
	if cfg.StatusListen != "127.0.0.1:8" {
		t.Errorf("StatusListen = %q, want 127.0.0.1:8", cfg.StatusListen)
	}
}

func TestLoadConfigRejectsBadStatusListen(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ts-proxy.yaml")
	if err := os.WriteFile(cfgPath, []byte("status_listen: nope\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	got, err := cmd.Parse[root]("--config", cfgPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = loadConfig(got.Flags)
	if !errors.Is(err, config.ErrStatusListen) {
		t.Fatalf("loadConfig err = %v, want ErrStatusListen", err)
	}
}

func TestParseServerFlags(t *testing.T) {
	got, err := cmd.Parse[root]("server", "--config", "ts-proxy.yaml", "--dry-run")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Server == nil || !got.Server.DryRun.Value() {
		t.Fatalf("dry-run not set: %#v", got.Server)
	}
	if got.Flags.ConfigFile.Value() != "ts-proxy.yaml" {
		t.Errorf("config = %q", got.Flags.ConfigFile.Value())
	}
}

func TestUsageListsCommands(t *testing.T) {
	text, err := cmd.Usage[cmd.App[root]]("ts-proxyd")
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	for _, want := range []string{"server", "config", "--state-dir", "--stop-on-fail", "--status-listen", "--config", "127.0.0.1:8090"} {
		if !strings.Contains(text, want) {
			t.Errorf("usage missing %q\n%s", want, text)
		}
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
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	app, err := cmd.Parse[cmd.App[root]]("--config", cfgPath, "config")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	runErr := app.Run(t.Context())
	if err := w.Close(); err != nil {
		t.Fatalf("close stdout: %v", err)
	}
	out, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}
	if runErr != nil {
		t.Fatalf("config command: %v", runErr)
	}
	if !strings.Contains(string(out), "status_listen: 127.0.0.1:8090") {
		t.Fatalf("stdout = %q, want status_listen default", out)
	}
}
