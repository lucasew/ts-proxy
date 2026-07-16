package handler

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

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
		h.handleConn(context.Background(), server)
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

	ctx, cancel := context.WithCancel(context.Background())
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
		_, _ = conn.Write([]byte("pong:" + string(buf[:n])))
	}()

	h := NewTCP("tcp", ln.Addr().String())
	h.dialTimeout = 2 * time.Second

	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		h.handleConn(context.Background(), server)
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
	_ = client.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleConn did not finish after client close")
	}
	wg.Wait()
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
				_, _ = io.Copy(io.Discard, c)
			}(c)
		}
	}()

	h := NewTCP("tcp", upLn.Addr().String())
	proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("proxy listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- h.Serve(ctx, proxyLn)
	}()

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

	_ = upLn.Close()
	select {
	case <-upDone:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream accept loop did not exit")
	}
	upWg.Wait()
}
