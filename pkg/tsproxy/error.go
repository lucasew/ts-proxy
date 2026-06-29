package tsproxy

import (
	"log/slog"
)

// ReportError is the centralized error reporting function for ts-proxy.
// All code paths that handle unexpected errors MUST funnel through this function.
func ReportError(msg string, err error, attrs ...any) {
	// If Sentry were integrated, this is where we would send the error to Sentry.
	// Currently it just logs the error with context via slog.
	args := []any{"err", err}
	args = append(args, attrs...)
	slog.Error(msg, args...)
}
