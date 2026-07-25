package tsproxy

import (
	"context"
	"errors"
	"log/slog"
	"net"
)

// ReportError is the centralized error reporting function.
// All code paths that handle unexpected errors MUST funnel through this function.
//
// Expected control-flow errors (context cancel, already-closed network
// objects) are ignored so graceful shutdown does not spam "unexpected error"
// logs when call sites forget to pre-filter.
func ReportError(err error, contextArgs ...any) {
	if err == nil || isExpectedError(err) {
		return
	}

	args := []any{"err", err}
	args = append(args, contextArgs...)

	slog.Error("unexpected error", args...)
}

func isExpectedError(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, net.ErrClosed)
}
