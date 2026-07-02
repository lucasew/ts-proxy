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
## 2026-07-02 - Optimize sync.Pool usage to reduce interface boxing allocations
**Issue:** The `bufferPool` in `pkg/handler/tcp.go` was storing `[]byte` values directly.

**Root Cause:** When `sync.Pool.Get` and `sync.Pool.Put` are used with a struct or slice (like `[]byte`), Go wraps them in an empty interface (`interface{}`), causing memory allocations (interface boxing) every time they are retrieved or stored.

**Solution:** I changed the `bufferPool.New` function to return a pointer to the slice (`*[]byte`), and refactored the TCP handler's `cp` function to cast the retrieved item to `*[]byte` and dereference it. This avoids interface boxing allocations.

**Pattern:** When using `sync.Pool` for byte slices or structs, always store pointers (e.g., `*[]byte`) instead of the actual values to avoid unnecessary allocations during interface boxing.
