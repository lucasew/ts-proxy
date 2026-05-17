package tsproxy

import "log/slog"

// ReportError centralizes error logging and tracking.
// By routing all errors here, we ensure consistent structured logging
// and make it easy to plug in a service like Sentry later.
func ReportError(msg string, args ...any) {
	slog.Error(msg, args...)
}
