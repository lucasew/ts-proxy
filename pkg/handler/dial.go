package handler

import (
	"net"
	"time"
)

// DefaultDialTimeout is how long handlers wait when dialing upstream
// before giving up. Unlimited dials can pin a goroutine forever against
// a blackholed or slow peer.
const DefaultDialTimeout = 10 * time.Second

// Kept as aliases so existing names stay valid.
const (
	DefaultHTTPDialTimeout = DefaultDialTimeout
	DefaultTCPDialTimeout  = DefaultDialTimeout
)

func upstreamDialer(timeout time.Duration) net.Dialer {
	if timeout <= 0 {
		timeout = DefaultDialTimeout
	}
	return net.Dialer{Timeout: timeout}
}
