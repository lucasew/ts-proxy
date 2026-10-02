package server

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/lucasew/ts-proxy/internal/nettest"
	"github.com/lucasew/ts-proxy/pkg/handler"
)

func TestForwardAddr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		host string
		port uint16
		want string
	}{
		{host: "127.0.0.2", port: 22, want: "127.0.0.2:22"},
		{host: "localhost", port: 80, want: "localhost:80"},
		{host: "::1", port: 443, want: "[::1]:443"},
		{host: "2001:db8::1", port: 9, want: "[2001:db8::1]:9"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			got := ForwardAddr(tt.host, tt.port)
			if got != tt.want {
				t.Fatalf("ForwardAddr(%q, %d) = %q, want %q", tt.host, tt.port, got, tt.want)
			}
		})
	}
}

func TestFallbackTCPSplicesDestPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.2:0")
	if err != nil {
		// 127.0.0.2 is loopback; if the host rejects it, bind 127.0.0.1.
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
	}
	defer ln.Close()

	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	p, err := net.LookupPort("tcp", portStr)
	if err != nil {
		t.Fatalf("LookupPort: %v", err)
	}
	port := uint16(p)

	wait := nettest.AcceptPongOnce(ln)
	defer wait()

	h := handler.NewTCP("tcp", "")
	cb := forwardFallback{ctx: t.Context(), name: "gremio", host: host, h: h}.handle
	src := netip.MustParseAddrPort("100.64.0.2:4242")
	dst := netip.AddrPortFrom(netip.MustParseAddr("100.64.0.1"), port)
	serve, intercept := cb(src, dst)
	if !intercept {
		t.Fatal("fallback must intercept")
	}
	if serve == nil {
		t.Fatal("fallback must return a handler")
	}

	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		serve(server)
		close(done)
	}()

	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := client.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 64)
	n, err := client.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := string(buf[:n]); got != "pong:ping" {
		t.Fatalf("got %q, want %q", got, "pong:ping")
	}
	if err := client.Close(); err != nil {
		t.Logf("client close: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fallback handler did not finish")
	}
}

func TestFallbackTCPRejectsNothing(t *testing.T) {
	t.Parallel()
	h := handler.NewTCP("tcp", "")
	cb := forwardFallback{ctx: t.Context(), name: "empty", host: "127.0.0.2", h: h}.handle
	_, intercept := cb(
		netip.MustParseAddrPort("100.64.0.2:1"),
		netip.MustParseAddrPort("100.64.0.1:65535"),
	)
	if !intercept {
		t.Fatal("forward fallback must intercept every unmatched TCP port")
	}
}
