package handler

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tailscale.com/client/local"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

func recvUpstream(t *testing.T, got <-chan *http.Request) *http.Request {
	t.Helper()
	select {
	case up := <-got:
		return up
	case <-time.After(2 * time.Second):
		t.Fatal("upstream did not receive request")
		return nil
	}
}

// startUpstream records the first request it receives and returns 204.
func startUpstream(t *testing.T) (addr string, got chan *http.Request, cleanup func()) {
	t.Helper()
	got = make(chan *http.Request, 1)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Copy fields we care about before the server reuses the request.
			clone := r.Clone(r.Context())
			clone.Body = nil
			select {
			case got <- clone:
			default:
			}
			w.WriteHeader(http.StatusNoContent)
		}),
	}
	go func() { _ = srv.Serve(ln) }()
	return ln.Addr().String(), got, func() {
		_ = srv.Close()
		_ = ln.Close()
	}
}

func TestServeHTTPPeerNotFoundStillProxies(t *testing.T) {
	addr, got, cleanup := startUpstream(t)
	defer cleanup()

	h := NewHTTP(HTTPOptions{
		Hostname:        "app.example.ts.net",
		EnableTLS:       true,
		UpstreamAddress: addr,
		WhoIs: func(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error) {
			return nil, local.ErrPeerNotFound
		},
	})

	req := httptest.NewRequest(http.MethodGet, "http://app.example.ts.net/public", nil)
	req.Host = "app.example.ts.net"
	req.RemoteAddr = "203.0.113.10:54321"
	// Spoofed identity must not reach upstream for anonymous Funnel clients.
	req.Header.Set(TailscaleUserLoginHeader, "attacker@evil.example")
	req.Header.Set("X_Forwarded_For", "1.2.3.4")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %q; want %d (Funnel/public must not 500 on peer-not-found)",
			rec.Code, rec.Body.String(), http.StatusNoContent)
	}

	up := recvUpstream(t, got)

	if v := up.Header.Get(TailscaleUserLoginHeader); v != "" {
		t.Errorf("upstream Tailscale-User-Login = %q, want empty for peer-not-found", v)
	}
	if v := up.Header.Get(HeaderXForwardedProto); v != SchemeHTTPS {
		t.Errorf("X-Forwarded-Proto = %q, want %q", v, SchemeHTTPS)
	}
	if v := up.Header.Get(HeaderXForwardedHost); v != "app.example.ts.net" {
		t.Errorf("X-Forwarded-Host = %q, want app.example.ts.net", v)
	}
	// ReverseProxy sets X-Forwarded-For from RemoteAddr after we strip spoofs.
	if v := up.Header.Get(HeaderXForwardedFor); v != "203.0.113.10" {
		t.Errorf("X-Forwarded-For = %q, want 203.0.113.10 (spoof stripped, real client only)", v)
	}
}

func TestServeHTTPWhoIsHardErrorIs500(t *testing.T) {
	addr, _, cleanup := startUpstream(t)
	defer cleanup()

	h := NewHTTP(HTTPOptions{
		Hostname:        "app.example.ts.net",
		UpstreamAddress: addr,
		WhoIs: func(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error) {
			return nil, errors.New("localapi down")
		},
	})

	req := httptest.NewRequest(http.MethodGet, "http://app.example.ts.net/", nil)
	req.Host = "app.example.ts.net"
	req.RemoteAddr = "100.64.0.1:1234"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for unexpected whois errors", rec.Code)
	}
}

func TestServeHTTPSetsIdentityForTailnetUser(t *testing.T) {
	addr, got, cleanup := startUpstream(t)
	defer cleanup()

	h := NewHTTP(HTTPOptions{
		Hostname:        "app.example.ts.net",
		EnableTLS:       true,
		UpstreamAddress: addr,
		WhoIs: func(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error) {
			return &apitype.WhoIsResponse{
				UserProfile: &tailcfg.UserProfile{
					LoginName:     "user@example.com",
					DisplayName:   "User",
					ProfilePicURL: "http://example.com/pic.jpg",
				},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "http://app.example.ts.net/", nil)
	req.Host = "app.example.ts.net"
	req.RemoteAddr = "100.64.0.2:9999"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	up := recvUpstream(t, got)
	if v := up.Header.Get(TailscaleUserLoginHeader); v != "user@example.com" {
		t.Errorf("Tailscale-User-Login = %q, want user@example.com", v)
	}
	if v := up.Header.Get(HeaderXForwardedFor); v != "100.64.0.2" {
		t.Errorf("X-Forwarded-For = %q, want 100.64.0.2", v)
	}
	if v := up.Header.Get(HeaderXForwardedProto); v != SchemeHTTPS {
		t.Errorf("X-Forwarded-Proto = %q, want %q", v, SchemeHTTPS)
	}
}

func TestServeHTTPTaggedDeviceOmitsIdentity(t *testing.T) {
	addr, got, cleanup := startUpstream(t)
	defer cleanup()

	h := NewHTTP(HTTPOptions{
		Hostname:        "app.example.ts.net",
		UpstreamAddress: addr,
		WhoIs: func(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error) {
			return &apitype.WhoIsResponse{
				UserProfile: &tailcfg.UserProfile{
					LoginName: taggedDevicesLogin,
				},
			}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "http://app.example.ts.net/", nil)
	req.Host = "app.example.ts.net"
	req.RemoteAddr = "100.64.0.3:1"
	req.Header.Set(TailscaleUserLoginHeader, "spoofed@example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	up := recvUpstream(t, got)
	if v := up.Header.Get(TailscaleUserLoginHeader); v != "" {
		t.Errorf("Tailscale-User-Login = %q, want empty for tagged devices", v)
	}
}

func TestServeHTTPNilUserProfileNoPanic(t *testing.T) {
	addr, got, cleanup := startUpstream(t)
	defer cleanup()

	h := NewHTTP(HTTPOptions{
		Hostname:        "app.example.ts.net",
		UpstreamAddress: addr,
		WhoIs: func(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error) {
			return &apitype.WhoIsResponse{UserProfile: nil}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "http://app.example.ts.net/", nil)
	req.Host = "app.example.ts.net"
	req.RemoteAddr = "100.64.0.4:1"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d (nil UserProfile must not panic/500)", rec.Code, http.StatusNoContent)
	}
	up := recvUpstream(t, got)
	if v := up.Header.Get(TailscaleUserLoginHeader); v != "" {
		t.Errorf("Tailscale-User-Login = %q, want empty", v)
	}
}

func TestHasTailscaleUserIdentity(t *testing.T) {
	if hasTailscaleUserIdentity(nil) {
		t.Error("nil should be false")
	}
	if hasTailscaleUserIdentity(&apitype.WhoIsResponse{}) {
		t.Error("nil profile should be false")
	}
	if hasTailscaleUserIdentity(&apitype.WhoIsResponse{UserProfile: &tailcfg.UserProfile{}}) {
		t.Error("empty login should be false")
	}
	if hasTailscaleUserIdentity(&apitype.WhoIsResponse{UserProfile: &tailcfg.UserProfile{LoginName: taggedDevicesLogin}}) {
		t.Error("tagged-devices should be false")
	}
	if !hasTailscaleUserIdentity(&apitype.WhoIsResponse{UserProfile: &tailcfg.UserProfile{LoginName: "a@b.c"}}) {
		t.Error("real user should be true")
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
