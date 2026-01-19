## 2026-01-11 - Replace Magic Strings with Named Constants for Headers
**Issue:** The `setTailscaleHeaders` function in `http.go` used hardcoded string literals ("magic strings") for several HTTP header names, such as "Tailscale-User-Login" and "Tailscale-User-Name".

**Root Cause:** This is a common occurrence in initial development where hardcoded values are used for expediency. Over time, this creates a maintenance risk, as typos in the strings would not be caught by the compiler and could lead to subtle bugs.

**Solution:** I defined a `const` block at the package level in `http.go` to declare these header names as named constants. The `setTailscaleHeaders` function was then refactored to use these constants, ensuring consistency and compile-time checking for any future modifications.

**Pattern:** For this codebase, any frequently used and critical string literals, especially those representing keys or identifiers like HTTP headers, should be defined as constants. This centralizes their definition, reduces the risk of typos, and improves code readability and maintainability.

## 2026-01-19 - Fix Context Leak in NewTailscaleProxyServer
**Issue:** `go vet` reported a potential context leak in `NewTailscaleProxyServer` because the `cancel` function associated with the new context was not called on early error returns.
**Root Cause:** The `NewTailscaleProxyServer` function initializes a cancellable context early on but performs validation checks afterwards. If these checks failed (e.g., missing address or directory creation failure), the function returned without calling `cancel()`, leaving the context active indefinitely (or until the parent context expires).
**Solution:** I inserted `cancel()` calls before the return statements in the error handling blocks for `options.Address` validation and `os.MkdirAll`. This ensures the context is properly cleaned up if the server fails to initialize.
**Pattern:** Always ensure that resources or contexts acquired at the beginning of a function are properly released or cancelled on all exit paths, especially error paths.
