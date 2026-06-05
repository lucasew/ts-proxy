package tsproxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

func TestSetTailscaleHeadersSanitization(t *testing.T) {
	// Create a dummy user info
	userInfo := &apitype.WhoIsResponse{
		UserProfile: &tailcfg.UserProfile{
			LoginName:     "user@example.com",
			DisplayName:   "User Name",
			ProfilePicURL: "http://example.com/pic.jpg",
		},
	}

	tests := []struct {
		name           string
		initialHeaders map[string]string
		wantHeaders    map[string]string // Headers that must exist with specific value
		missingHeaders []string          // Headers that must NOT exist (in any casing/format)
	}{
		{
			name: "Sanitize underscore spoofing",
			initialHeaders: map[string]string{
				"Tailscale_User_Login": "attacker@example.com",
			},
			wantHeaders: map[string]string{
				"Tailscale-User-Login": "user@example.com",
			},
			missingHeaders: []string{
				"Tailscale_User_Login",
				"Tailscale_user_login",
			},
		},
		{
			name: "Sanitize mixed case underscore spoofing",
			initialHeaders: map[string]string{
				"TAILSCALE_USER_LOGIN": "attacker@example.com",
			},
			wantHeaders: map[string]string{
				"Tailscale-User-Login": "user@example.com",
			},
			missingHeaders: []string{
				"Tailscale_user_login",
			},
		},
		{
			name: "Sanitize case insensitive dash spoofing",
			initialHeaders: map[string]string{
				"tailscale-user-login": "attacker@example.com",
			},
			wantHeaders: map[string]string{
				"Tailscale-User-Login": "user@example.com",
			},
			missingHeaders: []string{}, // Go's Header.Del handles canonical forms, so this is just a sanity check
		},
		{
			name: "Sanitize other headers",
			initialHeaders: map[string]string{
				"Tailscale_User_Name": "Attacker Name",
			},
			wantHeaders: map[string]string{
				"Tailscale-User-Name": "User Name",
			},
			missingHeaders: []string{
				"Tailscale_User_Name",
				"Tailscale_user_name",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
			for k, v := range tt.initialHeaders {
				req.Header.Set(k, v)
			}

			setTailscaleHeaders(req, userInfo)

			// Check wanted headers
			for k, v := range tt.wantHeaders {
				if got := req.Header.Get(k); got != v {
					t.Errorf("Header %q = %q, want %q", k, got, v)
				}
			}

			// Check missing headers
			// We iterate through the actual headers map to catch non-canonical keys
			for h := range req.Header {
				for _, missing := range tt.missingHeaders {
					// We can check exact match or normalized match.
					// Since we want to ensure *specifically* the underscore versions are gone:
					if h == missing {
						t.Errorf("Header %q should have been deleted", h)
					}
					// Also check if the 'missing' target matches the header key in canonical form
					// (though our main concern is the ones that bypass canonicalization)
				}
			}
		})
	}
}

func TestEnrichHeadersSanitization(t *testing.T) {
	// Instead of instantiating the full proxy server (which requires a running tsnet instance),
	// we will manually create a mock object that implements what's needed for the test
	// Actually, enrichHeaders only depends on tps.server.options.EnableTLS and tps.server.Hostname()

	// mock TailscaleHTTPProxyServer
	options := TailscaleProxyServerOptions{
		Hostname:  "test-proxy",
		EnableTLS: true,
		Address:   "127.0.0.1:80",
	}
	baseServer := &TailscaleProxyServer{
		options: options,
		// We actually cannot easily mock tsnet.Server.CertDomains.
		// Wait, Hostname() implementation does `for _, domain := range tps.server.CertDomains()`.
		// But if `tps.server` is nil or not started, it panics?
		// Actually CertDomains requires `s.getBackend()`. If s is just a struct, it crashes on `s.mu.Lock()` or similar if it's nil. Let me check the crash stack.
	}
	httpServer := &TailscaleHTTPProxyServer{
		server: baseServer,
	}

	userInfo := &apitype.WhoIsResponse{
		UserProfile: &tailcfg.UserProfile{
			LoginName:     "user@example.com",
			DisplayName:   "User Name",
			ProfilePicURL: "http://example.com/pic.jpg",
		},
	}

	tests := []struct {
		name           string
		initialHeaders map[string]string
		wantHeaders    map[string]string
		missingHeaders []string
	}{
		{
			name: "Sanitize X-Forwarded underscore spoofing",
			initialHeaders: map[string]string{
				"X_Forwarded_Proto": "http",
				"X_Forwarded_Host":  "evil.com",
			},
			wantHeaders: map[string]string{
				"X-Forwarded-Proto": "https",
				"X-Forwarded-Host":  "test-proxy",
			},
			missingHeaders: []string{
				"X_Forwarded_Proto",
				"X_Forwarded_Host",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
			for k, v := range tt.initialHeaders {
				req.Header[k] = []string{v} // bypass canonicalization of Add/Set
			}

			httpServer.enrichHeaders(req, userInfo)

			for k, v := range tt.wantHeaders {
				if got := req.Header.Get(k); got != v {
					t.Errorf("Header %q = %q, want %q", k, got, v)
				}
			}

			for h := range req.Header {
				for _, missing := range tt.missingHeaders {
					if h == missing {
						t.Errorf("Header %q should have been deleted", h)
					}
				}
			}
		})
	}
}
