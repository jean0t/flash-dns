package main

import (
	"context"
	"dns-server/internal/logger"
	"dns-server/internal/server"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

var (
	start                  bool
	err                    error
	localAddr, upstreamDns string
)

func init() {
	flag.BoolVar(&start, "s", false, "Start the Server")
	flag.StringVar(&localAddr, "a", "0.0.0.0", "Address that the DNS server will listen")
	flag.StringVar(&upstreamDns, "d", "1.1.1.1", "Upstream DNS to consult ips")
}

func main() {
	flag.Parse()

	var (
		ctx     context.Context
		cancel  context.CancelFunc
		sigChan chan os.Signal = make(chan os.Signal, 1)
	)

	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	if err = logger.Init(logger.DefaultPath); err != nil {
		cancel()
		panic("Failed to initialize logger: " + err.Error())
	}

	go func() {
		var sig os.Signal = <-sigChan
		logger.Info("Closing DNS Server, received signal: " + sig.String())
		cancel()
	}()

	if start {
		var server *server.DNSServer = server.NewDNSServer(localAddr, upstreamDns)
		if err = server.Start(ctx); err != nil {
			logger.Error("Server gave an error: " + err.Error())
			panic("DNS Error")
		}
	}

	logger.Info("Server Shutdown Complete and Successfully")
}
