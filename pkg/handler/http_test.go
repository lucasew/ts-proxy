package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

func TestHandleRedirect(t *testing.T) {
	tests := []struct {
		name         string
		hostname     string
		enableTLS    bool
		reqHost      string
		target       string
		wantRedirect bool
		wantLocation string
	}{
		{
			name:         "wrong host redirects https",
			hostname:     "app.example.ts.net",
			enableTLS:    true,
			reqHost:      "wrong.example.ts.net",
			target:       "/foo?x=1",
			wantRedirect: true,
			wantLocation: "https://app.example.ts.net/foo?x=1",
		},
		{
			name:         "wrong host with port redirects http",
			hostname:     "app.example.ts.net",
			enableTLS:    false,
			reqHost:      "wrong.example.ts.net:8080",
			target:       "/bar",
			wantRedirect: true,
			wantLocation: "http://app.example.ts.net/bar",
		},
		{
			name:         "matching host does not redirect",
			hostname:     "app.example.ts.net",
			enableTLS:    true,
			reqHost:      "app.example.ts.net",
			target:       "/ok",
			wantRedirect: false,
		},
		{
			name:         "matching host with port does not redirect",
			hostname:     "app.example.ts.net",
			enableTLS:    true,
			reqHost:      "app.example.ts.net:443",
			target:       "/ok",
			wantRedirect: false,
		},
		{
			name:         "empty host does not redirect",
			hostname:     "app.example.ts.net",
			enableTLS:    true,
			reqHost:      "",
			target:       "/ok",
			wantRedirect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHTTP(HTTPOptions{
				Hostname:  tt.hostname,
				EnableTLS: tt.enableTLS,
			})
			// Server-style request: path-only URL, host only on Request.Host.
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			req.Host = tt.reqHost
			if req.URL.Host != "" {
				t.Fatalf("precondition: URL.Host = %q, want empty for server-style request", req.URL.Host)
			}

			rec := httptest.NewRecorder()
			got := h.handleRedirect(rec, req)
			if got != tt.wantRedirect {
				t.Fatalf("handleRedirect = %v, want %v", got, tt.wantRedirect)
			}
			if !tt.wantRedirect {
				return
			}
			if rec.Code != http.StatusMovedPermanently {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusMovedPermanently)
			}
			if loc := rec.Header().Get("Location"); loc != tt.wantLocation {
				t.Errorf("Location = %q, want %q", loc, tt.wantLocation)
			}
		})
	}
}

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
