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
	userIdx := -1
	for i, p := range paths {
		if p == "/etc/ts-proxy" {
			etcIdx = i
		}
		// Real absolute path — never a literal "$HOME/..." or "$XDG_..." placeholder.
		if strings.Contains(p, "$HOME") || strings.Contains(p, "$XDG_") {
			t.Errorf("path %q still contains literal env placeholder; want os.UserConfigDir resolution", p)
		}
	}
	if etcIdx < 0 {
		t.Fatalf("paths = %v, want /etc/ts-proxy", paths)
	}

	userCfg, err := os.UserConfigDir()
	if err != nil || userCfg == "" {
		// No user config dir available: only cwd + /etc is fine.
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
	// User config before /etc so a user file still beats the system file.
	if userIdx > etcIdx {
		t.Errorf("user path index %d after /etc index %d; user config should win over system", userIdx, etcIdx)
	}
	if !filepath.IsAbs(wantUser) {
		t.Fatalf("resolved user config path %q is not absolute", wantUser)
	}
}

// TestDefaultConfigPathsHonorsXDGConfigHome ensures a custom XDG_CONFIG_HOME
// is used instead of hard-coding $HOME/.config (XDG Base Directory Spec).
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
	// Must not also inject a stale $HOME/.config/ts-proxy when XDG is set.
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
