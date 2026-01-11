package tsproxy

import (
	"net/http"
	"testing"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tailcfg"
)

func TestSetTailscaleHeaders(t *testing.T) {
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Tailscale-User-Login", "spoofed-user")
	req.Header.Set("TAILSCALE-USER-NAME", "Spoofed User")
	req.Header.Set("tailscale-user-profile-pic", "http://spoofed.com/pic.jpg")
	req.Header.Set("X-Custom-Header", "should-remain")

	userInfo := &apitype.WhoIsResponse{
		UserProfile: &tailcfg.UserProfile{
			LoginName:     "real-user",
			DisplayName:   "Real User",
			ProfilePicURL: "http://real.com/pic.jpg",
		},
	}

	setTailscaleHeaders(req, userInfo)

	if login := req.Header.Get("Tailscale-User-Login"); login != "real-user" {
		t.Errorf("Tailscale-User-Login header is incorrect, got: %s, want: %s.", login, "real-user")
	}
	if name := req.Header.Get("Tailscale-User-Name"); name != "Real User" {
		t.Errorf("Tailscale-User-Name header is incorrect, got: %s, want: %s.", name, "Real User")
	}
	if pic := req.Header.Get("Tailscale-User-Profile-Pic"); pic != "http://real.com/pic.jpg" {
		t.Errorf("Tailscale-User-Profile-Pic header is incorrect, got: %s, want: %s.", pic, "http://real.com/pic.jpg")
	}
	if custom := req.Header.Get("X-Custom-Header"); custom != "should-remain" {
		t.Errorf("X-Custom-Header header was removed, but it should have been kept.")
	}
}
