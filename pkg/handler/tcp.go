package handler

import (
	"context"
	"github.com/lucasew/ts-proxy/pkg/tsproxy"
	"io"
	"log/slog"
	"net"
	"sync"
)

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
		if err := ln.Close(); err != nil {
			tsproxy.ReportError(err, "context", "listener close error")
		}
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
		go h.handleConn(conn)
	}
}

func (h *TCPHandler) handleConn(downstream net.Conn) {
	upstream, err := net.Dial(h.upstreamNetwork, h.upstreamAddress)
	if err != nil {
		tsproxy.ReportError(err, "context", "tcp dial upstream", "upstream", h.upstreamAddress)
		if cerr := downstream.Close(); cerr != nil {
			tsproxy.ReportError(cerr, "context", "downstream close error")
		}
		return
	}

	first := make(chan struct{}, 1)
	cp := func(dst, src net.Conn) {
		buf := bufferPool.Get().(*[]byte)
		defer bufferPool.Put(buf)
		_, err := io.CopyBuffer(dst, src, *buf)
		select {
		case first <- struct{}{}:
			if err != nil {
				tsproxy.ReportError(err, "context", "tcp copy error")
			}
			if cerr := dst.Close(); cerr != nil {
				tsproxy.ReportError(cerr, "context", "dst close error")
			}
			if cerr := src.Close(); cerr != nil {
				tsproxy.ReportError(cerr, "context", "src close error")
			}
			slog.Info("tcp disconnected", "remote", downstream.RemoteAddr())
		default:
		}
	}
	go cp(downstream, upstream)
	cp(upstream, downstream)
}
