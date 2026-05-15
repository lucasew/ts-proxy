package flagsource

import (
	"flag"

	"github.com/lucasew/ts-proxy/internal/configsources"
)

// FlagSource reads a single server configuration from command-line flags.
// It preserves backward compatibility with the original ts-proxyd CLI.
type FlagSource struct {
	network string
	address string
	hostname string
	funnel  bool
	tls     bool
	raw     bool
	stateDir string
	listen  string
}

// New creates a FlagSource and registers its flags on the default flag set.
// Must be called before flag.Parse().
func New() *FlagSource {
	s := &FlagSource{}
	flag.StringVar(&s.network, "net", "", "Network, for net.Dial")
	flag.StringVar(&s.address, "address", "", "Where to forward the connection")
	flag.StringVar(&s.hostname, "n", "", "Hostname in tailscale devices list")
	flag.BoolVar(&s.funnel, "f", false, "Enable tailscale funnel")
	flag.BoolVar(&s.tls, "t", false, "Enable HTTPS/TLS")
	flag.StringVar(&s.stateDir, "s", "", "State directory")
	flag.StringVar(&s.listen, "listen", "", "Port to listen")
	flag.BoolVar(&s.raw, "raw", false, "Disable HTTP handling")
	return s
}

func (s *FlagSource) Name() string { return "flags" }

// Discover returns a single server config when -address is provided,
// or an empty list when no flag-based server is defined.
func (s *FlagSource) Discover() ([]configsources.ServerConfig, error) {
	if s.address == "" {
		return nil, nil
	}
	cfg := configsources.ServerConfig{
		Name:         s.hostname,
		Network:      s.network,
		Address:      s.address,
		Hostname:     s.hostname,
		EnableFunnel: s.funnel,
		EnableTLS:    s.tls,
		EnableHTTP:   !s.raw,
		StateDir:     s.stateDir,
		Listen:       s.listen,
	}
	return []configsources.ServerConfig{cfg}, nil
}
