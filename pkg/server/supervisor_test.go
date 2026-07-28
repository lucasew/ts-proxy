package server

import (
	"strings"
	"testing"

	"github.com/lucasew/ts-proxy/pkg/config"
)

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
