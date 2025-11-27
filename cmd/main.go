package main

import (
	"dns-server/internal/logger"
	"dns-server/internal/server"
	"flag"
	"log"
)

var (
	start                  bool
	err                    error
	localAddr, upstreamDns string
	port                   string = ":53"
)

func init() {
	flag.BoolVar(&start, "s", false, "Start the Server")
	flag.StringVar(&localAddr, "a", "0.0.0.0", "Address that the DNS server will listen")
	flag.StringVar(&upstreamDns, "d", "1.1.1.1", "Upstream DNS to consult ips")
}

func main() {
	flag.Parse()

	if start {
		var server *server.DNSServer = server.NewDNSServer(localAddr+port, upstreamDns)
		if err = server.Start(); err != nil {
			log.Println("Erro: ", err.Error())
			_ = logger.Init(logger.DefaultPath)
			logger.Error("Server gave an error: " + err.Error())
			panic("DNS Error")
		}
	}
}
