package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
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
	homeIdx := -1
	for i, p := range paths {
		if p == "/etc/ts-proxy" {
			etcIdx = i
		}
		// Real absolute home path — never a literal "$HOME/..." placeholder.
		if strings.Contains(p, "$HOME") {
			t.Errorf("path %q still contains literal $HOME; want os.UserHomeDir resolution", p)
		}
	}
	if etcIdx < 0 {
		t.Fatalf("paths = %v, want /etc/ts-proxy", paths)
	}

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		// No home dir available: only cwd + /etc is fine.
		if len(paths) != 2 {
			t.Fatalf("paths = %v, want [., /etc/ts-proxy] when UserHomeDir fails", paths)
		}
		return
	}
	wantHome := filepath.Join(home, ".config", "ts-proxy")
	for i, p := range paths {
		if p == wantHome {
			homeIdx = i
			break
		}
	}
	if homeIdx < 0 {
		t.Fatalf("paths = %v, want home config dir %q", paths, wantHome)
	}
	// Home before /etc so a user config still beats the system file.
	if homeIdx > etcIdx {
		t.Errorf("home path index %d after /etc index %d; user config should win over system", homeIdx, etcIdx)
	}
	if !filepath.IsAbs(wantHome) {
		t.Fatalf("resolved home config path %q is not absolute", wantHome)
	}
}

// TestLoadConfigEmptyEnvExpansionReappliesDefaults ensures a placeholder that
// expands to "" (env var set but empty) does not leave required fields blank.
// SetDefaults runs before ExpandEnv, so it cannot see the post-expansion empty
// value unless it is called again.
func TestLoadConfigEmptyEnvExpansionReappliesDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "ts-proxy.yaml")
	content := `
state_dir: "${TS_PROXY_TEST_EMPTY_STATE}"
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

	// Isolate global viper + cfgFile used by initConfig/loadConfig.
	viper.Reset()
	t.Cleanup(viper.Reset)
	oldCfg := cfgFile
	cfgFile = cfgPath
	t.Cleanup(func() { cfgFile = oldCfg })

	initConfig()
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.StateDir != "/var/lib/ts-proxy" {
		t.Errorf("StateDir = %q, want default /var/lib/ts-proxy after empty env expansion", cfg.StateDir)
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
