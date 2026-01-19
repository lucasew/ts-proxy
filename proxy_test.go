package tsproxy

import (
	"testing"
)

func TestNewTailscaleProxyServer_Validation(t *testing.T) {
	tests := []struct {
		name        string
		options     TailscaleProxyServerOptions
		wantErr     bool
		expectedErr error
	}{
		{
			name: "Missing Address",
			options: TailscaleProxyServerOptions{
				Hostname: "test-host",
			},
			wantErr:     true,
			expectedErr: ErrInvalidUpstream,
		},
		{
			name: "Valid Address",
			options: TailscaleProxyServerOptions{
				Hostname: "test-host",
				Address:  "127.0.0.1:8080",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewTailscaleProxyServer(tt.options)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewTailscaleProxyServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != tt.expectedErr {
				t.Errorf("NewTailscaleProxyServer() error = %v, expectedErr %v", err, tt.expectedErr)
			}
			if !tt.wantErr && server == nil {
				t.Error("NewTailscaleProxyServer() returned nil server but expected success")
			}
			if !tt.wantErr && server != nil {
				// Cleanup context for successful creation to avoid leak in test
				if server.cancel != nil {
					server.cancel()
				}
			}
		})
	}
}
