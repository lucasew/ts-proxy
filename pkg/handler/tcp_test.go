package handler

import (
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
		h.handleConn(server)
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
		h.handleConn(server)
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
