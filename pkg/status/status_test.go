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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/lucasew/ts-proxy/pkg/status"
)

func TestHealthAndDash(t *testing.T) {
	up, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, up.Close())
	})

	closed, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closedAddr := closed.Addr().String()
	require.NoError(t, closed.Close())

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
		require.Equal(t, http.StatusOK, rec.Code, "body %s", rec.Body)
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
		require.NoError(t, jsonv2.Unmarshal(rec.Body.Bytes(), &doc))
		require.True(t, doc.OK)
		require.Len(t, doc.Servers, 1)
		require.Equal(t, "web", doc.Servers[0].Name)
		got := map[string]string{}
		for _, handler := range doc.Servers[0].Handlers {
			got[handler.Listen] = handler.Availability
		}
		require.Equal(t, status.AvailabilityUp, got[":22"])
		require.Equal(t, status.AvailabilityDown, got[":23"])
		require.Equal(t, status.AvailabilityUnchecked, got[":9"])
	})

	t.Run("not running", func(t *testing.T) {
		down := status.NewHandler(func() []status.Server {
			return []status.Server{{Name: "web", State: "starting", FQDN: "web"}}
		}, nil)
		rec := httptest.NewRecorder()
		down.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("dash", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		for _, want := range []string{sakuracss.Path, "/events", "<svg", "web", "running", "127.0.0.2", status.AvailabilityUp, "All servers are running."} {
			assert.Contains(t, body, want)
		}
	})

	t.Run("sakura", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, sakuracss.Path, nil))
		require.Equal(t, http.StatusOK, rec.Code, "body %s", rec.Body)
		require.Contains(t, rec.Body.String(), "Sakura.css")
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
	require.Contains(t, first, "starting")
	state.Store("running")
	bus.PublishKeep(struct{}{})
	second := readEvent(t, rd)
	require.Contains(t, second, "running")
	require.NotContains(t, second, "starting")
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
		require.NoError(t, got.err)
		return got.s
	case <-time.After(2 * time.Second):
		require.Fail(t, "timeout waiting for event")
	}
	return ""
}

func TestServeStopsWhenContextEnds(t *testing.T) {
	ln, err := status.Listen("127.0.0.1:0")
	require.NoError(t, err)
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
		if time.Until(deadline) < 0 {
			require.NoError(t, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer func() {
		assert.NoError(t, resp.Body.Close())
	}()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "body %s", body)

	cancel()
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.Fail(t, "Serve did not return after cancel")
	}
}
