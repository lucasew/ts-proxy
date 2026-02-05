package tsproxy

import (
	"io"
	"log/slog"
	"net"
	"testing"
	"time"
)

// TestTCPProxyIdleTimeout verifies that idle connections are closed.
func TestTCPProxyIdleTimeout(t *testing.T) {
	// Silence logger for this test to avoid confusing CI with "ERROR" logs
	// that are actually expected behavior (timeouts).
	originalLogger := slog.Default()
	defer slog.SetDefault(originalLogger)
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

	// Set a short timeout for testing
	origTimeout := tcpIdleTimeout
	tcpIdleTimeout = 200 * time.Millisecond
	defer func() { tcpIdleTimeout = origTimeout }()

	// Setup a real TCP listener for upstream (c2)
	upstreamLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer upstreamLn.Close()

	// Handle upstream connections (discard data)
	go func() {
		conn, err := upstreamLn.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(io.Discard, conn)
	}()

	// Connect to upstream (create c2)
	c2, err := net.Dial("tcp", upstreamLn.Addr().String())
	if err != nil {
		t.Fatalf("dial upstream failed: %v", err)
	}
	defer c2.Close()

	// Setup a real TCP listener for proxy simulation (to get c1)
	proxyLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen proxy: %v", err)
	}
	defer proxyLn.Close()

	// Connect client (create c1 client side)
	c1Client, err := net.Dial("tcp", proxyLn.Addr().String())
	if err != nil {
		t.Fatalf("dial proxy failed: %v", err)
	}
	defer c1Client.Close()

	// Accept c1 (server side)
	c1Server, err := proxyLn.Accept()
	if err != nil {
		t.Fatalf("accept proxy failed: %v", err)
	}
	defer c1Server.Close()

	// Run handleTCPConn in a goroutine
	done := make(chan struct{})
	go func() {
		// handleTCPConn closes c1Server and c2 when done
		handleTCPConn(nil, c1Server, c2)
		close(done)
	}()

	// Test 1: Active connection stays open
	// Write to c1Client, check if it stays open > timeout
	// We write "ping" and expect it to NOT be closed.
	_, err = c1Client.Write([]byte("ping"))
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	// Wait half the timeout
	time.Sleep(100 * time.Millisecond)

	// Write again "pong"
	_, err = c1Client.Write([]byte("pong"))
	if err != nil {
		t.Fatalf("failed to write 2: %v", err)
	}

	// Test 2: Idle connection closes
	// Wait > timeout (200ms)
	// We wait 300ms.
	time.Sleep(300 * time.Millisecond)

	// Verify c1Client is closed or receives error on read
	c1Client.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	buf := make([]byte, 10)
	n, readErr := c1Client.Read(buf)

	// We expect an error (EOF or use of closed network connection)
	if readErr == nil {
		t.Errorf("expected read error after timeout, got nil, n=%d", n)
	}

	// Verify handleTCPConn returned
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Error("handleTCPConn did not return after timeout")
	}
}
