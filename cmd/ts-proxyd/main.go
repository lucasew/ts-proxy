package main

import (
	"flag"
	"log/slog"

	tsproxy "github.com/lucasew/ts-proxy"
)

func main() {
	var options tsproxy.TailscaleProxyServerOptions

	flag.StringVar(&options.Network, "net", "tcp", "Network, for net.Dial")
	flag.StringVar(&options.Address, "address", "", "Where to forward the connection")
	flag.StringVar(&options.Hostname, "n", "", "Hostname in tailscale devices list")
	flag.BoolVar(&options.EnableFunnel, "f", false, "Enable tailscale funnel")
	flag.BoolVar(&options.EnableTLS, "t", false, "Enable HTTPS/TLS")
	flag.StringVar(&options.StateDir, "s", "", "State directory")
	flag.StringVar(&options.Listen, "listen", "", "Port to listen")
	flag.BoolVar(&options.EnableHTTP, "raw", false, "Disable HTTP handling")
	flag.Parse()

	options.EnableHTTP = !options.EnableHTTP
	if options.Listen == "" && options.EnableHTTP {
		if options.EnableFunnel || options.EnableTLS {
			options.Listen = ":443"
		} else {
			options.Listen = ":80"
		}
	}
	if options.Listen == "" {
		panic("-listen not defined")
	}

	slog.Info("configuration",
		"network", options.Network,
		"address", options.Address,
		"hostname", options.Hostname,
		"funnel", options.EnableFunnel,
		"tls", options.EnableTLS,
		"statedir", options.StateDir,
		"listen", options.Listen,
		"http", options.EnableHTTP,
	)

	server, err := tsproxy.NewTailscaleProxyServer(options)
	if err != nil {
		panic(err)
	}
	server.Run()
}
