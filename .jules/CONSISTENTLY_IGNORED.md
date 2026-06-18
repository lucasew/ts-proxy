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
**- Justification:** Violates the strict global instruction to "Never ignore errors" and "No silent failures". Errors from deferred cleanups must be explicitly checked and reported using the project's centralized error-reporting mechanism (e.g., `ReportError(err, ...)`).
**- Files Affected:** All `.go` files.

## IGNORE: Manual Dependency Digest Updates

**- Pattern:** Manually updating GitHub Action hashes or dependency versions in CI workflows.
**- Justification:** The project uses Renovate for automated dependency updates. Manual version bumps conflict with automated PRs and are consistently rejected.
**- Files Affected:** `.github/workflows/autorelease.yml`
