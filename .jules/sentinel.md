# Sentinel Journal

- Fixed build by removing undefined `log` package usage in `http.go`, enforcing the project mandate to use `log/slog`.

## 2026-01-11 - Sanitize incoming headers to prevent spoofing
**Vulnerability:** The proxy was not sanitizing incoming headers before setting the authoritative Tailscale identity headers. An attacker could send headers with different casing or underscores (e.g., `Tailscale_User_Login`) to bypass the case-sensitive deletion and spoof their identity to the upstream service.

**Learning:** The previous implementation only deleted exact-match headers, which is insufficient. It's crucial to normalize headers (e.g., to lowercase, replacing underscores) before checking them against a deny-list to ensure all variations are caught.

**Prevention:** When handling headers or any user-controllable input that has security implications, always normalize it to a canonical form before processing. For headers, this means converting to a consistent case and accounting for separators like hyphens and underscores.
