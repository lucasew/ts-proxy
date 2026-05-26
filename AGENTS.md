# Project Conventions

## Documentation
* **Go Code**: Always use standard Go line comments (`//`) for docstrings on exported functions, types, and variables. Do NOT use JSDoc-style comments (`/** ... */`) for Go code, as they are consistently rejected in this repository.
* **Content**: Docstrings should focus on the _why_ and the _flow_ of the code. Avoid stating the obvious (e.g., "Returns user" for `getUser`). Mention edge cases, side effects, and architectural context where relevant.

## Error Handling
* **Centralized Reporting**: The project enforces a single, centralized error-reporting function (`tsproxy.ReportError` or equivalent in the Go backend). All scattered error logging (e.g., `slog.Error`) must funnel through this centralized mechanism instead of being called directly at the call site.
* **No Silent Failures**: Unhandled errors or panics must not be ignored.

## Operational Memory
Where to find things in this repository:
* `cmd/ts-proxyd/main.go` -> Main application entry point, flag parsing, and configuration.
* `proxy.go` -> The core orchestrator (`TailscaleProxyServer`) managing the `tsnet` lifecycle and proxy connections.
* `tcp.go` -> Handlers and logic for the standard TCP proxy.
* `http.go` -> Handlers and logic for the HTTP proxy server.
* `.github/workflows/autorelease.yml` -> CI/CD pipeline and release definitions.
