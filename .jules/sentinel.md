# Sentinel Journal

- Fixed build by removing undefined `log` package usage in `http.go`, enforcing the project mandate to use `log/slog`.

## 2026-01-25 - Fix Tailscale Header Spoofing
**Vulnerability:** The proxy only deleted canonical Tailscale headers (e.g., `Tailscale-User-Login`), allowing attackers to inject spoofed headers using non-canonical casing or underscores (e.g., `Tailscale_User_Login`) if the upstream server normalizes them.
**Learning:** `http.Header.Del` only removes the specific canonical key. Iterating and normalizing keys is required when the goal is to sanitize all variations of a header name that backend servers might interpret.
**Prevention:** Always normalize and iterate over headers when implementing a deny-list for sensitive headers, instead of relying on exact match deletion.
