// Package nettest holds shared network fixtures for package tests.
package nettest

import (
	"io"
	"net"
	"sync"
)

// AcceptPongOnce accepts one connection on ln and replies "pong:" plus
// the first read. Returns a wait func for the accept goroutine.
func AcceptPongOnce(ln net.Listener) (wait func()) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 64)
		n, err := c.Read(buf)
		if err != nil && err != io.EOF {
			return
		}
		if _, err := c.Write([]byte("pong:" + string(buf[:n]))); err != nil {
			return
		}
	}()
	return wg.Wait
}
