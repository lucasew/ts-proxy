// Package status serves a loopback page and /health for ts-proxy servers.
// The page uses the sakuracss asset from lewkit.
package status

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	jsonv2 "encoding/json/v2"

	"github.com/lewtec/lewkit/x/http/asset"
	"github.com/lewtec/lewkit/x/http/asset/sakuracss"
)

const (
	// AvailabilityUp means a TCP dial to the upstream succeeded.
	AvailabilityUp = "up"
	// AvailabilityDown means a TCP dial to the upstream failed.
	AvailabilityDown = "down"
	// AvailabilityUnchecked means the upstream is not probed (UDP, or no address).
	AvailabilityUnchecked = "unchecked"

	stateRunning = "running"
	probeTimeout = 300 * time.Millisecond
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
func Serve(ctx context.Context, ln net.Listener, snap func() []Server) error {
	srv := &http.Server{
		Handler:           NewHandler(snap),
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

// NewHandler serves GET /, GET /health, and the sakuracss asset.
// snap may be nil. /health is 200 only when every server state is running.
func NewHandler(snap func() []Server) http.Handler {
	p := &page{snap: snap}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", p.home)
	mux.HandleFunc("GET /health", p.health)
	return asset.Mount(mux)
}

type page struct {
	snap func() []Server
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
	buf, err := renderHome(p.probe(r.Context()))
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

func renderHome(servers []Server) ([]byte, error) {
	data := struct {
		Style   string
		OK      bool
		Servers []Server
	}{
		Style:   sakuracss.Path,
		OK:      allRunning(servers),
		Servers: servers,
	}
	var buf bytes.Buffer
	if err := pageTmpl.Execute(&buf, data); err != nil {
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

var pageTmpl = template.Must(template.New("status").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="refresh" content="5">
<title>ts-proxy</title>
<link rel="stylesheet" href="{{ .Style }}">
</head>
<body>
<h1>ts-proxy</h1>
{{ if .OK }}<p>All servers are running.</p>{{ else }}<p>Not every server is running.</p>{{ end }}
{{ range .Servers }}
<h2>{{ .Name }}</h2>
<dl>
<dt>State</dt><dd>{{ .State }}</dd>
<dt>Hostname</dt><dd>{{ .FQDN }}</dd>
{{ if .Forward }}<dt>Forward</dt><dd>{{ .Forward }}</dd>{{ end }}
</dl>
{{ if .Handlers }}
<table>
<thead><tr><th>Type</th><th>Listen</th><th>Upstream</th><th>Availability</th></tr></thead>
<tbody>
{{ range .Handlers }}<tr><td>{{ .Type }}</td><td>{{ .Listen }}</td><td>{{ .Upstream }}</td><td>{{ .Availability }}</td></tr>
{{ end }}
</tbody>
</table>
{{ else }}<p>No handlers.</p>{{ end }}
{{ end }}
</body>
</html>
`))
