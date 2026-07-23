package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lucasew/ts-proxy/pkg/config"
)

func TestWaitRestartCompletes(t *testing.T) {
	ctx := context.Background()
	start := time.Now()
	if !waitRestart(ctx, 20*time.Millisecond) {
		t.Fatal("waitRestart returned false, want true after delay")
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("waitRestart returned too early: %v", elapsed)
	}
}

func TestWaitRestartCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	if waitRestart(ctx, time.Hour) {
		t.Fatal("waitRestart returned true, want false after cancel")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("waitRestart ignored cancel, took %v", elapsed)
	}
}

func TestWaitRestartCancelDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		done <- waitRestart(ctx, time.Hour)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case got := <-done:
		if got {
			t.Fatal("waitRestart returned true after mid-wait cancel, want false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitRestart did not return after cancel")
	}
}

func TestNewSupervisorResolvesTokensAndStateDir(t *testing.T) {
	cfg := &config.Config{
		StateDir: "/var/lib/ts-proxy",
		Tokens: map[string]config.TokenConfig{
			"prod": {AuthKey: "tskey-test"},
		},
		Servers: map[string]config.ServerConfig{
			"web": {
				Hostname: "my-web",
				Token:    "prod",
				Handlers: []config.HandlerConfig{
					{Type: "http", Listen: ":80", UpstreamAddress: "127.0.0.1:8080"},
				},
			},
			"api": {
				Hostname: "my-api",
				Token:    "prod",
				Handlers: []config.HandlerConfig{
					{Type: "http", Listen: ":443", UpstreamAddress: "127.0.0.1:3000", TLS: true},
				},
			},
		},
	}

	sup := NewSupervisor(cfg)
	servers := sup.Servers()
	if len(servers) != 2 {
		t.Fatalf("Servers() len = %d, want 2", len(servers))
	}
	if servers[0].Name() != "api" || servers[1].Name() != "web" {
		t.Fatalf("order/names = %q, %q; want api, web", servers[0].Name(), servers[1].Name())
	}
	if servers[0].opts.AuthKey != "tskey-test" || servers[1].opts.AuthKey != "tskey-test" {
		t.Fatalf("auth keys not resolved from token: %q, %q", servers[0].opts.AuthKey, servers[1].opts.AuthKey)
	}
	if servers[0].opts.StateDir != "/var/lib/ts-proxy/api" {
		t.Fatalf("api StateDir = %q, want /var/lib/ts-proxy/api", servers[0].opts.StateDir)
	}
	if servers[1].opts.StateDir != "/var/lib/ts-proxy/web" {
		t.Fatalf("web StateDir = %q, want /var/lib/ts-proxy/web", servers[1].opts.StateDir)
	}
}

func TestDisplayAuthenticatedUsesFQDNFallback(t *testing.T) {
	cfg := &config.Config{
		StateDir: "/tmp/ts-proxy-test",
		Servers: map[string]config.ServerConfig{
			"web": {
				Hostname: "my-web",
				Handlers: []config.HandlerConfig{
					{Type: "http", Listen: ":80", UpstreamAddress: "127.0.0.1:8080"},
				},
			},
		},
	}
	sup := NewSupervisor(cfg)
	out := sup.DisplayAuthenticated()
	if !strings.HasPrefix(out, "web (my-web)\n") {
		t.Fatalf("DisplayAuthenticated = %q, want prefix %q", out, "web (my-web)\n")
	}
	for _, part := range []string{":80", "HTTP", "127.0.0.1:8080"} {
		if !strings.Contains(out, part) {
			t.Fatalf("DisplayAuthenticated missing %q: %q", part, out)
		}
	}
}
