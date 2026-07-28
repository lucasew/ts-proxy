// Package ctxwait provides context-aware timing helpers shared by packages
// that need a cancellable delay without leaking timers.
package ctxwait

import (
	"context"
	"time"
)

// Delay blocks until d elapses or ctx is cancelled.
// Returns true when the delay completed (caller may proceed), false when
// ctx was cancelled (caller should stop).
//
// Uses time.NewTimer instead of time.After so a cancelled wait does not leave
// a timer and its values channel live until the delay fires — important in
// long-lived retry/restart loops.
func Delay(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		if !timer.Stop() {
			// timer already fired; drain so the channel can be GC'd
			select {
			case <-timer.C:
			default:
			}
		}
		return false
	}
}
