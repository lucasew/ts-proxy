package handler

import (
	"context"
	"net"

	"tailscale.com/client/tailscale/apitype"
)

// Handler serves connections on a net.Listener.
type Handler interface {
	Serve(ctx context.Context, ln net.Listener) error
}

// WhoIsFunc resolves a remote address to Tailscale user information.
type WhoIsFunc func(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error)
