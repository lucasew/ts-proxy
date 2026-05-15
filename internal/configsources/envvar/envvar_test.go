package envvar

import (
	"testing"
)

func TestSplitSuffix(t *testing.T) {
	tests := []struct {
		input      string
		wantName   string
		wantOption string
	}{
		{"WEBAPP_ADDRESS", "WEBAPP", "ADDRESS"},
		{"API_HOSTNAME", "API", "HOSTNAME"},
		{"MY_APP_FUNNEL", "MY_APP", "FUNNEL"},
		{"X_TLS", "X", "TLS"},
		{"NOTSUFFIX", "", ""},
		{"_ADDRESS", "", ""},
		{"WEBAPP_STATEDIR", "WEBAPP", "STATEDIR"},
		{"WEBAPP_RAW", "WEBAPP", "RAW"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			name, option := splitSuffix(tt.input)
			if name != tt.wantName || option != tt.wantOption {
				t.Errorf("splitSuffix(%q) = (%q, %q), want (%q, %q)",
					tt.input, name, option, tt.wantName, tt.wantOption)
			}
		})
	}
}

func TestIsTruthy(t *testing.T) {
	for _, v := range []string{"1", "true", "True", "TRUE", "yes", "YES", "on", "ON"} {
		if !isTruthy(v) {
			t.Errorf("isTruthy(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"0", "false", "", "no", "off", "whatever"} {
		if isTruthy(v) {
			t.Errorf("isTruthy(%q) = true, want false", v)
		}
	}
}

func TestDiscoverFromEnv(t *testing.T) {
	t.Setenv("TSPROXY_SVC1_ADDRESS", "localhost:3000")
	t.Setenv("TSPROXY_SVC1_HOSTNAME", "svc1host")
	t.Setenv("TSPROXY_SVC1_FUNNEL", "true")
	t.Setenv("TSPROXY_SVC2_ADDRESS", "localhost:9090")
	t.Setenv("TSPROXY_SVC2_TLS", "1")
	t.Setenv("TSPROXY_SVC2_RAW", "true")
	// No address → should be skipped
	t.Setenv("TSPROXY_NOADDR_HOSTNAME", "ghost")

	s := New()
	configs, err := s.Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(configs) != 2 {
		t.Fatalf("got %d configs, want 2", len(configs))
	}

	// Find by name
	byName := map[string]int{}
	for i, c := range configs {
		byName[c.Name] = i
	}

	idx, ok := byName["svc1"]
	if !ok {
		t.Fatal("missing svc1")
	}
	c := configs[idx]
	if c.Address != "localhost:3000" {
		t.Errorf("svc1 address = %q", c.Address)
	}
	if c.Hostname != "svc1host" {
		t.Errorf("svc1 hostname = %q", c.Hostname)
	}
	if !c.EnableFunnel {
		t.Error("svc1 funnel should be true")
	}
	if !c.EnableHTTP {
		t.Error("svc1 http should default to true")
	}

	idx, ok = byName["svc2"]
	if !ok {
		t.Fatal("missing svc2")
	}
	c = configs[idx]
	if c.Address != "localhost:9090" {
		t.Errorf("svc2 address = %q", c.Address)
	}
	if !c.EnableTLS {
		t.Error("svc2 tls should be true")
	}
	if c.EnableHTTP {
		t.Error("svc2 http should be false (raw=true)")
	}
}
