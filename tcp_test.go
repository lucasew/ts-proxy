package tsproxy

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestTimeoutConn(t *testing.T) {
	originalTimeout := TCPIdleTimeout
	TCPIdleTimeout = 10 * time.Millisecond
	defer func() { TCPIdleTimeout = originalTimeout }()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		tc := &timeoutConn{Conn: conn, timeout: TCPIdleTimeout}
		buf := make([]byte, 1024)
		_, err = tc.Read(buf)
		if err == nil {
			t.Error("expected timeout error")
		} else {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				// expected
			} else if err != io.EOF {
				t.Errorf("expected timeout, got %v", err)
			}
		}
		close(done)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("test timed out")
	}
}
