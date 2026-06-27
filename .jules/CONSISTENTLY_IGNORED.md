## IGNORE: Bundling Out-of-Scope CI Changes

**- Pattern:** Modifying `.github/workflows/autorelease.yml` (e.g., moving `docker build` steps or modifying cross-compilation loops) in a PR focused on an unrelated task like documentation, refactoring, or security fixes.
**- Justification:** Changes must remain strictly scoped to the primary objective. Bundling unrelated CI workflow changes pollutes the diff and violates scope boundaries.
**- Files Affected:** `.github/workflows/autorelease.yml`

## IGNORE: Replacing Granular CI Workflow Steps

**- Pattern:** Consolidating multiple CI workflow steps in `.github/workflows/autorelease.yml` into a single `mise run ci` or `mise run build-all:go` command.
**- Justification:** Granular steps are required for proper reporting and tracing in GitHub Actions. Grouping them inside single execution blocks obscures step-by-step CI visibility.
**- Files Affected:** `.github/workflows/autorelease.yml`

## IGNORE: Ignoring Errors in Deferred Cleanups

**- Pattern:** Explicitly ignoring errors from deferred cleanup functions, such as using `defer func() { _ = ln.Close() }()`.
**- Justification:** Violates the strict global instruction to "Never ignore errors" and "No silent failures". Errors from deferred cleanups must be explicitly checked and reported using the project's centralized error-reporting mechanism.
**- Files Affected:** All `.go` files.

## IGNORE: Direct slog.Error Calls

**- Pattern:** Using `slog.Error` directly to log errors instead of a centralized error reporter.
**- Justification:** The project mandates a single, centralized error-reporting function (e.g., `tsproxy.ReportError`) to decouple error handling from business logic and ensure consistent observability.
**- Files Affected:** All `.go` files.

## IGNORE: Using http.Header.Del() for Security Sanitization

**- Pattern:** Relying on `http.Header.Del(key)` to remove spoofed headers.
**- Justification:** `Del()` canonicalizes the key, failing to remove non-canonical variations (like those with underscores). Use `delete(headerMap, key)` instead.
**- Files Affected:** All `.go` files handling HTTP requests.

## IGNORE: JSDoc-style Comments in Go Code

**- Pattern:** Using JSDoc-style comments (`/** ... */`) to document Go functions or structs.
**- Justification:** Go enforces the standard `//` line comment format for docstrings.
**- Files Affected:** All `.go` files.

## IGNORE: Manual Dependency Updates

**- Pattern:** Manually updating dependency versions or GitHub Action digests in CI workflows or Dockerfiles.
**- Justification:** The project uses Renovate for automated dependency updates. Manual version bumps conflict with automated PRs and unpin pinned digests.
**- Files Affected:** `.github/workflows/autorelease.yml`, `Dockerfile`, `go.mod`.

## IGNORE: Manual Context Cleanup Without Defer

**- Pattern:** Calling a `cancel()` context function manually at multiple exit points without using `defer`.
**- Justification:** This leads to context leaks if an error path returns early without calling `cancel()`. Always use `defer` to ensure proper cleanup.
**- Files Affected:** All `.go` files managing contexts.
