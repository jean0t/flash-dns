package main

import (
	"context"
	"dns-server/internal/filter"
	"dns-server/internal/logger"
	"dns-server/internal/server"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
)

var (
	start            bool
	err              error
	localAddr        string
	upstreamDns      string
	filterDomainFile string
	filterList       *filter.FilterList
)

func init() {
	flag.BoolVar(&start, "s", false, "Start the Server")
	flag.StringVar(&localAddr, "a", "0.0.0.0", "Address that the DNS server will listen")
	flag.StringVar(&upstreamDns, "d", "1.1.1.1", "Upstream DNS to consult ips")
	flag.StringVar(&filterDomainFile, "f", "", "Path to file with domains to be filtered")
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

func getFilterList() {
	if filterDomainFile == "" {
		filterList = filter.NewFilterList()
		return
	}

	var (
		absolutePath string
		err          error
	)
	absolutePath, err = filepath.Abs(filterDomainFile)
	if err != nil {
		logger.Error("File path to the filter list returned an error.")
	}
	filterList = filter.NewFilterList()
	filterList.LoadFromFile(absolutePath)
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
		var server *server.DNSServer = server.NewDNSServer(localAddr, upstreamDns, filterList)
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
	getFilterList()
	startServer()
}
