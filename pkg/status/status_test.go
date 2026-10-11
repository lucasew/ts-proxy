package status_test

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	jsonv2 "encoding/json/v2"

	"github.com/lewtec/lewkit/x/event"
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
	h := status.NewHandler(snap, nil)

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
		}, nil)
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
		for _, want := range []string{sakuracss.Path, "/events", "<svg", "web", "running", "127.0.0.2", status.AvailabilityUp, "All servers are running."} {
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

func TestEventsFollowBus(t *testing.T) {
	bus := event.New[struct{}]()
	var state atomic.Value
	state.Store("starting")
	h := status.NewHandler(func() []status.Server {
		return []status.Server{{Name: "web", State: state.Load().(string), FQDN: "web"}}
	}, bus)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pr, pw := io.Pipe()
	go func() {
		h.ServeHTTP(&flushWriter{header: make(http.Header), w: pw}, httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx))
		pw.Close()
	}()
	rd := bufio.NewReader(pr)
	first := readEvent(t, rd)
	if !strings.Contains(first, "starting") {
		t.Fatalf("first event = %q", first)
	}
	state.Store("running")
	bus.PublishKeep(struct{}{})
	second := readEvent(t, rd)
	if !strings.Contains(second, "running") || strings.Contains(second, "starting") {
		t.Fatalf("second event = %q", second)
	}
	cancel()
}

type flushWriter struct {
	header http.Header
	w      io.Writer
}

func (f *flushWriter) Header() http.Header { return f.header }

func (f *flushWriter) WriteHeader(int) {}

func (f *flushWriter) Write(p []byte) (int, error) { return f.w.Write(p) }

func (f *flushWriter) Flush() {}

func readEvent(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	type result struct {
		s   string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		var b strings.Builder
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				ch <- result{err: err}
				return
			}
			if line == "\n" {
				ch <- result{s: b.String()}
				return
			}
			b.WriteString(line)
		}
	}()
	select {
	case got := <-ch:
		if got.err != nil {
			t.Fatalf("read event: %v", got.err)
		}
		return got.s
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for event")
	}
	return ""
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
		}, nil)
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
