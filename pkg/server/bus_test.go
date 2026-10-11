package server

import (
	"testing"
	"time"

	"github.com/lewtec/lewkit/x/event"
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
		t.Fatal("rejected transition published")
	case <-time.After(50 * time.Millisecond):
	}

	srv.ResetState()
	waitStatus(t, updates)
}

func waitStatus(t *testing.T, updates <-chan struct{}) {
	t.Helper()
	select {
	case _, ok := <-updates:
		if !ok {
			t.Fatal("status bus closed")
		}
	case <-time.After(time.Second):
		t.Fatal("status bus did not publish")
	}
}
