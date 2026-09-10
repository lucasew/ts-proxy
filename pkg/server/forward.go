package server

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"strconv"

	"github.com/lucasew/ts-proxy/pkg/handler"
)

// ForwardAddr joins a forward host and an inbound dest port into a dial address.
func ForwardAddr(host string, port uint16) string {
	return net.JoinHostPort(host, strconv.FormatUint(uint64(port), 10))
}

type forwardFallback struct {
	ctx  context.Context
	name string
	host string
	h    *handler.TCPHandler
}

func (f forwardFallback) handle(src, dst netip.AddrPort) (func(net.Conn), bool) {
	addr := ForwardAddr(f.host, dst.Port())
	slog.Info("tcp forward", "server", f.name, "remote", src, "upstream", addr)
	return func(c net.Conn) {
		f.h.ServeConn(f.ctx, c, "tcp", addr)
	}, true
}
