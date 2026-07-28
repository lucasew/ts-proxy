package ctxwait

import (
	"context"
	"testing"
	"time"
)

func TestDelayCompletes(t *testing.T) {
	ctx := t.Context()
	start := time.Now()
	if !Delay(ctx, 20*time.Millisecond) {
		t.Fatal("Delay returned false, want true after delay")
	}
	if elapsed := time.Since(start); elapsed < 20*time.Millisecond {
		t.Fatalf("Delay returned too early: %v", elapsed)
	}
}

func TestDelayCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	start := time.Now()
	if Delay(ctx, time.Hour) {
		t.Fatal("Delay returned true, want false after cancel")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("Delay ignored cancel, took %v", elapsed)
	}
}

func TestDelayCancelDuringWait(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan bool, 1)
	go func() {
		done <- Delay(ctx, time.Hour)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case got := <-done:
		if got {
			t.Fatal("Delay returned true after mid-wait cancel, want false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Delay did not return after cancel")
	}
}
