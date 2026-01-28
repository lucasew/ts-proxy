package tsproxy

import (
	"io"
	"log/slog"
	"net"
	"sync"
)

/**
 * TailscaleTCPProxyServer implements a raw L4 TCP proxy.
 * It accepts connections from the Tailscale listener and tunnels them
 * directly to the upstream service without inspecting the payload.
 */
type TailscaleTCPProxyServer struct {
	server *TailscaleProxyServer
}

/**
 * NewTailscaleTCPProxyServer creates a new TCP proxy instance.
 */
func NewTailscaleTCPProxyServer(server *TailscaleProxyServer) Server {
	return &TailscaleTCPProxyServer{
		server: server,
	}
}

/**
 * Serve accepts incoming TCP connections and spawns a goroutine for each.
 * It runs until the parent context is canceled.
 */
func (tps *TailscaleTCPProxyServer) Serve(ln net.Listener) error {
	for {
		select {
		case <-tps.server.ctx.Done():
			break
		default:
			conn, err := ln.Accept()
			if err != nil {
				slog.Error("error/accept", "err", err)
				continue
			}
			slog.Info("got tcp conn")
			go handleTCPConn(tps.server, conn, nil)
		}
	}
}

var bufferPool = sync.Pool{
	New: func() interface{} {
		// TODO maybe different buffer size?
		// benchmark pls
		return make([]byte, 1<<15)
	},
}

/**
 * handleTCPConn manages the bidirectional data flow between two connections.
 *
 * If the upstream connection (c2) is not provided, it dials the upstream
 * using the server's configuration.
 *
 * It spawns two copy routines: client->upstream and upstream->client.
 * Whichever side closes or errors first triggers the closure of both connections,
 * ensuring no resources are leaked.
 */
func handleTCPConn(server *TailscaleProxyServer, c1 net.Conn, c2 net.Conn) {
	var err error
	if c2 == nil {
		c2, err = server.Dial("whatever", "whatever")
		if err != nil {
			slog.Error("tcp error", "err", err)
			c1.Close()
			slog.Info("disconnected", "remote_addr", c1.RemoteAddr())
			return
		}

	}
	first := make(chan<- struct{}, 1)
	cp := func(dst net.Conn, src net.Conn) {
		buf := bufferPool.Get().([]byte)
		defer bufferPool.Put(buf)
		// TODO use splice on linux
		// TODO needs some timeout to prevent torshammer ddos
		_, err := io.CopyBuffer(dst, src, buf)
		select {
		case first <- struct{}{}:
			if err != nil {
				slog.Error("tcp error", "err", err)
			}
			dst.Close()
			src.Close()
			slog.Info("disconnected", "remote_addr", c1.RemoteAddr())
		default:
		}
	}
	go cp(c1, c2)
	cp(c2, c1)
}
