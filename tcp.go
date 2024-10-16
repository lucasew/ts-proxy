package tsproxy

import (
	"io"
	"log"
	"net"
	"sync"
)

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
				log.Printf("error/accept: %s", err.Error())
				continue
			}
			log.Printf("got tcp conn")
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
			log.Print(err)
			c1.Close()
			log.Printf("disconnected %v", c1.RemoteAddr())
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
				log.Print(err)
			}
			dst.Close()
			src.Close()
			log.Printf("disconnected %v", c1.RemoteAddr())
		default:
		}
	}
	go cp(c1, c2)
	cp(c2, c1)
}
