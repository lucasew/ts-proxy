## 2026-01-11 - Replace Magic Strings with Named Constants for Headers
**Issue:** The `setTailscaleHeaders` function in `http.go` used hardcoded string literals ("magic strings") for several HTTP header names, such as "Tailscale-User-Login" and "Tailscale-User-Name".

**Root Cause:** This is a common occurrence in initial development where hardcoded values are used for expediency. Over time, this creates a maintenance risk, as typos in the strings would not be caught by the compiler and could lead to subtle bugs.

**Solution:** I defined a `const` block at the package level in `http.go` to declare these header names as named constants. The `setTailscaleHeaders` function was then refactored to use these constants, ensuring consistency and compile-time checking for any future modifications.

**Pattern:** For this codebase, any frequently used and critical string literals, especially those representing keys or identifiers like HTTP headers, should be defined as constants. This centralizes their definition, reduces the risk of typos, and improves code readability and maintainability.

## 2026-01-25 - Refactor Main to Remove Global State and Dead Code

**Issue:** The `cmd/ts-proxyd/main.go` file used a global `options` variable and an `init` function to parse flags, which obscures control flow and introduces side effects on import. Additionally, it contained a dead code block checking an unassigned `err` variable.

**Root Cause:** This likely originated from a quick initial implementation where `init` was used to set up the environment before `main`, but without strict adherence to structured lifecycle management.

**Solution:** I moved the flag parsing, validation, and the `options` variable into the `main` function. This eliminates global state and makes the execution flow linear and explicit. I also removed the unreachable error check.

**Pattern:** Avoid using `init` functions for application configuration or flag parsing. Instead, explicitly handle initialization in `main` or a dedicated configuration function to improve testability and readability. Dead code should be aggressively removed to prevent confusion.
- 2026-06-07: Ensure mise.toml task definitions use wildcard dependencies for grouped jobs, and explicitly ignore unhandled Close errors to satisfy linters.

## 2026-06-29 - Centralize error reporting
**Issue:** Errors were being logged directly using `slog.Error` across various files in the codebase, leading to a decentralized approach.

**Root Cause:** The project conventions (memory and AGENTS.md equivalent) mandate a single, centralized error-reporting function (`tsproxy.ReportError`) so that errors can easily be wired to external monitoring services like Sentry. Using scattered `slog.Error` calls violates this policy.

**Solution:** I created a `tsproxy` package in `pkg/tsproxy` with a `ReportError` function that delegates to `slog.Error`. I then migrated all scattered `slog.Error` calls in `cmd/ts-proxyd/root.go`, `pkg/server/supervisor.go`, `pkg/handler/http.go`, and `pkg/handler/tcp.go` to use `tsproxy.ReportError` instead, preserving their arguments. I also added a unit test `error_test.go` to ensure `ReportError` accurately records the error and additional fields.

**Pattern:** All scattered error logging (e.g., `slog.Error`) must funnel through a centralized mechanism (`tsproxy.ReportError` or equivalent) rather than being called directly at the call site.
