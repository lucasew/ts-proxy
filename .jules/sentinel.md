# Sentinel Journal

- Fixed build by removing undefined `log` package usage in `http.go`, enforcing the project mandate to use `log/slog`.

## 2026-01-25 - Fix Tailscale header spoofing via underscore variations
**Vulnerability:** Attackers could spoof `Tailscale-User-Login` and other identity headers by sending them with underscores (e.g., `Tailscale_User_Login`). The proxy's previous sanitization only removed canonical headers, allowing non-canonical variations to pass through to upstream services (like PHP/Apache) which might normalize them and treat them as trusted.
**Learning:** Go's `http.Header.Del` canonicalizes keys, which is insufficient for scrubbing non-canonical headers that are preserved in the map. Direct map iteration and deletion is required for robust sanitization.
**Prevention:** When implementing security headers that override user input, always iterate through *all* inbound headers and sanitize based on normalized keys (e.g., converting to a common format) before setting authoritative values.

## 2026-02-05 - Fix TCP proxy potential DoS (Torshammer) via idle timeouts
**Vulnerability:** The TCP proxy logic used `io.CopyBuffer` without setting deadlines on the connections. This allowed an attacker to open connections and keep them idle indefinitely (Torshammer/Slowloris style), potentially exhausting file descriptors or memory.
**Learning:** `io.Copy` and `io.CopyBuffer` in Go do not handle timeouts automatically. They block until `Read` returns an error or EOF.
**Prevention:** Always wrap `net.Conn` with logic that sets `SetReadDeadline` and `SetWriteDeadline` on every IO operation when building proxies or long-lived connection handlers.
