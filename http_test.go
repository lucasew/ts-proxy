package tsproxy

import (
	"net/http"
	"testing"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

func TestSetTailscaleHeaders(t *testing.T) {
	// 1. Create a request with "spoofed" headers
	r, err := http.NewRequest("GET", "http://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Add canonical header (will be overwritten by existing logic)
	r.Header.Set("Tailscale-User-Login", "hacker@evil.com")

	// Add non-canonical header (might bypass existing logic)
	r.Header["Tailscale_User_Login"] = []string{"hacker@evil.com"}
    // Some servers might treat underscores as dashes, so this is a spoofing vector.

	// 2. Mock WhoIsResponse
	userInfo := &apitype.WhoIsResponse{
		UserProfile: &tailcfg.UserProfile{
			ID:            123,
			LoginName:     "alice@example.com",
			DisplayName:   "Alice",
			ProfilePicURL: "https://example.com/alice.png",
		},
	}

	// 3. Call the function under test
	setTailscaleHeaders(r, userInfo)

	// 4. Assertions
	// Check the canonical header is correct
	if got := r.Header.Get("Tailscale-User-Login"); got != "alice@example.com" {
		t.Errorf("Tailscale-User-Login = %q; want %q", got, "alice@example.com")
	}

	// Check that the spoofed non-canonical header is GONE
	if _, exists := r.Header["Tailscale_User_Login"]; exists {
		t.Errorf("Tailscale_User_Login header was not removed!")
	}
}
