package main

import (
	"flag"
	"github.com/lucasew/ts-proxy"
	"log"
	"net/url"
	"github.com/davecgh/go-spew/spew"
)

var options tsproxy.TailscaleProxyServerOptions

func init() {
	var err error
	var remoteHost string
	flag.StringVar(&remoteHost, "h", "", "Where to forward the connection")
	flag.BoolVar(&options.EnableFunnel, "f", false, "Enable tailscale funnel")
	flag.StringVar(&options.Hostname, "n", "", "Hostname in tailscale devices list")
	flag.StringVar(&options.StateDir, "s", "", "State directory")
	flag.Parse()
	defaultPort := ":80"
	if options.EnableFunnel {
		defaultPort = ":443"
	}
	flag.StringVar(&options.Addr, "addr", defaultPort, "Port to listen")
	flag.Parse()
	options.Upstream, err = url.Parse(remoteHost)
	spew.Dump(options)
	if err != nil {
		log.Fatal(err)
	}
}

func main() {
	server, err := tsproxy.NewTailscaleProxyServer(options)
	if err != nil {
		panic(err)
	}
	server.Run()
}
