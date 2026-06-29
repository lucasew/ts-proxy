package tsproxy

import (
	"log/slog"
)

// ReportError centralizes error reporting for the application.
// Currently it delegates to slog.Error, but in a production setup it would
// ideally wire to something like Sentry (Sentry.captureException).
func ReportError(msg string, err error, args ...any) {
	allArgs := []any{"err", err}
	allArgs = append(allArgs, args...)
	slog.Error(msg, allArgs...)
}
