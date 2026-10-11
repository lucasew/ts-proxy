package config

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSlug(t *testing.T) {
	tests := []struct {
		slug    string
		wantErr bool
	}{
		{"myservice", false},
		{"my_service", false},
		{"service123", false},
		{"My_Service_42", false},
		{"_leading", false},
		{"123numeric", false},
		{"", true},
		{"my-service", true},
		{"my.service", true},
		{"my service", true},
		{"my@service", true},
		{"path/slash", true},
	}

	for _, tt := range tests {
		t.Run(tt.slug, func(t *testing.T) {
			err := ValidateSlug(tt.slug)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

// withWeb copies the "web" server, applies edit, and stores it back.
// ServerConfig is a map value, so field writes on the copy would otherwise be lost.
func withWeb(c *Config, edit func(*ServerConfig)) {
	srv := c.Servers["web"]
	edit(&srv)
	c.Servers["web"] = srv
}

func validConfig() Config {
	return Config{
		StateDir: "/tmp/test",
		Tokens: map[string]TokenConfig{
			"default": {AuthKey: "tskey-test"},
		},
		Servers: map[string]ServerConfig{
			"web": {
				Hostname: "web",
				Token:    "default",
				Handlers: []HandlerConfig{
					{Type: "http", Listen: ":80", UpstreamAddress: "localhost:8080", UpstreamNetwork: "tcp"},
				},
			},
		},
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr error
	}{
		{
			name:   "valid config",
			modify: func(c *Config) {},
		},
		{
			name: "invalid server slug",
			modify: func(c *Config) {
				c.Servers["bad-name"] = c.Servers["web"]
				delete(c.Servers, "web")
			},
			wantErr: ErrNameInvalid,
		},
		{
			name: "invalid token slug",
			modify: func(c *Config) {
				c.Tokens["bad-token"] = c.Tokens["default"]
				delete(c.Tokens, "default")
				c.Servers["web"] = ServerConfig{
					Token:    "bad-token",
					Handlers: c.Servers["web"].Handlers,
				}
			},
			wantErr: ErrNameInvalid,
		},
		{
			name: "missing token reference",
			modify: func(c *Config) {
				withWeb(c, func(srv *ServerConfig) { srv.Token = "nonexistent" })
			},
			wantErr: ErrUndefinedToken,
		},
		{
			name: "no handlers is allowed",
			modify: func(c *Config) {
				withWeb(c, func(srv *ServerConfig) { srv.Handlers = nil })
			},
		},
		{
			name: "forward only is allowed",
			modify: func(c *Config) {
				withWeb(c, func(srv *ServerConfig) {
					srv.Handlers = nil
					srv.Forward = "127.0.0.2"
				})
			},
		},
		{
			name: "forward with port is rejected",
			modify: func(c *Config) {
				withWeb(c, func(srv *ServerConfig) { srv.Forward = "127.0.0.2:80" })
			},
			wantErr: ErrForwardHostOnly,
		},
		{
			name: "forward ipv6 is allowed",
			modify: func(c *Config) {
				withWeb(c, func(srv *ServerConfig) { srv.Forward = "::1" })
			},
		},
		{
			name: "forward hostname is allowed",
			modify: func(c *Config) {
				withWeb(c, func(srv *ServerConfig) { srv.Forward = "nas.lan" })
			},
		},
		{
			name: "forward bracketed ipv6 with port is rejected",
			modify: func(c *Config) {
				withWeb(c, func(srv *ServerConfig) { srv.Forward = "[::1]:80" })
			},
			wantErr: ErrForwardHostOnly,
		},
		{
			name: "unknown handler type",
			modify: func(c *Config) {
				withWeb(c, func(srv *ServerConfig) { srv.Handlers[0].Type = "grpc" })
			},
			wantErr: ErrUnknownHandlerType,
		},
		{
			name: "missing listen",
			modify: func(c *Config) {
				withWeb(c, func(srv *ServerConfig) { srv.Handlers[0].Listen = "" })
			},
			wantErr: ErrListenRequired,
		},
		{
			name: "missing upstream_address",
			modify: func(c *Config) {
				withWeb(c, func(srv *ServerConfig) { srv.Handlers[0].UpstreamAddress = "" })
			},
			wantErr: ErrUpstreamRequired,
		},
		{
			name: "duplicate listen address",
			modify: func(c *Config) {
				withWeb(c, func(srv *ServerConfig) {
					srv.Handlers = append(srv.Handlers, HandlerConfig{
						Type: "tcp", Listen: ":80", UpstreamAddress: "localhost:9090", UpstreamNetwork: "tcp",
					})
				})
			},
			wantErr: ErrDuplicateListen,
		},
		{
			name: "empty token ref is allowed",
			modify: func(c *Config) {
				withWeb(c, func(srv *ServerConfig) { srv.Token = "" })
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(&cfg)
			err := cfg.Validate()
			if tt.wantErr == nil {
				assert.NoError(t, err)
				return
			}
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestSetDefaults(t *testing.T) {
	cfg := Config{
		Servers: map[string]ServerConfig{
			"myapp": {
				Handlers: []HandlerConfig{
					{Type: "http", UpstreamAddress: "localhost:8080"},
					{Type: "http", UpstreamAddress: "localhost:8080", TLS: true},
					{Type: "tcp", UpstreamAddress: "localhost:22", Listen: ":2222"},
				},
			},
		},
	}
	cfg.SetDefaults()

	assert.Equal(t, "/var/lib/ts-proxy", cfg.StateDir)
	assert.Equal(t, DefaultStatusListen, cfg.StatusListen)

	srv := cfg.Servers["myapp"]
	assert.Equal(t, "myapp", srv.Hostname)
	assert.Equal(t, ":80", srv.Handlers[0].Listen)
	assert.Equal(t, "tcp", srv.Handlers[0].UpstreamNetwork)
	assert.Equal(t, ":443", srv.Handlers[1].Listen)
	assert.Equal(t, ":2222", srv.Handlers[2].Listen)
}

func TestExpandEnv(t *testing.T) {
	t.Setenv("TEST_TS_KEY", "tskey-test-value")

	cfg := Config{
		Tokens: map[string]TokenConfig{
			"default": {AuthKey: "${TEST_TS_KEY}"},
			"literal": {AuthKey: "tskey-literal"},
		},
	}
	require.NoError(t, cfg.ExpandEnv())
	assert.Equal(t, "tskey-test-value", cfg.Tokens["default"].AuthKey)
	assert.Equal(t, "tskey-literal", cfg.Tokens["literal"].AuthKey)
}

func TestExpandEnvForward(t *testing.T) {
	t.Setenv("TEST_FORWARD_HOST", "127.0.0.2")
	cfg := validConfig()
	withWeb(&cfg, func(srv *ServerConfig) { srv.Forward = "${TEST_FORWARD_HOST}" })
	require.NoError(t, cfg.ExpandEnv())
	require.Equal(t, "127.0.0.2", cfg.Servers["web"].Forward)
}

func TestDisplayStringForward(t *testing.T) {
	cfg := validConfig()
	withWeb(&cfg, func(srv *ServerConfig) { srv.Forward = "127.0.0.2" })
	require.Contains(t, cfg.DisplayString(), "[forward: 127.0.0.2]")
}

func TestDisplayString(t *testing.T) {
	cfg := validConfig()
	s := cfg.DisplayString()
	assert.Contains(t, s, "web")
	assert.Contains(t, s, ":80")
	assert.Contains(t, s, "HTTP")
	assert.Contains(t, s, "localhost:8080")
}

// Regression: TCP handler with no listen after defaults should fail validation
func TestTCPHandlerNoListenDefault(t *testing.T) {
	cfg := Config{
		StateDir: "/tmp/test",
		Servers: map[string]ServerConfig{
			"myapp": {
				Handlers: []HandlerConfig{
					{Type: "tcp", UpstreamAddress: "localhost:22"},
				},
			},
		},
	}
	cfg.SetDefaults()

	// TCP handlers should NOT get a default listen address
	assert.Empty(t, cfg.Servers["myapp"].Handlers[0].Listen)
	err := cfg.Validate()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrListenRequired)
}

// ExpandEnv must error when a referenced variable is not set (prevents
// silent empty auth keys in automated/container environments).
func TestExpandEnvUnset(t *testing.T) {
	require.NoError(t, os.Unsetenv("DEFINITELY_NOT_SET_12345"))
	cfg := Config{
		Tokens: map[string]TokenConfig{
			"default": {AuthKey: "${DEFINITELY_NOT_SET_12345}"},
		},
	}
	err := cfg.ExpandEnv()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUndefinedEnvVar)
	// Diagnostic context (field path + var name) stays in the message.
	msg := err.Error()
	assert.Contains(t, msg, "DEFINITELY_NOT_SET_12345")
	assert.Contains(t, msg, "token \"default\"")
}

// Regression: empty config should pass validation (no servers is valid config)
func TestEmptyConfigValidation(t *testing.T) {
	cfg := Config{}
	cfg.SetDefaults()
	assert.NoError(t, cfg.Validate())
}

// Regression: multiple servers sharing the same token
func TestMultipleServersOneToken(t *testing.T) {
	cfg := Config{
		StateDir: "/tmp/test",
		Tokens: map[string]TokenConfig{
			"shared": {AuthKey: "tskey-shared"},
		},
		Servers: map[string]ServerConfig{
			"app1": {
				Token:    "shared",
				Handlers: []HandlerConfig{{Type: "http", Listen: ":80", UpstreamAddress: "localhost:8001", UpstreamNetwork: "tcp"}},
			},
			"app2": {
				Token:    "shared",
				Handlers: []HandlerConfig{{Type: "http", Listen: ":80", UpstreamAddress: "localhost:8002", UpstreamNetwork: "tcp"}},
			},
		},
	}
	cfg.SetDefaults()
	assert.NoError(t, cfg.Validate())
}

// Regression: handler with funnel but not TLS should still default listen to
// :443 and have TLS enabled (Funnel always terminates TLS at the edge).
func TestFunnelDefaultsTo443(t *testing.T) {
	cfg := Config{
		Servers: map[string]ServerConfig{
			"myapp": {
				Handlers: []HandlerConfig{
					{Type: "http", UpstreamAddress: "localhost:8080", Funnel: true},
				},
			},
		},
	}
	cfg.SetDefaults()

	h := cfg.Servers["myapp"].Handlers[0]
	assert.Equal(t, ":443", h.Listen)
	assert.True(t, h.TLS)
}

// Funnel without an explicit tls flag must still surface as TLS in display
// output (HTTP+TLS+Funnel), matching edge termination.
func TestFunnelImpliesTLSInDisplay(t *testing.T) {
	cfg := Config{
		Servers: map[string]ServerConfig{
			"web": {
				Hostname: "web",
				Handlers: []HandlerConfig{
					{Type: "http", UpstreamAddress: "localhost:8080", Funnel: true},
				},
			},
		},
	}
	cfg.SetDefaults()
	s := cfg.DisplayString()
	assert.Contains(t, s, "TLS")
	assert.Contains(t, s, "Funnel")
}

// Regression: DisplayString includes TLS and Funnel flags
func TestDisplayStringFlags(t *testing.T) {
	cfg := Config{
		Servers: map[string]ServerConfig{
			"web": {
				Hostname: "web",
				Handlers: []HandlerConfig{
					{Type: "http", Listen: ":443", UpstreamAddress: "localhost:8080", TLS: true, Funnel: true},
				},
			},
		},
	}
	s := cfg.DisplayString()
	assert.Contains(t, s, "TLS")
	assert.Contains(t, s, "Funnel")
}

func TestFormatHandlerSectionsAlignsColumns(t *testing.T) {
	got := FormatHandlerSections([]HandlerSection{
		{Header: "web\n", Handlers: []HandlerConfig{
			{Type: "http", Listen: ":80", UpstreamAddress: "localhost:8080"},
		}},
		{Header: "api\n", Handlers: []HandlerConfig{
			{Type: "http", Listen: ":8443", UpstreamAddress: "localhost:3000", TLS: true},
		}},
	})
	// Widths are global: ":80" pads to len(":8443"), "HTTP" pads to len("HTTP+TLS").
	require.Contains(t, got, "  :80   HTTP     -> localhost:8080")
	require.Contains(t, got, "  :8443 HTTP+TLS -> localhost:3000")
	require.True(t, strings.HasPrefix(got, "web\n"), "got:\n%s", got)
	require.Contains(t, got, "api\n")
}

func TestServerNames(t *testing.T) {
	cfg := Config{
		Servers: map[string]ServerConfig{
			"charlie": {},
			"alpha":   {},
			"bravo":   {},
		},
	}
	names := cfg.ServerNames()
	require.Len(t, names, 3)
	assert.Equal(t, []string{"alpha", "bravo", "charlie"}, names)
}
