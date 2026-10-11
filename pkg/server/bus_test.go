package server

import (
	"testing"
	"time"

	"github.com/lewtec/lewkit/x/event"
	"github.com/stretchr/testify/require"
)

func TestTransitionPublishesStatus(t *testing.T) {
	bus := event.New[struct{}]()
	srv := NewServer("web", Options{Hostname: "web"})
	srv.bus = bus
	updates := bus.Subscribe(t.Context())

	srv.mustTransition(StateStarting)
	waitStatus(t, updates)

	srv.mustTransition(StateStarting)
	select {
	case <-updates:
		require.Fail(t, "rejected transition published")
	case <-time.After(50 * time.Millisecond):
	}

	srv.ResetState()
	waitStatus(t, updates)
}

func waitStatus(t *testing.T, updates <-chan struct{}) {
	t.Helper()
	select {
	case _, ok := <-updates:
		require.True(t, ok, "status bus closed")
	case <-time.After(time.Second):
		require.Fail(t, "status bus did not publish")
	}
}
