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
