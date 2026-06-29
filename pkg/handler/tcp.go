package handler

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/lucasew/ts-proxy/pkg/tsproxy"
)

var bufferPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 1<<15)
	},
}

// TCPHandler forwards raw TCP connections to an upstream.
type TCPHandler struct {
	upstreamNetwork string
	upstreamAddress string
}

// NewTCP creates a handler that forwards raw TCP connections.
func NewTCP(upstreamNetwork, upstreamAddress string) *TCPHandler {
	return &TCPHandler{
		upstreamNetwork: upstreamNetwork,
		upstreamAddress: upstreamAddress,
	}
}

func (h *TCPHandler) Serve(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			tsproxy.ReportError("tcp accept error", err)
			continue
		}
		slog.Info("tcp connection", "remote", conn.RemoteAddr())
		go h.handleConn(conn)
	}
}

func (h *TCPHandler) handleConn(downstream net.Conn) {
	upstream, err := net.Dial(h.upstreamNetwork, h.upstreamAddress)
	if err != nil {
		tsproxy.ReportError("tcp dial upstream", err, "upstream", h.upstreamAddress)
		downstream.Close()
		return
	}

	first := make(chan struct{}, 1)
	cp := func(dst, src net.Conn) {
		buf := bufferPool.Get().([]byte)
		defer bufferPool.Put(buf)
		_, err := io.CopyBuffer(dst, src, buf)
		select {
		case first <- struct{}{}:
			if err != nil {
				tsproxy.ReportError("tcp copy error", err)
			}
			dst.Close()
			src.Close()
			slog.Info("tcp disconnected", "remote", downstream.RemoteAddr())
		default:
		}
	}
	go cp(downstream, upstream)
	cp(upstream, downstream)
}
