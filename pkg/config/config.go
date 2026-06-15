package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"
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
	var err error

	// Top level
	c.StateDir, err = expand("state_dir", c.StateDir)
	expandErrs = append(expandErrs, err)

	// Tokens
	for name, token := range c.Tokens {
		token.AuthKey, err = expand(fmt.Sprintf("token %q auth_key", name), token.AuthKey)
		c.Tokens[name] = token
		expandErrs = append(expandErrs, err)
	}

	// Servers + handlers
	for sname, srv := range c.Servers {
		srv.Hostname, err = expand(fmt.Sprintf("server %q hostname", sname), srv.Hostname)
		expandErrs = append(expandErrs, err)

		srv.Token, err = expand(fmt.Sprintf("server %q token", sname), srv.Token)
		expandErrs = append(expandErrs, err)

		for i := range srv.Handlers {
			h := &srv.Handlers[i]
			prefix := fmt.Sprintf("server %q handler[%d]", sname, i)

			h.Type, err = expand(prefix+" type", h.Type)
			expandErrs = append(expandErrs, err)

			h.Listen, err = expand(prefix+" listen", h.Listen)
			expandErrs = append(expandErrs, err)

			h.UpstreamAddress, err = expand(prefix+" upstream_address", h.UpstreamAddress)
			expandErrs = append(expandErrs, err)

			h.UpstreamNetwork, err = expand(prefix+" upstream_network", h.UpstreamNetwork)
			expandErrs = append(expandErrs, err)
		}

		c.Servers[sname] = srv
	}

	if len(expandErrs) > 0 {
		return errors.Join(expandErrs...)
	}
	return nil
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

// DisplayString returns a human-readable representation of configured servers.
func (c *Config) DisplayString() string {
	var b strings.Builder
	for _, name := range c.ServerNames() {
		srv := c.Servers[name]
		fmt.Fprintf(&b, "%s (hostname: %s)", name, srv.Hostname)
		if srv.Token != "" {
			fmt.Fprintf(&b, " [token: %s]", srv.Token)
		}
		b.WriteString("\n")
		tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
		for _, h := range srv.Handlers {
			var flags []string
			if h.TLS {
				flags = append(flags, "TLS")
			}
			if h.Funnel {
				flags = append(flags, "Funnel")
			}
			flagStr := ""
			if len(flags) > 0 {
				flagStr = " [" + strings.Join(flags, ", ") + "]"
			}
			fmt.Fprintf(tw, "  %s\t%s%s\t->\t%s\n",
				h.Listen, strings.ToUpper(h.Type), flagStr, h.UpstreamAddress)
		}
		tw.Flush()
	}
	return b.String()
}
