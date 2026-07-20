package main

import (
	"testing"
)

func TestDefaultConfigPathsPreferWorkingDirectory(t *testing.T) {
	paths := defaultConfigPaths()
	if len(paths) < 3 {
		t.Fatalf("defaultConfigPaths() len = %d, want at least 3", len(paths))
	}
	if paths[0] != "." {
		t.Errorf("first search path = %q, want %q so cwd wins over system paths", paths[0], ".")
	}
	// Home before /etc so a user config still beats the system file.
	homeIdx, etcIdx := -1, -1
	for i, p := range paths {
		if p == "$HOME/.config/ts-proxy" {
			homeIdx = i
		}
		if p == "/etc/ts-proxy" {
			etcIdx = i
		}
	}
	if homeIdx < 0 || etcIdx < 0 {
		t.Fatalf("paths = %v, want both $HOME/.config/ts-proxy and /etc/ts-proxy", paths)
	}
	if homeIdx > etcIdx {
		t.Errorf("home path index %d after /etc index %d; user config should win over system", homeIdx, etcIdx)
	}
}
