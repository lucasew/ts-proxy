# Project Conventions

This file acts as the source of truth for project-specific rules, guidelines, and memory for AI agents.

## Code Conventions
- **Documentation:** For Go code, strictly use standard Go line comments (`//`). Do not use JSDoc/TSDoc format (`/** ... */`) under any circumstance.
- **Error Reporting:** The project mandates a single, centralized error-reporting function (`tsproxy.ReportError` or equivalent in the Go backend). All scattered error logging (e.g., `slog.Error`) MUST funnel through this centralized mechanism.
- **Header Sanitization:** When iterating over an `http.Header` map to sanitize headers (e.g., removing spoofed variations), use the native `delete(headerMap, key)` function instead of `header.Del(key)`.
- **Initialization:** Avoid using `init()` functions for flag parsing and application configuration to prevent global state and side effects. Favor explicit initialization within `main()`.
- **Deferred Cleanup:** To prevent context leaks in functions that return an error, use named return parameters and a deferred closure (e.g., `defer func() { if err != nil { cancel() } }()`). When handling `defer` cleanup methods that return errors (like `ln.Close()`), wrap them in an anonymous function to explicitly ignore or handle the error (e.g., `defer func() { _ = ln.Close() }()`) to satisfy linter checks.

## Architecture
- **Entrypoint:** `cmd/ts-proxyd/main.go`
- **Proxy Logic:** `proxy.go`, `http.go`, `tcp.go`

## Tooling
- All task execution must be performed using `mise` (e.g., `mise run lint`).
- The project strictly uses `workspaced` via `mise` (`workspaced codebase lint` and `workspaced codebase format`).
- Do not run `go mod tidy` unless a dependency update is explicitly intended.
