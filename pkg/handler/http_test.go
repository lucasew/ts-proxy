package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

// TestHTTPUpstreamDialTimeout ensures a blackholed upstream cannot pin
// ServeHTTP forever. 192.0.2.0/24 is TEST-NET-1 (RFC 5737) and is not routed
// on the public Internet; combined with a short dial timeout this should fail
// quickly (mirrors TestHandleConnDialTimeout for TCP).
func TestHTTPUpstreamDialTimeout(t *testing.T) {
	h := NewHTTP(HTTPOptions{
		Hostname:        "app.example.ts.net",
		UpstreamNetwork: "tcp",
		UpstreamAddress: "192.0.2.1:9",
	})
	h.dialTimeout = 200 * time.Millisecond

	req := httptest.NewRequest(http.MethodGet, "http://app.example.ts.net/", nil)
	rec := httptest.NewRecorder()

	start := time.Now()
	h.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Fatalf("ServeHTTP took %v, want roughly dialTimeout", elapsed)
	}
	// ReverseProxy's default ErrorHandler responds 502 on dial failure.
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

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

func TestEnrichHeadersStripsXForwardedVariants(t *testing.T) {
	userInfo := &apitype.WhoIsResponse{
		UserProfile: &tailcfg.UserProfile{
			LoginName:     "user@example.com",
			DisplayName:   "User Name",
			ProfilePicURL: "http://example.com/pic.jpg",
		},
	}

	tests := []struct {
		name           string
		enableTLS      bool
		initialHeaders map[string]string
		wantProto      string
		wantHost       string
	}{
		{
			name:      "underscore proto and host spoofing",
			enableTLS: true,
			initialHeaders: map[string]string{
				"X_Forwarded_Proto": "http",
				"X_Forwarded_Host":  "evil.example.com",
			},
			wantProto: SchemeHTTPS,
			wantHost:  "app.example.ts.net",
		},
		{
			name:      "mixed-case underscore spoofing over http",
			enableTLS: false,
			initialHeaders: map[string]string{
				"X_FORWARDED_PROTO": "https",
				"x_forwarded_host":  "evil.example.com",
			},
			wantProto: SchemeHTTP,
			wantHost:  "app.example.ts.net",
		},
		{
			name:      "canonical dash keys replaced",
			enableTLS: true,
			initialHeaders: map[string]string{
				"X-Forwarded-Proto": "http",
				"X-Forwarded-Host":  "evil.example.com",
			},
			wantProto: SchemeHTTPS,
			wantHost:  "app.example.ts.net",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHTTP(HTTPOptions{
				Hostname:  "app.example.ts.net",
				EnableTLS: tt.enableTLS,
			})
			req := httptest.NewRequest(http.MethodGet, "http://app.example.ts.net/x", nil)
			for k, v := range tt.initialHeaders {
				req.Header.Set(k, v)
			}

			h.enrichHeaders(req, userInfo)

			if got := req.Header.Get(HeaderXForwardedProto); got != tt.wantProto {
				t.Errorf("X-Forwarded-Proto = %q, want %q", got, tt.wantProto)
			}
			if got := req.Header.Get(HeaderXForwardedHost); got != tt.wantHost {
				t.Errorf("X-Forwarded-Host = %q, want %q", got, tt.wantHost)
			}
			// Underscore (or other non-canonical) variants must not remain as
			// separate map keys alongside the values we set.
			for k := range req.Header {
				if strings.Contains(k, "_") {
					normalized := strings.ReplaceAll(k, "_", "-")
					if strings.EqualFold(normalized, HeaderXForwardedProto) ||
						strings.EqualFold(normalized, HeaderXForwardedHost) {
						t.Errorf("spoofed variant key still present: %q", k)
					}
				}
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
