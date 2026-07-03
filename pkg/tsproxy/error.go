package tsproxy

import (
	"log/slog"
)

// ReportError is the centralized error reporting function.
// All code paths that handle unexpected errors MUST funnel through this function.
func ReportError(err error, contextArgs ...any) {
	if err == nil {
		return
	}

	args := []any{"err", err}
	args = append(args, contextArgs...)

	slog.Error("unexpected error", args...)
}
