package tsproxy

import (
	"net/http/httptest"
	"testing"

	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

func TestSetTailscaleHeaders_Sanitization(t *testing.T) {
	// Setup request
	req := httptest.NewRequest("GET", "http://example.com", nil)

	// Manually inject malicious headers with non-canonical casing
	// Go's http.Header is map[string][]string
	req.Header["tailscale-user-login"] = []string{"attacker@evil.com"}
	req.Header["Tailscale_User_Name"] = []string{"Attacker"} // Underscores!

	// Setup user info
	userInfo := &apitype.WhoIsResponse{
		UserProfile: &tailcfg.UserProfile{
			LoginName:     "user@example.com",
			DisplayName:   "User Name",
			ProfilePicURL: "http://example.com/pic.jpg",
		},
	}

	// Call function
	setTailscaleHeaders(req, userInfo)

	// Check if the malicious headers still exist
	if _, ok := req.Header["tailscale-user-login"]; ok {
		t.Errorf("Security risk: Failed to remove 'tailscale-user-login' header")
	}
	if _, ok := req.Header["Tailscale_User_Name"]; ok {
		t.Errorf("Security risk: Failed to remove 'Tailscale_User_Name' header")
	}

	// Verify authoritative headers are set correctly
	if got := req.Header.Get(TailscaleUserLoginHeader); got != "user@example.com" {
		t.Errorf("Expected authoritative header %q to be %q, got %q", TailscaleUserLoginHeader, "user@example.com", got)
	}
}
