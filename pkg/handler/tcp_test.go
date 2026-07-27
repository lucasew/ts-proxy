package handler

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ErrAcceptTransient is a non-closed Accept failure used to exercise the
// Serve backoff path (must not be net.ErrClosed).
var ErrAcceptTransient = errors.New("accept: simulated resource limit")

// startServe runs h.Serve in a goroutine and returns a channel for its result.
func startServe(ctx context.Context, h *TCPHandler, ln net.Listener) <-chan error {
	done := make(chan error, 1)
	go func() {
		done <- h.Serve(ctx, ln)
	}()
	return done
}

// flakyListener fails Accept with ErrAcceptTransient until cancel closes it
// via Close (returns net.ErrClosed). Used to prove Serve backs off instead of
// spinning the accept loop.
type flakyListener struct {
	accepts atomic.Int64
	closed  chan struct{}
	once    sync.Once
}

func newFlakyListener() *flakyListener {
	return &flakyListener{closed: make(chan struct{})}
}

func (l *flakyListener) Accept() (net.Conn, error) {
	l.accepts.Add(1)
	select {
	case <-l.closed:
		return nil, net.ErrClosed
	default:
		return nil, ErrAcceptTransient
	}
}

func (l *flakyListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *flakyListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}

// TestServeAcceptErrorBackoff ensures a permanent Accept failure does not
// busy-loop: Accept should fire roughly once per acceptRetryDelay, and cancel
// during the backoff must return promptly.
func TestServeAcceptErrorBackoff(t *testing.T) {
	ln := newFlakyListener()
	h := NewTCP("tcp", "127.0.0.1:9")

	ctx, cancel := context.WithCancel(t.Context())
	start := time.Now()
	serveDone := startServe(ctx, h, ln)

	// Let a few failed Accepts + backoffs run.
	time.Sleep(350 * time.Millisecond)
	n := ln.accepts.Load()
	// Without backoff, 350ms would produce thousands of Accepts. With 100ms
	// delay, expect a small handful (startup + a few retries).
	if n < 2 {
		cancel()
		t.Fatalf("Accept calls = %d, want at least 2 failures before cancel", n)
	}
	if n > 20 {
		cancel()
		t.Fatalf("Accept calls = %d in 350ms, want backoff (~%v); loop looks busy", n, acceptRetryDelay)
	}

	cancel()
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after cancel during accept backoff")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Serve shutdown took %v, want prompt cancel", elapsed)
	}
}

// TestServeAcceptErrorLogRateLimit ensures permanent Accept failures do not
// log at the retry rate. First failure logs; further failures within the
// interval are silent so a stuck Accept path cannot flood slog.
func TestServeAcceptErrorLogRateLimit(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ln := newFlakyListener()
	h := NewTCP("tcp", "127.0.0.1:9")
	// Interval longer than the observation window so only the first failure logs.
	h.acceptErrorLogEvery = time.Hour

	ctx, cancel := context.WithCancel(t.Context())
	serveDone := startServe(ctx, h, ln)

	// Several Accept failures + backoffs (100ms each); more than one log
	// would mean rate limiting is not applied.
	time.Sleep(350 * time.Millisecond)
	n := ln.accepts.Load()
	if n < 2 {
		cancel()
		t.Fatalf("Accept calls = %d, want multiple failures to exercise rate limit", n)
	}

	cancel()
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}

	out := buf.String()
	count := strings.Count(out, "tcp accept error")
	if count != 1 {
		t.Fatalf("logged %d accept errors, want 1 (rate-limited); log=%q", count, out)
	}
}

// TestHandleConnDialTimeout ensures a blackholed upstream cannot pin handleConn
// forever. 192.0.2.0/24 is TEST-NET-1 (RFC 5737) and is not routed on the public
// Internet; combined with a short dial timeout this should fail quickly.
func TestHandleConnDialTimeout(t *testing.T) {
	h := NewTCP("tcp", "192.0.2.1:9")
	h.dialTimeout = 200 * time.Millisecond

	client, server := net.Pipe()
	// Keep client open so the pipe stays valid for handleConn to close server.
	defer client.Close()

	start := time.Now()
	done := make(chan struct{})
	go func() {
		h.handleConn(t.Context(), server)
		close(done)
	}()

	select {
	case <-done:
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("handleConn took %v, want roughly dialTimeout", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handleConn did not return after dial timeout")
	}
}

// TestHandleConnDialContextCancel ensures dial aborts when the Serve context
// is cancelled instead of waiting for the full dial timeout.
func TestHandleConnDialContextCancel(t *testing.T) {
	h := NewTCP("tcp", "192.0.2.1:9")
	h.dialTimeout = 10 * time.Second

	client, server := net.Pipe()
	defer client.Close()

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	start := time.Now()
	go func() {
		h.handleConn(ctx, server)
		close(done)
	}()

	// Give dial a moment to start, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("handleConn took %v after cancel, want prompt abort", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handleConn did not return after context cancel during dial")
	}
}

// TestHandleConnProxiesBytes is a smoke test that a reachable upstream still works.
func TestHandleConnProxiesBytes(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 64)
		n, err := conn.Read(buf)
		if err != nil && err != io.EOF {
			return
		}
		if _, err := conn.Write([]byte("pong:" + string(buf[:n]))); err != nil {
			return
		}
	}()

	h := NewTCP("tcp", ln.Addr().String())
	h.dialTimeout = 2 * time.Second

	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		h.handleConn(t.Context(), server)
		close(done)
	}()

	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 64)
	if err := client.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	n, err := client.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(buf[:n])
	if got != "pong:ping" {
		t.Fatalf("got %q, want %q", got, "pong:ping")
	}
	if err := client.Close(); err != nil {
		t.Logf("client close: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleConn did not finish after client close")
	}
	wg.Wait()
}

// TestServeHalfCloseDeliversResponse ensures that when the client finishes
// writing and half-closes, the upstream can still send a full response.
// The previous proxy closed both ends when the first copy finished, which
// truncated the reverse direction for half-close protocols.
func TestServeHalfCloseDeliversResponse(t *testing.T) {
	upLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("upstream listen: %v", err)
	}
	defer upLn.Close()

	upDone := make(chan struct{})
	go func() {
		defer close(upDone)
		c, err := upLn.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		// Read until client CloseWrite (EOF), then reply.
		req, err := io.ReadAll(c)
		if err != nil {
			return
		}
		// Small delay so a buggy first-finisher-closes-both proxy would
		// already have torn down the client side.
		time.Sleep(50 * time.Millisecond)
		if _, err := c.Write([]byte("pong:" + string(req))); err != nil {
			return
		}
	}()

	h := NewTCP("tcp", upLn.Addr().String())
	proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("proxy listen: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	serveDone := startServe(ctx, h, proxyLn)

	client, err := net.Dial("tcp", proxyLn.Addr().String())
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer client.Close()

	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	tcpClient, ok := client.(*net.TCPConn)
	if !ok {
		t.Fatal("client is not *net.TCPConn; cannot CloseWrite")
	}
	if err := tcpClient.CloseWrite(); err != nil {
		t.Fatalf("CloseWrite: %v", err)
	}

	if err := client.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	resp, err := io.ReadAll(client)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if got := string(resp); got != "pong:ping" {
		t.Fatalf("response = %q, want %q", got, "pong:ping")
	}

	cancel()
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
	select {
	case <-upDone:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream did not finish")
	}
}

// TestServeClosesActiveConnsOnCancel ensures cancelling Serve tears down
// in-flight proxy sessions, not only the accept loop listener.
func TestServeClosesActiveConnsOnCancel(t *testing.T) {
	upLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("upstream listen: %v", err)
	}
	defer upLn.Close()

	var upWg sync.WaitGroup
	upDone := make(chan struct{})
	go func() {
		defer close(upDone)
		for {
			c, err := upLn.Accept()
			if err != nil {
				return
			}
			upWg.Add(1)
			go func(c net.Conn) {
				defer upWg.Done()
				defer c.Close()
				// Hold the connection open until the peer closes.
				if _, err := io.Copy(io.Discard, c); err != nil {
					return
				}
			}(c)
		}
	}()

	h := NewTCP("tcp", upLn.Addr().String())
	proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("proxy listen: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	serveDone := startServe(ctx, h, proxyLn)

	client, err := net.Dial("tcp", proxyLn.Addr().String())
	if err != nil {
		cancel()
		t.Fatalf("dial proxy: %v", err)
	}
	defer client.Close()

	// Nudge data so the proxy dials upstream and both directions are live.
	if _, err := client.Write([]byte("x")); err != nil {
		cancel()
		t.Fatalf("write: %v", err)
	}
	// Allow handleConn to track both ends.
	time.Sleep(100 * time.Millisecond)

	cancel()

	// Client must observe the forced close promptly (not hang until peer idle).
	if err := client.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	start := time.Now()
	buf := make([]byte, 1)
	_, err = client.Read(buf)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected read error after Serve cancel closed the connection")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("read after cancel took %v, want connection closed promptly", elapsed)
	}

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}

	if err := upLn.Close(); err != nil {
		t.Logf("upstream listen close: %v", err)
	}
	select {
	case <-upDone:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream accept loop did not exit")
	}
	upWg.Wait()

	// Serve waits for handleConn (sessions WaitGroup) before returning, so
	// every tracked connection must already be untracked.
	h.mu.Lock()
	left := len(h.active)
	h.mu.Unlock()
	if left != 0 {
		t.Fatalf("Serve returned with %d active connections still tracked", left)
	}
}

// TestServeDrainsSessionsBeforeReturn ensures Serve does not return while a
// proxy session is still running. Without sessions.Wait, the accept loop
// would exit as soon as the listener closed and the caller could tear down
// tsnet under in-flight copies.
func TestServeDrainsSessionsBeforeReturn(t *testing.T) {
	upLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("upstream listen: %v", err)
	}
	defer upLn.Close()

	// Upstream blocks in Copy until the proxy closes the connection.
	upAccepted := make(chan struct{})
	go func() {
		c, err := upLn.Accept()
		if err != nil {
			return
		}
		close(upAccepted)
		defer c.Close()
		if _, err := io.Copy(io.Discard, c); err != nil {
			return
		}
	}()

	h := NewTCP("tcp", upLn.Addr().String())
	proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("proxy listen: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	serveDone := startServe(ctx, h, proxyLn)

	client, err := net.Dial("tcp", proxyLn.Addr().String())
	if err != nil {
		cancel()
		t.Fatalf("dial proxy: %v", err)
	}
	defer client.Close()

	if _, err := client.Write([]byte("x")); err != nil {
		cancel()
		t.Fatalf("write: %v", err)
	}
	select {
	case <-upAccepted:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("upstream did not accept")
	}
	// Session is live: both ends tracked, copies blocked on idle conns.
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("Serve: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}

	h.mu.Lock()
	left := len(h.active)
	h.mu.Unlock()
	if left != 0 {
		t.Fatalf("after Serve return: %d active connections still tracked", left)
	}
}
