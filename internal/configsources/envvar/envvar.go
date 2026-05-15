package envvar

import (
	"os"
	"strings"

	"github.com/lucasew/ts-proxy/internal/configsources"
)

const prefix = "TSPROXY_"

// Known option suffixes. The server name is everything between
// the prefix and the last occurrence of one of these suffixes.
var knownSuffixes = []string{
	"_ADDRESS",
	"_HOSTNAME",
	"_NETWORK",
	"_FUNNEL",
	"_TLS",
	"_HTTP",
	"_STATEDIR",
	"_LISTEN",
	"_RAW",
}

// EnvVarSource discovers server configurations from environment variables
// matching the pattern TSPROXY_<NAME>_<OPTION>.
//
// Example:
//
//	TSPROXY_WEBAPP_ADDRESS=localhost:8080
//	TSPROXY_WEBAPP_HOSTNAME=mywebapp
//	TSPROXY_API_ADDRESS=localhost:9090
//	TSPROXY_API_FUNNEL=true
type EnvVarSource struct{}

func New() *EnvVarSource {
	return &EnvVarSource{}
}

func (s *EnvVarSource) Name() string { return "envvar" }

func (s *EnvVarSource) Discover() ([]configsources.ServerConfig, error) {
	// Collect raw key=value pairs grouped by server name.
	servers := make(map[string]map[string]string)
	// Track insertion order for deterministic output.
	var order []string

	for _, env := range os.Environ() {
		key, value, ok := strings.Cut(env, "=")
		if !ok {
			continue
		}
		upper := strings.ToUpper(key)
		if !strings.HasPrefix(upper, prefix) {
			continue
		}
		rest := upper[len(prefix):]
		name, option := splitSuffix(rest)
		if name == "" || option == "" {
			continue
		}
		if _, exists := servers[name]; !exists {
			servers[name] = make(map[string]string)
			order = append(order, name)
		}
		servers[name][option] = value
	}

	var configs []configsources.ServerConfig
	for _, name := range order {
		opts := servers[name]
		cfg := configsources.ServerConfig{
			Name:         strings.ToLower(name),
			Network:      opts["NETWORK"],
			Address:      opts["ADDRESS"],
			Hostname:     opts["HOSTNAME"],
			EnableFunnel: isTruthy(opts["FUNNEL"]),
			EnableTLS:    isTruthy(opts["TLS"]),
			EnableHTTP:   !isTruthy(opts["RAW"]),
			StateDir:     opts["STATEDIR"],
			Listen:       opts["LISTEN"],
		}
		// Only include if at least an address is defined.
		if cfg.Address == "" {
			continue
		}
		// Explicit HTTP override takes precedence over RAW.
		if v, ok := opts["HTTP"]; ok {
			cfg.EnableHTTP = isTruthy(v)
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

// splitSuffix finds the longest known suffix in s and returns (name, suffix)
// with the leading underscore of the suffix stripped. Returns ("","") if
// no known suffix matches.
func splitSuffix(s string) (string, string) {
	for _, suf := range knownSuffixes {
		if strings.HasSuffix(s, suf) {
			name := s[:len(s)-len(suf)]
			if name == "" {
				continue
			}
			return name, suf[1:] // strip leading "_"
		}
	}
	return "", ""
}

func isTruthy(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
