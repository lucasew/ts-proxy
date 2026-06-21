package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

var slugPattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// Config is the top-level configuration for ts-proxy.
type Config struct {
	StateDir   string                  `mapstructure:"state_dir" yaml:"state_dir"`
	StopOnFail bool                    `mapstructure:"stop_on_fail" yaml:"stop_on_fail"`
	Tokens     map[string]TokenConfig  `mapstructure:"tokens" yaml:"tokens"`
	Servers    map[string]ServerConfig `mapstructure:"servers" yaml:"servers"`
}

// TokenConfig defines a Tailscale authentication token.
// One token can be referenced by many servers (1:n relationship).
type TokenConfig struct {
	AuthKey string `mapstructure:"auth_key" yaml:"auth_key"`
}

// ServerConfig defines a single Tailscale node with its handlers.
type ServerConfig struct {
	Hostname string          `mapstructure:"hostname" yaml:"hostname"`
	Token    string          `mapstructure:"token" yaml:"token"`
	Handlers []HandlerConfig `mapstructure:"handlers" yaml:"handlers"`
}

// HandlerConfig defines how a handler listens and where it forwards traffic.
type HandlerConfig struct {
	Type            string `mapstructure:"type" yaml:"type"`
	Listen          string `mapstructure:"listen" yaml:"listen"`
	UpstreamAddress string `mapstructure:"upstream_address" yaml:"upstream_address"`
	UpstreamNetwork string `mapstructure:"upstream_network" yaml:"upstream_network"`
	Funnel          bool   `mapstructure:"funnel" yaml:"funnel"`
	TLS             bool   `mapstructure:"tls" yaml:"tls"`
}

// ValidateSlug checks that a name contains only letters, numbers, and underscores.
func ValidateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if !slugPattern.MatchString(slug) {
		return fmt.Errorf("name %q is invalid: must contain only letters, numbers, and underscores", slug)
	}
	return nil
}

// SetDefaults fills in default values for unset fields.
func (c *Config) SetDefaults() {
	if c.StateDir == "" {
		c.StateDir = "/var/lib/ts-proxy"
	}
	if c.Tokens == nil {
		c.Tokens = make(map[string]TokenConfig)
	}
	if c.Servers == nil {
		c.Servers = make(map[string]ServerConfig)
	}
	for name, srv := range c.Servers {
		if srv.Hostname == "" {
			srv.Hostname = name
		}
		for i := range srv.Handlers {
			h := &srv.Handlers[i]
			if h.UpstreamNetwork == "" {
				h.UpstreamNetwork = "tcp"
			}
			if h.Listen == "" && h.Type == "http" {
				if h.TLS || h.Funnel {
					h.Listen = ":443"
				} else {
					h.Listen = ":80"
				}
			}
		}
		c.Servers[name] = srv
	}
}

// ExpandEnv expands environment variable references (using ${VAR} or $VAR syntax)
// in all string fields of the configuration.
//
// Supported fields:
//   - state_dir
//   - tokens.<name>.auth_key
//   - servers.<name>.hostname
//   - servers.<name>.token
//   - servers.<name>.handlers[].type
//   - servers.<name>.handlers[].listen
//   - servers.<name>.handlers[].upstream_address
//   - servers.<name>.handlers[].upstream_network
//
// It collects errors for every field that references an undefined variable and
// returns them joined with errors.Join (so all problems are reported at once).
func (c *Config) ExpandEnv() error {
	// Helper that expands a single value and reports missing vars with context.
	expand := func(context string, original string) (string, error) {
		if original == "" {
			return "", nil
		}

		missing := []string{}
		seen := map[string]bool{}

		expanded := os.Expand(original, func(key string) string {
			if seen[key] {
				val, _ := os.LookupEnv(key)
				return val
			}
			seen[key] = true

			if val, ok := os.LookupEnv(key); ok {
				return val
			}
			missing = append(missing, key)
			return ""
		})

		if len(missing) > 0 {
			return expanded, fmt.Errorf("%s references undefined environment variable(s): %s (original: %q)",
				context, strings.Join(missing, ", "), original)
		}
		return expanded, nil
	}

	var expandErrs []error
	collect := func(err error) {
		if err != nil {
			expandErrs = append(expandErrs, err)
		}
	}

	// Top level
	var err error
	c.StateDir, err = expand("state_dir", c.StateDir)
	collect(err)

	// Tokens
	for name, token := range c.Tokens {
		token.AuthKey, err = expand(fmt.Sprintf("token %q auth_key", name), token.AuthKey)
		c.Tokens[name] = token
		collect(err)
	}

	// Servers + handlers
	for sname, srv := range c.Servers {
		srv.Hostname, err = expand(fmt.Sprintf("server %q hostname", sname), srv.Hostname)
		collect(err)

		srv.Token, err = expand(fmt.Sprintf("server %q token", sname), srv.Token)
		collect(err)

		for i := range srv.Handlers {
			h := &srv.Handlers[i]
			prefix := fmt.Sprintf("server %q handler[%d]", sname, i)

			h.Type, err = expand(prefix+" type", h.Type)
			collect(err)

			h.Listen, err = expand(prefix+" listen", h.Listen)
			collect(err)

			h.UpstreamAddress, err = expand(prefix+" upstream_address", h.UpstreamAddress)
			collect(err)

			h.UpstreamNetwork, err = expand(prefix+" upstream_network", h.UpstreamNetwork)
			collect(err)
		}

		c.Servers[sname] = srv
	}

	return errors.Join(expandErrs...)
}

// Validate checks that the config is well-formed.
func (c *Config) Validate() error {
	for name := range c.Tokens {
		if err := ValidateSlug(name); err != nil {
			return fmt.Errorf("token %q: %w", name, err)
		}
	}
	for name, srv := range c.Servers {
		if err := ValidateSlug(name); err != nil {
			return fmt.Errorf("server %q: %w", name, err)
		}
		if srv.Token != "" {
			if _, ok := c.Tokens[srv.Token]; !ok {
				return fmt.Errorf("server %q: references undefined token %q", name, srv.Token)
			}
		}
		if len(srv.Handlers) == 0 {
			return fmt.Errorf("server %q: no handlers defined", name)
		}
		seen := make(map[string]bool)
		for i, h := range srv.Handlers {
			switch h.Type {
			case "tcp", "http":
			default:
				return fmt.Errorf("server %q: handler[%d]: unknown type %q", name, i, h.Type)
			}
			if h.Listen == "" {
				return fmt.Errorf("server %q: handler[%d]: listen address is required", name, i)
			}
			if h.UpstreamAddress == "" {
				return fmt.Errorf("server %q: handler[%d]: upstream_address is required", name, i)
			}
			key := h.Listen
			if seen[key] {
				return fmt.Errorf("server %q: handler[%d]: duplicate listen address %q", name, i, h.Listen)
			}
			seen[key] = true
		}
	}
	return nil
}

// ServerNames returns sorted server names for deterministic iteration.
func (c *Config) ServerNames() []string {
	names := make([]string, 0, len(c.Servers))
	for name := range c.Servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// HandlerTypeFlags returns a display label like "HTTP", "HTTP+TLS", or "TCP+Funnel".
func HandlerTypeFlags(h HandlerConfig) string {
	var flagParts []string
	if h.TLS {
		flagParts = append(flagParts, "TLS")
	}
	if h.Funnel {
		flagParts = append(flagParts, "Funnel")
	}
	typeFlags := strings.ToUpper(h.Type)
	if len(flagParts) > 0 {
		typeFlags += "+" + strings.Join(flagParts, "+")
	}
	return typeFlags
}

// HandlerColumnWidths returns max widths for listen and type/flags columns
// across the given handlers, for aligned multi-server display.
func HandlerColumnWidths(handlers []HandlerConfig) (maxListen, maxTypeFlags int) {
	for _, h := range handlers {
		if len(h.Listen) > maxListen {
			maxListen = len(h.Listen)
		}
		if n := len(HandlerTypeFlags(h)); n > maxTypeFlags {
			maxTypeFlags = n
		}
	}
	return maxListen, maxTypeFlags
}

// FormatHandlerLine returns one indented handler line for DisplayString-style output.
func FormatHandlerLine(h HandlerConfig, maxListen, maxTypeFlags int) string {
	return fmt.Sprintf("  %-*s %-*s -> %s\n",
		maxListen, h.Listen,
		maxTypeFlags, HandlerTypeFlags(h),
		h.UpstreamAddress)
}

// DisplayString returns a human-readable representation of configured servers.
func (c *Config) DisplayString() string {
	var b strings.Builder

	// Compute global max widths for handler columns so all sections align vertically
	var all []HandlerConfig
	for _, name := range c.ServerNames() {
		all = append(all, c.Servers[name].Handlers...)
	}
	maxListen, maxTypeFlags := HandlerColumnWidths(all)

	for _, name := range c.ServerNames() {
		srv := c.Servers[name]
		fmt.Fprintf(&b, "%s (hostname: %s)", name, srv.Hostname)
		if srv.Token != "" {
			fmt.Fprintf(&b, " [token: %s]", srv.Token)
		}
		b.WriteString("\n")
		for _, h := range srv.Handlers {
			b.WriteString(FormatHandlerLine(h, maxListen, maxTypeFlags))
		}
	}
	return b.String()
}
