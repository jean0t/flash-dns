package main

import (
	"context"
	"dns-server/internal/logger"
	"dns-server/internal/server"
	"flag"
	"fmt"
	"os"
	"os/signal"
)

var (
	start       bool
	err         error
	localAddr   string
	upstreamDns string
)

func init() {
	flag.BoolVar(&start, "s", false, "Start the Server")
	flag.StringVar(&localAddr, "a", "0.0.0.0", "Address that the DNS server will listen")
	flag.StringVar(&upstreamDns, "d", "1.1.1.1", "Upstream DNS to consult ips")
}

func verifications() {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "Script must be run a root")
		os.Exit(1)
	}

	if err = logger.Init(logger.DefaultPath); err != nil {
		fmt.Fprintln(os.Stderr, "Failed to initialize logger: "+err.Error())
		os.Exit(1)
	}
}

func startServer() {
	var (
		ctx     context.Context
		cancel  context.CancelFunc
		sigChan chan os.Signal = make(chan os.Signal, 1)
	)

	signal.Notify(sigChan, os.Interrupt, os.Kill)

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	go func() {
		var sig os.Signal = <-sigChan
		logger.Info("Closing DNS Server, received signal: " + sig.String())
		cancel()
	}()

	if start {
		var server *server.DNSServer = server.NewDNSServer(localAddr, upstreamDns)
		if err = server.Start(ctx); err != nil {
			logger.Error("Server gave an error: " + err.Error())
			fmt.Fprintln(os.Stderr, "Server had an error while starting, is port 53 free?")
			os.Exit(1)
		}
	}

	logger.Info("Server Shutdown Complete and Successfully")
}

func main() {
	flag.Parse()
	verifications()
	startServer()
}
