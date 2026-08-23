package handler

import (
	"testing"
	"time"
)

func TestUpstreamDialerTimeout(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want time.Duration
	}{
		{name: "positive", in: 200 * time.Millisecond, want: 200 * time.Millisecond},
		{name: "zero falls back", in: 0, want: DefaultDialTimeout},
		{name: "negative falls back", in: -1, want: DefaultDialTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := upstreamDialer(tt.in)
			if d.Timeout != tt.want {
				t.Fatalf("Timeout = %v, want %v", d.Timeout, tt.want)
			}
		})
	}
}

func TestDialTimeoutAliases(t *testing.T) {
	if DefaultHTTPDialTimeout != DefaultDialTimeout {
		t.Fatalf("DefaultHTTPDialTimeout = %v, want %v", DefaultHTTPDialTimeout, DefaultDialTimeout)
	}
	if DefaultTCPDialTimeout != DefaultDialTimeout {
		t.Fatalf("DefaultTCPDialTimeout = %v, want %v", DefaultTCPDialTimeout, DefaultDialTimeout)
	}
}
