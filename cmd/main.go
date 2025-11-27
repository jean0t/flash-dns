package main

import (
	"dns-server/internal/logger"
	"dns-server/internal/server"
	"flag"
)

func main() {
	var (
		start                  bool
		err                    error
		localAddr, upstreamDns string
	)
	flag.BoolVar(&start, "s", true, "Start the Server")
	flag.StringVar(&localAddr, "a", "0.0.0.0", "Address the DNS server will listen, default to all network interfaces (0.0.0.0)")
	flag.StringVar(&upstreamDns, "d", "1.1.1.1", "Upstream DNS to consult ips, default to cloudflare (1.1.1.1)")
	flag.Parse()

	if start {
		var server *server.DNSServer = server.NewDNSServer(localAddr, upstreamDns)
		if err = server.Start(); err != nil {
			_ = logger.Init(logger.DefaultPath)
			logger.Error("Server gave an error: " + err.Error())
			panic("DNS Error")
		}
	}
}
