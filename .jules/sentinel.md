# Sentinel Journal

- Fixed build by removing undefined `log` package usage in `http.go`, enforcing the project mandate to use `log/slog`.

## 2026-01-25 - Fix Tailscale header spoofing via underscore variations
**Vulnerability:** Attackers could spoof `Tailscale-User-Login` and other identity headers by sending them with underscores (e.g., `Tailscale_User_Login`). The proxy's previous sanitization only removed canonical headers, allowing non-canonical variations to pass through to upstream services (like PHP/Apache) which might normalize them and treat them as trusted.
**Learning:** Go's `http.Header.Del` canonicalizes keys, which is insufficient for scrubbing non-canonical headers that are preserved in the map. Direct map iteration and deletion is required for robust sanitization.
**Prevention:** When implementing security headers that override user input, always iterate through *all* inbound headers and sanitize based on normalized keys (e.g., converting to a common format) before setting authoritative values.
