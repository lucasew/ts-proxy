package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/lucasew/ts-proxy/pkg/config"
	"github.com/lucasew/ts-proxy/pkg/status"
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
	require.Len(t, servers, 2)
	require.Equal(t, "api", servers[0].Name())
	require.Equal(t, "web", servers[1].Name())
	require.Equal(t, "tskey-test", servers[0].opts.AuthKey)
	require.Equal(t, "tskey-test", servers[1].opts.AuthKey)
	require.Equal(t, "/var/lib/ts-proxy/api", servers[0].opts.StateDir)
	require.Equal(t, "/var/lib/ts-proxy/web", servers[1].opts.StateDir)
}

func TestNewSupervisorPassesForward(t *testing.T) {
	cfg := &config.Config{
		StateDir: "/var/lib/ts-proxy",
		Servers: map[string]config.ServerConfig{
			"gremio": {
				Hostname: "gremio",
				Forward:  "127.0.0.2",
			},
		},
	}
	sup := NewSupervisor(cfg)
	servers := sup.Servers()
	require.Len(t, servers, 1)
	require.Equal(t, "127.0.0.2", servers[0].opts.Forward)
	require.Contains(t, sup.DisplayAuthenticated(), "[forward: 127.0.0.2]")
}

func TestSnapshotHealthFollowsRunningState(t *testing.T) {
	cfg := &config.Config{
		StateDir: "/var/lib/ts-proxy",
		Servers: map[string]config.ServerConfig{
			"web": {
				Hostname: "my-web",
				Handlers: []config.HandlerConfig{
					{Type: "tcp", Listen: ":22", UpstreamAddress: "127.0.0.1:1", UpstreamNetwork: "tcp"},
				},
			},
		},
	}
	sup := NewSupervisor(cfg)
	h := status.NewHandler(sup.Snapshot, sup.bus)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code, "body %s", rec.Body)

	sup.servers[0].mustTransition(StateStarting)
	sup.servers[0].mustTransition(StateRunning)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	require.Equal(t, http.StatusOK, rec.Code, "body %s", rec.Body)
	body := rec.Body.String()
	// Encoding may or may not insert a space after the colon.
	require.True(t,
		strings.Contains(body, `"name":"web"`) || strings.Contains(body, `"name": "web"`),
		"body = %s", body,
	)
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
	require.True(t, strings.HasPrefix(out, "web (my-web)\n"), "got %q", out)
	for _, part := range []string{":80", "HTTP", "127.0.0.1:8080"} {
		require.Contains(t, out, part)
	}
}
