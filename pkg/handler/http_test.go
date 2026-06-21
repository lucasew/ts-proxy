package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

func TestSetTailscaleHeadersSanitization(t *testing.T) {
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
			name: "sanitize underscore spoofing",
			initialHeaders: map[string]string{
				"Tailscale_User_Login": "attacker@example.com",
			},
			wantHeaders: map[string]string{
				"Tailscale-User-Login": "user@example.com",
			},
			missingHeaders: []string{
				"Tailscale_User_Login",
			},
		},
		{
			name: "sanitize mixed case underscore spoofing",
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
			name: "sanitize case insensitive dash spoofing",
			initialHeaders: map[string]string{
				"tailscale-user-login": "attacker@example.com",
			},
			wantHeaders: map[string]string{
				"Tailscale-User-Login": "user@example.com",
			},
			missingHeaders: []string{},
		},
		{
			name: "sanitize other headers",
			initialHeaders: map[string]string{
				"Tailscale_User_Name": "Attacker Name",
			},
			wantHeaders: map[string]string{
				"Tailscale-User-Name": "User Name",
			},
			missingHeaders: []string{
				"Tailscale_User_Name",
			},
		},
		{
			name:           "all tailscale headers set correctly",
			initialHeaders: map[string]string{},
			wantHeaders: map[string]string{
				"Tailscale-User-Login":       "user@example.com",
				"Tailscale-User-Name":        "User Name",
				"Tailscale-User-Profile-Pic": "http://example.com/pic.jpg",
				"Tailscale-Headers-Info":     "https://tailscale.com/s/serve-headers",
			},
			missingHeaders: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
			for k, v := range tt.initialHeaders {
				req.Header.Set(k, v)
			}

			SetTailscaleHeaders(req, userInfo)

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
