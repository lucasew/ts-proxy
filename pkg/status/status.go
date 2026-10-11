// Package status serves a loopback page and /health for ts-proxy servers.
// The page is a templ component. Sakura comes from sakuracss.Load.
package status

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	jsonv2 "encoding/json/v2"

	"github.com/a-h/templ"
	"github.com/lewtec/lewkit/x/event"
	"github.com/lewtec/lewkit/x/http/asset"
)

//go:generate go run github.com/a-h/templ/cmd/templ@v0.3.1020 generate

const pageScript = `<script>
const box = document.getElementById("status");
const stream = new EventSource("/events");
stream.onmessage = (ev) => { box.innerHTML = ev.data; };
</script>
`

const (
	// AvailabilityUp means a TCP dial to the upstream succeeded.
	AvailabilityUp = "up"
	// AvailabilityDown means a TCP dial to the upstream failed.
	AvailabilityDown = "down"
	// AvailabilityUnchecked means the upstream is not probed (UDP, or no address).
	AvailabilityUnchecked = "unchecked"

	stateRunning = "running"
	probeTimeout = 300 * time.Millisecond
	eventEvery   = 5 * time.Second
)

// Server is one tsnet server at a moment in time.
type Server struct {
	Name     string
	State    string
	FQDN     string
	Forward  string
	Handlers []Handler
}

// Handler is one published port and its upstream.
type Handler struct {
	Type         string
	Listen       string
	Upstream     string
	Network      string `json:"-"`
	Availability string
}

// Listen binds the status listener. Serve owns the returned listener.
func Listen(addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("status listen %s: %w", addr, err)
	}
	return ln, nil
}

// Serve serves snap until ctx is canceled, then returns nil.
// Any other serve error is returned. Serve closes ln.
func Serve(ctx context.Context, ln net.Listener, snap func() []Server, bus *event.Bus[struct{}]) error {
	srv := &http.Server{
		Handler:           NewHandler(snap, bus),
		ReadHeaderTimeout: 5 * time.Second,
	}
	stop := context.AfterFunc(ctx, func() {
		// The parent is already canceled. WithoutCancel keeps the shutdown
		// deadline from being inherited as an immediate cancel.
		shutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Info("status shutdown", "err", err)
		}
	})
	err := srv.Serve(ln)
	stop()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// NewHandler serves GET /, GET /health, GET /events, and the sakuracss asset.
// snap may be nil. bus may be nil. /health is 200 only when every server state is running.
// /events is a server-sent stream. A bus value wakes it immediately. It also
// sends a fresh probe every 5 seconds so upstream availability moves without
// a lifecycle change.
func NewHandler(snap func() []Server, bus *event.Bus[struct{}]) http.Handler {
	p := &page{snap: snap, bus: bus}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", p.home)
	mux.HandleFunc("GET /health", p.health)
	mux.HandleFunc("GET /events", p.events)
	return asset.Mount(mux)
}

type page struct {
	snap func() []Server
	bus  *event.Bus[struct{}]
}

type healthDoc struct {
	OK      bool           `json:"ok"`
	Servers []healthServer `json:"servers"`
}

type healthServer struct {
	Name     string          `json:"name"`
	State    string          `json:"state"`
	FQDN     string          `json:"fqdn"`
	Forward  string          `json:"forward,omitempty"`
	Handlers []healthHandler `json:"handlers"`
}

type healthHandler struct {
	Type         string `json:"type"`
	Listen       string `json:"listen"`
	Upstream     string `json:"upstream"`
	Availability string `json:"availability"`
}

func (p *page) health(w http.ResponseWriter, r *http.Request) {
	doc := p.document(r.Context())
	body, err := jsonv2.Marshal(&doc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	code := http.StatusOK
	if !doc.OK {
		code = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if _, err := w.Write(body); err != nil {
		slog.Info("status health write", "err", err)
	}
}

func (p *page) home(w http.ResponseWriter, r *http.Request) {
	buf, err := renderHome(r.Context(), p.probe(r.Context()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(buf); err != nil {
		slog.Info("status page write", "err", err)
	}
}

func (p *page) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	var updates <-chan struct{}
	if p.bus != nil {
		updates = p.bus.Subscribe(r.Context())
	}
	if !p.sendEvent(w, r) {
		return
	}
	ticker := time.NewTicker(eventEvery)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case _, ok := <-updates:
			if !ok {
				return
			}
			if !p.sendEvent(w, r) {
				return
			}
		case <-ticker.C:
			if !p.sendEvent(w, r) {
				return
			}
		}
	}
}

func (p *page) sendEvent(w http.ResponseWriter, r *http.Request) bool {
	body, err := renderBody(r.Context(), p.probe(r.Context()))
	if err != nil {
		slog.Info("status event render", "err", err)
		return false
	}
	if err := writeEvent(w, body); err != nil {
		slog.Info("status event write", "err", err)
		return false
	}
	if err := http.NewResponseController(w).Flush(); err != nil {
		slog.Info("status event flush", "err", err)
		return false
	}
	return true
}

func writeEvent(w http.ResponseWriter, body []byte) error {
	for line := range bytes.SplitSeq(body, []byte("\n")) {
		if _, err := w.Write([]byte("data: ")); err != nil {
			return err
		}
		if _, err := w.Write(line); err != nil {
			return err
		}
		if _, err := w.Write([]byte("\n")); err != nil {
			return err
		}
	}
	_, err := w.Write([]byte("\n"))
	return err
}

func renderHome(ctx context.Context, servers []Server) ([]byte, error) {
	var buf bytes.Buffer
	if err := Dash(allRunning(servers), servers).Render(ctx, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderBody(ctx context.Context, servers []Server) ([]byte, error) {
	var buf bytes.Buffer
	if err := DashBody(allRunning(servers), servers).Render(ctx, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (p *page) document(ctx context.Context) healthDoc {
	servers := p.probe(ctx)
	doc := healthDoc{
		OK:      allRunning(servers),
		Servers: make([]healthServer, len(servers)),
	}
	for i, srv := range servers {
		handlers := make([]healthHandler, len(srv.Handlers))
		for j, h := range srv.Handlers {
			handlers[j] = healthHandler{
				Type:         h.Type,
				Listen:       h.Listen,
				Upstream:     h.Upstream,
				Availability: h.Availability,
			}
		}
		doc.Servers[i] = healthServer{
			Name:     srv.Name,
			State:    srv.State,
			FQDN:     srv.FQDN,
			Forward:  srv.Forward,
			Handlers: handlers,
		}
	}
	return doc
}

func (p *page) probe(ctx context.Context) []Server {
	if p.snap == nil {
		return nil
	}
	return probeAll(ctx, p.snap())
}

func allRunning(servers []Server) bool {
	if len(servers) == 0 {
		return false
	}
	for _, srv := range servers {
		if srv.State != stateRunning {
			return false
		}
	}
	return true
}

func probeAll(ctx context.Context, in []Server) []Server {
	out := make([]Server, len(in))
	var wg sync.WaitGroup
	for i, srv := range in {
		handlers := make([]Handler, len(srv.Handlers))
		copy(handlers, srv.Handlers)
		out[i] = srv
		out[i].Handlers = handlers
		for j := range handlers {
			wg.Go(func() {
				handlers[j].Availability = probe(ctx, handlers[j].Network, handlers[j].Upstream)
			})
		}
	}
	wg.Wait()
	return out
}

func probe(ctx context.Context, network, address string) string {
	switch network {
	case "", "tcp", "tcp4", "tcp6":
		if address == "" {
			return AvailabilityUnchecked
		}
		if network == "" {
			network = "tcp"
		}
	default:
		return AvailabilityUnchecked
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, network, address)
	if err != nil {
		return AvailabilityDown
	}
	if err := conn.Close(); err != nil {
		slog.Debug("status probe close", "err", err)
	}
	return AvailabilityUp
}

const (
	colorOK   = "#2e7d32"
	colorBad  = "#c62828"
	colorWait = "#ef6c00"
	colorIdle = "#607d8b"
)

// glyphs are fixed inline icons. The map key is never copied into the SVG.
var glyphs = map[string]struct {
	color string
	body  string
}{
	"running":        {colorOK, `<path d="M3.5 8.5 6.5 11.5 12.5 4.5"/>`},
	"up":             {colorOK, `<path d="M3.5 8.5 6.5 11.5 12.5 4.5"/>`},
	"failed":         {colorBad, `<path d="M4 4l8 8M12 4 4 12"/>`},
	"down":           {colorBad, `<path d="M4 4l8 8M12 4 4 12"/>`},
	"warn":           {colorWait, `<path d="M8 2.5 14.5 13.5h-13Z"/><path d="M8 6.5v3M8 11.5h.01"/>`},
	"stopped":        {colorIdle, `<rect x="4" y="4" width="8" height="8" rx="1"/>`},
	"init":           {colorIdle, `<circle cx="8" cy="8" r="5"/>`},
	"unchecked":      {colorIdle, `<path d="M4 8h8"/>`},
	"starting":       {colorWait, `<circle cx="8" cy="8" r="5"/><path d="M8 5v3.2l2.2 1.3"/>`},
	"authenticating": {colorWait, `<rect x="4" y="7" width="8" height="5.5" rx="1"/><path d="M6 7V5.5a2 2 0 0 1 4 0V7"/>`},
}

func icon(name string) templ.Component {
	g, ok := glyphs[name]
	if !ok {
		return templ.NopComponent
	}
	return templ.Raw(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="1em" height="1em" viewBox="0 0 16 16" fill="none" stroke="%s" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" style="vertical-align:-0.15em;margin-right:0.3em">%s</svg>`,
		g.color, g.body,
	))
}
