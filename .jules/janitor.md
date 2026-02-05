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

## 2026-02-04 - Fix Context Leak in NewTailscaleProxyServer
**Issue:** `go vet` reported a possible context leak in `NewTailscaleProxyServer` because the `cancel` function returned by `context.WithCancel` was not called on error paths.

**Root Cause:** The `cancel` function was created early in the function but not invoked when returning errors for validation failures (e.g., missing address) or directory creation errors.

**Solution:** I added explicit `cancel()` calls before returning `nil, err` in all error branches of the `NewTailscaleProxyServer` function.

**Pattern:** When using `context.WithCancel` (or `WithTimeout`/`WithDeadline`), ensure the returned `cancel` function is called on all code paths, including early error returns, to prevent resource leaks.
