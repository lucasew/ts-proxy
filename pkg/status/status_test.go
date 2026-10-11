package status_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	jsonv2 "encoding/json/v2"

	"github.com/lewtec/lewkit/x/http/asset/sakuracss"
	"github.com/lucasew/ts-proxy/pkg/status"
)

func TestHealthAndDash(t *testing.T) {
	up, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() {
		if err := up.Close(); err != nil {
			t.Errorf("close up listener: %v", err)
		}
	})

	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	closedAddr := closed.Addr().String()
	if err := closed.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	snap := func() []status.Server {
		return []status.Server{{
			Name:    "web",
			State:   "running",
			FQDN:    "web.example.ts.net",
			Forward: "127.0.0.2",
			Handlers: []status.Handler{
				{Type: "tcp", Listen: ":22", Upstream: up.Addr().String(), Network: "tcp"},
				{Type: "tcp", Listen: ":23", Upstream: closedAddr, Network: "tcp"},
				{Type: "udp", Listen: ":9", Upstream: "127.0.0.1:9", Network: "udp"},
			},
		}}
	}
	h := status.NewHandler(snap)

	t.Run("health", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		var doc struct {
			OK      bool `json:"ok"`
			Servers []struct {
				Name     string `json:"name"`
				Handlers []struct {
					Listen       string `json:"listen"`
					Availability string `json:"availability"`
				} `json:"handlers"`
			} `json:"servers"`
		}
		if err := jsonv2.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
			t.Fatalf("json: %v", err)
		}
		if !doc.OK || len(doc.Servers) != 1 || doc.Servers[0].Name != "web" {
			t.Fatalf("doc = %+v", doc)
		}
		got := map[string]string{}
		for _, handler := range doc.Servers[0].Handlers {
			got[handler.Listen] = handler.Availability
		}
		if got[":22"] != status.AvailabilityUp || got[":23"] != status.AvailabilityDown || got[":9"] != status.AvailabilityUnchecked {
			t.Fatalf("availability = %v", got)
		}
	})

	t.Run("not running", func(t *testing.T) {
		down := status.NewHandler(func() []status.Server {
			return []status.Server{{Name: "web", State: "starting", FQDN: "web"}}
		})
		rec := httptest.NewRecorder()
		down.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", rec.Code)
		}
	})

	t.Run("dash", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		body := rec.Body.String()
		for _, want := range []string{sakuracss.Path, "web", "running", "127.0.0.2", status.AvailabilityUp, "All servers are running."} {
			if !strings.Contains(body, want) {
				t.Errorf("body missing %q", want)
			}
		}
	})

	t.Run("sakura", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, sakuracss.Path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
		}
		if !strings.Contains(rec.Body.String(), "Sakura.css") {
			t.Fatalf("stylesheet body = %q", rec.Body.String()[:80])
		}
	})
}

func TestServeStopsWhenContextEnds(t *testing.T) {
	ln, err := status.Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() {
		errCh <- status.Serve(ctx, ln, func() []status.Server {
			return []status.Server{{Name: "web", State: "running", FQDN: "web"}}
		})
	}()

	url := "http://" + ln.Addr().String() + "/health"
	deadline := time.Now().Add(2 * time.Second)
	client := &http.Client{Timeout: time.Second}
	var resp *http.Response
	for {
		resp, err = client.Get(url)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("health: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close body: %v", err)
		}
	}()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read health body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", resp.StatusCode, body)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
}
