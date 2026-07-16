package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/lucasew/ts-proxy/pkg/tsproxy"
)

// DefaultTCPDialTimeout is how long handleConn waits when dialing upstream
// before giving up. Unlimited dials can pin a goroutine forever against a
// blackholed or slow peer.
const DefaultTCPDialTimeout = 10 * time.Second

var bufferPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, 1<<15)
		return &b
	},
}

// TCPHandler forwards raw TCP connections to an upstream.
type TCPHandler struct {
	upstreamNetwork string
	upstreamAddress string
	dialTimeout     time.Duration

	// active holds connections opened by this handler so Serve can close
	// them when the parent context is cancelled. Without this, cancelling
	// only closes the listener and in-flight proxy sessions hang until the
	// peer disconnects.
	mu     sync.Mutex
	active map[net.Conn]struct{}
}

// NewTCP creates a handler that forwards raw TCP connections.
func NewTCP(upstreamNetwork, upstreamAddress string) *TCPHandler {
	return &TCPHandler{
		upstreamNetwork: upstreamNetwork,
		upstreamAddress: upstreamAddress,
		dialTimeout:     DefaultTCPDialTimeout,
		active:          make(map[net.Conn]struct{}),
	}
}

func (h *TCPHandler) track(c net.Conn) {
	h.mu.Lock()
	h.active[c] = struct{}{}
	h.mu.Unlock()
}

func (h *TCPHandler) untrack(c net.Conn) {
	h.mu.Lock()
	delete(h.active, c)
	h.mu.Unlock()
}

// closeActive closes every tracked connection. Safe to call concurrently
// with handleConn; closes may race with io.Copy and are expected.
func (h *TCPHandler) closeActive() {
	h.mu.Lock()
	conns := make([]net.Conn, 0, len(h.active))
	for c := range h.active {
		conns = append(conns, c)
	}
	h.mu.Unlock()
	for _, c := range conns {
		// Close errors here are almost always "use of closed network
		// connection" from a racing copy teardown; ignore them.
		_ = c.Close()
	}
}

func (h *TCPHandler) Serve(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		if err := ln.Close(); err != nil {
			tsproxy.ReportError(err, "context", "listener close error")
		}
		h.closeActive()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			tsproxy.ReportError(err, "context", "tcp accept error")
			continue
		}
		slog.Info("tcp connection", "remote", conn.RemoteAddr())
		go h.handleConn(ctx, conn)
	}
}

func (h *TCPHandler) handleConn(ctx context.Context, downstream net.Conn) {
	h.track(downstream)
	defer h.untrack(downstream)

	timeout := h.dialTimeout
	if timeout <= 0 {
		timeout = DefaultTCPDialTimeout
	}
	d := net.Dialer{Timeout: timeout}
	upstream, err := d.DialContext(ctx, h.upstreamNetwork, h.upstreamAddress)
	if err != nil {
		// Cancel during shutdown is expected; real dial failures are not.
		if ctx.Err() == nil {
			tsproxy.ReportError(err, "context", "tcp dial upstream", "upstream", h.upstreamAddress)
		}
		if cerr := downstream.Close(); cerr != nil && ctx.Err() == nil {
			tsproxy.ReportError(cerr, "context", "downstream close error")
		}
		return
	}
	h.track(upstream)
	defer h.untrack(upstream)

	first := make(chan struct{}, 1)
	cp := func(dst, src net.Conn) {
		buf := bufferPool.Get().(*[]byte)
		defer bufferPool.Put(buf)
		_, err := io.CopyBuffer(dst, src, *buf)
		select {
		case first <- struct{}{}:
			// Context cancel force-closes both ends; copy/close errors then
			// are expected and not reported as unexpected failures.
			shutdown := ctx.Err() != nil
			if err != nil && !shutdown {
				tsproxy.ReportError(err, "context", "tcp copy error")
			}
			if cerr := dst.Close(); cerr != nil && !shutdown {
				tsproxy.ReportError(cerr, "context", "dst close error")
			}
			if cerr := src.Close(); cerr != nil && !shutdown {
				tsproxy.ReportError(cerr, "context", "src close error")
			}
			slog.Info("tcp disconnected", "remote", downstream.RemoteAddr())
		default:
		}
	}
	go cp(downstream, upstream)
	cp(upstream, downstream)
}
