# Sentinel Journal

- Fixed build by removing undefined `log` package usage in `http.go`, enforcing the project mandate to use `log/slog`.

## 2026-01-19 - Robust Tailscale Header Sanitization

**Vulnerability:** The proxy relied on standard `Header.Del` to remove potential spoofed Tailscale headers. However, `Header.Del` only removes canonicalized keys. If an attacker sent headers with non-canonical casing or variations (e.g., `Tailscale_User_Name`), and the upstream application normalized these differently, it could lead to header spoofing.

**Learning:** Relying on framework defaults for security sanitization can be insufficient when dealing with inputs that might be interpreted differently by downstream systems. Robust sanitization often requires manual normalization and exhaustive checking.

**Prevention:** Always normalize inputs to a canonical form before validating or sanitizing against a deny-list, especially when bridging between systems that might have different parsing rules.
