package tsproxy

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"syscall"
)

// ReportError is the centralized error reporting function.
// All code paths that handle unexpected errors MUST funnel through this function.
//
// Expected control-flow errors (context cancel, already-closed network
// objects, clean peer disconnect) are ignored so graceful shutdown and normal
// client hangups do not spam "unexpected error" logs when call sites forget
// to pre-filter.
func ReportError(err error, contextArgs ...any) {
	if err == nil || isExpectedError(err) {
		return
	}

	args := []any{"err", err}
	args = append(args, contextArgs...)

	slog.Error("unexpected error", args...)
}

func isExpectedError(err error) bool {
	// Shutdown / closed objects.
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, io.ErrClosedPipe) {
		return true
	}
	// Peer closed the stream cleanly (common on TCP proxy copy paths).
	if errors.Is(err, io.EOF) {
		return true
	}
	// Peer reset / local write after remote close — normal for reverse proxies.
	if errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNABORTED) {
		return true
	}
	return false
}
