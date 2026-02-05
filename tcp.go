package tsproxy

import (
	"io"
	"log/slog"
	"net"
	"sync"
	"time"
)

var tcpIdleTimeout = 60 * time.Second

type TailscaleTCPProxyServer struct {
	server *TailscaleProxyServer
}

func NewTailscaleTCPProxyServer(server *TailscaleProxyServer) Server {
	return &TailscaleTCPProxyServer{
		server: server,
	}
}

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

	if tcpIdleTimeout > 0 {
		c1 = &idleTimeoutConn{Conn: c1, timeout: tcpIdleTimeout}
		c2 = &idleTimeoutConn{Conn: c2, timeout: tcpIdleTimeout}
	}

	first := make(chan<- struct{}, 1)
	cp := func(dst net.Conn, src net.Conn) {
		buf := bufferPool.Get().([]byte)
		defer bufferPool.Put(buf)
		// TODO use splice on linux
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

type idleTimeoutConn struct {
	net.Conn
	timeout time.Duration
}

func (c *idleTimeoutConn) Read(b []byte) (int, error) {
	if err := c.Conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
		return 0, err
	}
	return c.Conn.Read(b)
}

func (c *idleTimeoutConn) Write(b []byte) (int, error) {
	if err := c.Conn.SetWriteDeadline(time.Now().Add(c.timeout)); err != nil {
		return 0, err
	}
	return c.Conn.Write(b)
}
