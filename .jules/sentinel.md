# Sentinel Journal

- Fixed build by removing undefined `log` package usage in `http.go`, enforcing the project mandate to use `log/slog`.

## 2026-01-25 - Fix Header Spoofing via Case-Insensitive Headers
**Vulnerability:** The proxy sanitization logic only removed headers with exact canonical names (e.g., `Tailscale-User-Login`). Attackers could bypass this by sending variations like `Tailscale_User_Login` (underscores), which Go canonicalizes differently but upstream services might interpret as the same environment variable (e.g., `HTTP_TAILSCALE_USER_LOGIN`).
**Learning:** Header sanitization must account for how upstream applications normalize headers (e.g., converting underscores to hyphens). Relying solely on Go's canonicalization is insufficient when protecting legacy or PHP/CGI-like backends.
**Prevention:** Iterate over all request headers, normalize keys by converting to lowercase and replacing underscores with hyphens, and check against a deny-list of sensitive headers.
