package config

import (
	"os"
	"strings"
	"testing"
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
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSlug(%q) error = %v, wantErr %v", tt.slug, err, tt.wantErr)
			}
		})
	}
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
		wantErr string
	}{
		{
			name:    "valid config",
			modify:  func(c *Config) {},
			wantErr: "",
		},
		{
			name: "invalid server slug",
			modify: func(c *Config) {
				c.Servers["bad-name"] = c.Servers["web"]
				delete(c.Servers, "web")
			},
			wantErr: "must contain only letters, numbers, and underscores",
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
			wantErr: "must contain only letters, numbers, and underscores",
		},
		{
			name: "missing token reference",
			modify: func(c *Config) {
				srv := c.Servers["web"]
				srv.Token = "nonexistent"
				c.Servers["web"] = srv
			},
			wantErr: "undefined token",
		},
		{
			name: "no handlers",
			modify: func(c *Config) {
				srv := c.Servers["web"]
				srv.Handlers = nil
				c.Servers["web"] = srv
			},
			wantErr: "no handlers defined",
		},
		{
			name: "unknown handler type",
			modify: func(c *Config) {
				srv := c.Servers["web"]
				srv.Handlers[0].Type = "grpc"
				c.Servers["web"] = srv
			},
			wantErr: "unknown type",
		},
		{
			name: "missing listen",
			modify: func(c *Config) {
				srv := c.Servers["web"]
				srv.Handlers[0].Listen = ""
				c.Servers["web"] = srv
			},
			wantErr: "listen address is required",
		},
		{
			name: "missing upstream_address",
			modify: func(c *Config) {
				srv := c.Servers["web"]
				srv.Handlers[0].UpstreamAddress = ""
				c.Servers["web"] = srv
			},
			wantErr: "upstream_address is required",
		},
		{
			name: "duplicate listen address",
			modify: func(c *Config) {
				srv := c.Servers["web"]
				srv.Handlers = append(srv.Handlers, HandlerConfig{
					Type: "tcp", Listen: ":80", UpstreamAddress: "localhost:9090", UpstreamNetwork: "tcp",
				})
				c.Servers["web"] = srv
			},
			wantErr: "duplicate listen address",
		},
		{
			name: "empty token ref is allowed",
			modify: func(c *Config) {
				srv := c.Servers["web"]
				srv.Token = ""
				c.Servers["web"] = srv
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.modify(&cfg)
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
			}
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

	if cfg.StateDir != "/var/lib/ts-proxy" {
		t.Errorf("StateDir = %q, want /var/lib/ts-proxy", cfg.StateDir)
	}

	srv := cfg.Servers["myapp"]
	if srv.Hostname != "myapp" {
		t.Errorf("Hostname = %q, want myapp", srv.Hostname)
	}

	if srv.Handlers[0].Listen != ":80" {
		t.Errorf("HTTP handler listen = %q, want :80", srv.Handlers[0].Listen)
	}
	if srv.Handlers[0].UpstreamNetwork != "tcp" {
		t.Errorf("HTTP handler upstream_network = %q, want tcp", srv.Handlers[0].UpstreamNetwork)
	}

	if srv.Handlers[1].Listen != ":443" {
		t.Errorf("HTTPS handler listen = %q, want :443", srv.Handlers[1].Listen)
	}

	if srv.Handlers[2].Listen != ":2222" {
		t.Errorf("TCP handler listen = %q, want :2222", srv.Handlers[2].Listen)
	}
}

func TestExpandEnv(t *testing.T) {
	os.Setenv("TEST_TS_KEY", "tskey-test-value")
	defer os.Unsetenv("TEST_TS_KEY")

	cfg := Config{
		Tokens: map[string]TokenConfig{
			"default": {AuthKey: "${TEST_TS_KEY}"},
			"literal": {AuthKey: "tskey-literal"},
		},
	}
	cfg.ExpandEnv()

	if cfg.Tokens["default"].AuthKey != "tskey-test-value" {
		t.Errorf("default auth_key = %q, want tskey-test-value", cfg.Tokens["default"].AuthKey)
	}
	if cfg.Tokens["literal"].AuthKey != "tskey-literal" {
		t.Errorf("literal auth_key = %q, want tskey-literal", cfg.Tokens["literal"].AuthKey)
	}
}

func TestDisplayString(t *testing.T) {
	cfg := validConfig()
	s := cfg.DisplayString()
	if !strings.Contains(s, "web") {
		t.Error("DisplayString should contain server name")
	}
	if !strings.Contains(s, ":80") {
		t.Error("DisplayString should contain listen address")
	}
	if !strings.Contains(s, "HTTP") {
		t.Error("DisplayString should contain handler type")
	}
	if !strings.Contains(s, "localhost:8080") {
		t.Error("DisplayString should contain upstream address")
	}
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
	if cfg.Servers["myapp"].Handlers[0].Listen != "" {
		t.Errorf("TCP handler should not get default listen, got %q", cfg.Servers["myapp"].Handlers[0].Listen)
	}

	// Validation should catch it
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error for TCP handler without listen")
	}
	if !strings.Contains(err.Error(), "listen address is required") {
		t.Errorf("expected 'listen address is required', got %q", err.Error())
	}
}

// Regression: ExpandEnv with unset env var returns empty string
func TestExpandEnvUnset(t *testing.T) {
	os.Unsetenv("DEFINITELY_NOT_SET_12345")
	cfg := Config{
		Tokens: map[string]TokenConfig{
			"default": {AuthKey: "${DEFINITELY_NOT_SET_12345}"},
		},
	}
	cfg.ExpandEnv()

	if cfg.Tokens["default"].AuthKey != "" {
		t.Errorf("unset env var should expand to empty string, got %q", cfg.Tokens["default"].AuthKey)
	}
}

// Regression: empty config should pass validation (no servers is valid config)
func TestEmptyConfigValidation(t *testing.T) {
	cfg := Config{}
	cfg.SetDefaults()
	if err := cfg.Validate(); err != nil {
		t.Errorf("empty config should be valid, got: %v", err)
	}
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
	if err := cfg.Validate(); err != nil {
		t.Errorf("multiple servers sharing one token should be valid, got: %v", err)
	}
}

// Regression: handler with funnel but not TLS should still default listen to :443
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

	if cfg.Servers["myapp"].Handlers[0].Listen != ":443" {
		t.Errorf("funnel handler should default to :443, got %q", cfg.Servers["myapp"].Handlers[0].Listen)
	}
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
	if !strings.Contains(s, "TLS") {
		t.Error("DisplayString should show TLS flag")
	}
	if !strings.Contains(s, "Funnel") {
		t.Error("DisplayString should show Funnel flag")
	}
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
	if len(names) != 3 {
		t.Fatalf("expected 3 names, got %d", len(names))
	}
	if names[0] != "alpha" || names[1] != "bravo" || names[2] != "charlie" {
		t.Errorf("expected sorted names, got %v", names)
	}
}
