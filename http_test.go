package tsproxy

import (
	"net/http/httptest"
	"testing"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

func TestTailscaleHeaderSpoofing(t *testing.T) {
	// Setup
	req := httptest.NewRequest("GET", "http://example.com", nil)

	// Add a "spoofed" header using underscores.
	// We use direct map assignment or Set.
	// Set canonicalizes to "Tailscale_user_login" (capital T, rest lowercase unless hyphen).
	// This is distinct from "Tailscale-User-Login".
	spoofedKey := "Tailscale_User_Login"
	req.Header.Set(spoofedKey, "hacker")

	// Ensure the header is present as we expect (canonicalized by Go)
	// "Tailscale_User_Login" -> "Tailscale_user_login"
	if req.Header.Get(spoofedKey) != "hacker" {
		t.Fatalf("Failed to set test header %s", spoofedKey)
	}

	userInfo := &apitype.WhoIsResponse{
		UserProfile: &tailcfg.UserProfile{
			LoginName:     "user@example.com",
			DisplayName:   "User Name",
			ProfilePicURL: "http://example.com/pic.jpg",
		},
	}

	// Act
	setTailscaleHeaders(req, userInfo)

	// Assert
	// We expect the spoofed header to be REMOVED.
	val := req.Header.Get(spoofedKey)
	if val != "" {
		t.Errorf("Spoofed header persisted with value: %s", val)
	}

    // Verify legitimate header is present
    if req.Header.Get(TailscaleUserLoginHeader) != "user@example.com" {
        t.Errorf("Expected %s to be set correctly", TailscaleUserLoginHeader)
    }
}
