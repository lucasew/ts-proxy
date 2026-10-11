package server

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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
			require.Equal(t, tt.want, ForwardAddr(tt.host, tt.port))
		})
	}
}

func TestFallbackTCPSplicesDestPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.2:0")
	if err != nil {
		// 127.0.0.2 is loopback; if the host rejects it, bind 127.0.0.1.
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
	}
	defer ln.Close()

	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	p, err := net.LookupPort("tcp", portStr)
	require.NoError(t, err)
	port := uint16(p)

	wait := nettest.AcceptPongOnce(ln)
	defer wait()

	h := handler.NewTCP("tcp", "")
	cb := forwardFallback{ctx: t.Context(), name: "gremio", host: host, h: h}.handle
	src := netip.MustParseAddrPort("100.64.0.2:4242")
	dst := netip.AddrPortFrom(netip.MustParseAddr("100.64.0.1"), port)
	serve, intercept := cb(src, dst)
	require.True(t, intercept)
	require.NotNil(t, serve)

	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		serve(server)
		close(done)
	}()

	_, err = client.Write([]byte("ping"))
	require.NoError(t, err)
	require.NoError(t, client.SetReadDeadline(time.Now().Add(2*time.Second)))
	buf := make([]byte, 64)
	n, err := client.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "pong:ping", string(buf[:n]))
	if err := client.Close(); err != nil {
		t.Logf("client close: %v", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "fallback handler did not finish")
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
	require.True(t, intercept, "forward fallback must intercept every unmatched TCP port")
}
