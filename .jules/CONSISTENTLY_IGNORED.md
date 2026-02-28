# Consistently Ignored Changes

This file lists patterns of changes that have been consistently rejected by human reviewers. All agents MUST consult this file before proposing a new change. If a planned change matches any pattern described below, it MUST be abandoned.

## IGNORE: JSDoc Documentation Format

**- Pattern:** Using `/** ... */` JSDoc-style comments for Go code.
**- Justification:** Go code should use standard `//` comments for documentation to be compatible with `go doc` and idiomatic style.
**- Files Affected:** `*.go`

## IGNORE: Header Sanitization via Del()

**- Pattern:** Relying on `http.Header.Del()` to remove non-canonical or malformed headers (e.g. underscore variations like `Tailscale_User_Login`) for security sanitization.
**- Justification:** `Header.Del()` canonicalizes keys and fails to remove non-canonical variations that might be present in the map. Use `delete(headerMap, key)` instead.
**- Files Affected:** `http.go`

## IGNORE: Manual Context Cleanup

**- Pattern:** Manually calling `cancel()` on multiple error return paths in constructors or setup functions.
**- Justification:** Prone to errors if a path is missed. Use `defer func() { if err != nil { cancel() } }()` pattern instead.
**- Files Affected:** `*.go`

## IGNORE: CI/CD Workflow Overhaul

**- Pattern:** Replacing the existing granular `autorelease.yml` workflow steps with a single `mise run ci` command, or removing `scorecard.yml`.
**- Justification:** The project prefers the existing granular workflow structure and `scorecard.yml` should be preserved. Large overhauls of the CI configuration are consistently rejected.
**- Files Affected:** `.github/workflows/*.yml`, `mise.toml`
