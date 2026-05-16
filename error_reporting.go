package tsproxy

import (
	"log/slog"
)

// ReportError centralizes error reporting for the application.
// Following the Single Responsibility Principle, this function decouples
// error handling/reporting from business logic, allowing a single integration
// point for observability tools (like Sentry) in the future.
func ReportError(err error, msg string, args ...any) {
	if err == nil {
		return
	}
	argsWithErr := append(args, "err", err)
	slog.Error(msg, argsWithErr...)
}
