## IGNORE: JSDoc-style Comments in Go Code

**- Pattern:** Using JSDoc-style comments (`/** ... */`) to document Go functions or structs.
**- Justification:** Go enforces the standard `//` line comment format for docstrings.
**- Files Affected:** All `.go` files.

## IGNORE: Using http.Header.Del() for Security Sanitization

**- Pattern:** Relying on `http.Header.Del(key)` to remove spoofed headers.
**- Justification:** `Del()` canonicalizes the key, failing to remove non-canonical variations (like those with underscores). Use `delete(headerMap, key)` instead.
**- Files Affected:** All `.go` files handling HTTP requests.

## IGNORE: Manual Context Cleanup Without Defer

**- Pattern:** Calling a `cancel()` context function manually at multiple exit points without using `defer`.
**- Justification:** This leads to context leaks if an error path returns early without calling `cancel()`. Always use `defer` to ensure proper cleanup.
**- Files Affected:** All `.go` files managing contexts.

## IGNORE: Replacing Granular CI Workflow Steps

**- Pattern:** Consolidating multiple CI workflow steps in `.github/workflows/autorelease.yml` into a single `mise run ci` command.
**- Justification:** Granular steps are required for proper reporting and tracing in GitHub Actions.
**- Files Affected:** `.github/workflows/autorelease.yml`

## IGNORE: Bundling Out-of-Scope CI Changes

**- Pattern:** Modifying or moving the `docker build` step or cross-compilation loops in `autorelease.yml` within PRs focused on unrelated tasks (e.g., documentation, refactoring, or security fixes).
**- Justification:** Changes must remain strictly scoped to the primary objective. Bundling unrelated CI workflow changes pollutes the diff.
**- Files Affected:** `.github/workflows/autorelease.yml`

## IGNORE: Logging Ignorable Errors in Defers

**- Pattern:** Adding explicit error reporting (e.g., `ReportError(err)`) to deferred cleanup functions like `ln.Close()` instead of explicitly ignoring them.
**- Justification:** Failing to close an already-closed or terminating connection is generally noise. Such errors should be explicitly ignored (e.g., `_ = ln.Close()`) to satisfy linters without cluttering logs.
**- Files Affected:** All `.go` files handling network listeners or connections.

## IGNORE: Direct slog.Error Calls

**- Pattern:** Using `slog.Error` directly to log errors instead of the centralized error reporter.
**- Justification:** The project mandates a centralized error-reporting function (e.g., `tsproxy.ReportError`) to decouple error handling from business logic and ensure consistent observability.
**- Files Affected:** All `.go` files.

## IGNORE: Manual Dependency Digest Updates

**- Pattern:** Manually updating GitHub Action hashes or dependency versions in CI workflows.
**- Justification:** The project uses Renovate for automated dependency updates. Manual bumps conflict with automated PRs and are rejected.
**- Files Affected:** `.github/workflows/autorelease.yml`
