package configsources

import (
	"fmt"
	"path/filepath"
)

// ServerConfig describes a single proxy server to launch.
type ServerConfig struct {
	// Name is a human-readable identifier, used for logging and state dir derivation.
	Name string
	// Network for net.Dial to the upstream (e.g. "tcp").
	Network string
	// Address of the upstream to forward to.
	Address string
	// Hostname in the Tailscale devices list.
	Hostname string
	// EnableFunnel enables Tailscale Funnel.
	EnableFunnel bool
	// EnableTLS enables HTTPS/TLS via Tailscale certs.
	EnableTLS bool
	// EnableHTTP enables HTTP reverse proxy logic (vs raw TCP).
	EnableHTTP bool
	// StateDir is where to store Tailscale state for this server.
	StateDir string
	// Listen address to bind the server.
	Listen string
}

// Source discovers server configurations from a particular backend.
type Source interface {
	// Name returns a human-readable name for this source (e.g. "flags", "envvar").
	Name() string
	// Discover returns all server configurations found by this source.
	Discover() ([]ServerConfig, error)
}

var sources []Source

// Register adds a configuration source to the global registry.
// Sources are queried in registration order by DiscoverAll.
func Register(s Source) {
	sources = append(sources, s)
}

// DiscoverAll queries every registered source and returns the union of all
// discovered server configurations.
func DiscoverAll() ([]ServerConfig, error) {
	var configs []ServerConfig
	for _, s := range sources {
		found, err := s.Discover()
		if err != nil {
			return nil, fmt.Errorf("source %s: %w", s.Name(), err)
		}
		configs = append(configs, found...)
	}
	return configs, nil
}

// ApplyDefaults fills in missing values on each config using sensible defaults.
// baseStateDir, if non-empty, is used to derive per-server state directories.
func ApplyDefaults(configs []ServerConfig, baseStateDir string) {
	for i := range configs {
		c := &configs[i]
		if c.Name == "" {
			c.Name = "default"
		}
		if c.Hostname == "" {
			c.Hostname = c.Name
		}
		if c.Network == "" {
			c.Network = "tcp"
		}
		if c.StateDir == "" && baseStateDir != "" {
			c.StateDir = filepath.Join(baseStateDir, c.Name)
		}
		if c.Listen == "" && c.EnableHTTP {
			if c.EnableFunnel || c.EnableTLS {
				c.Listen = ":443"
			} else {
				c.Listen = ":80"
			}
		}
	}
}

// ResetRegistry clears all registered sources. Intended for tests.
func ResetRegistry() {
	sources = nil
}
