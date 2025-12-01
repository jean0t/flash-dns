package server

import (
	"context"
	"dns-server/internal/cache"
	"dns-server/internal/logger"
	"dns-server/internal/utils"
	"fmt"
	"net"
	"os"
	"time"
)

type DNSServer struct {
	cache       *cache.DNSCache
	upstreamDNS string
	localAddr   string
}

func NewDNSServer(localAddr, upstreamDNS string) *DNSServer {

	var dnsPort string = ":53"
	return &DNSServer{
		cache:       cache.NewDNSCache(),
		upstreamDNS: upstreamDNS + dnsPort,
		localAddr:   localAddr + dnsPort,
	}
}

func (s *DNSServer) handleQuery(ctx context.Context, query []byte, clientAddr *net.UDPAddr, conn *net.UDPConn) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	var (
		cacheKey string = utils.CreateCacheKey(query)
	)

	if cachedResponse, found := s.cache.Get(cacheKey); found {
		logger.Info(fmt.Sprintf("Cache Hit: %s", cacheKey))

		var response []byte = make([]byte, len(cachedResponse))
		copy(response, cachedResponse)
		copy(response[0:2], query[0:2])

		conn.WriteToUDP(response, clientAddr)
		return
	}

	logger.Warn(fmt.Sprintf("Cache miss: %s - Querying upstream dns", cacheKey))
	var (
		upstreamConn net.Conn
		n            int
		ttl          uint32
		err          error
		response     []byte = make([]byte, 512)
	)
	upstreamConn, err = net.Dial("udp", s.upstreamDNS)
	if err != nil {
		logger.Error(fmt.Sprintf("Error connecting to upstream: %s", s.upstreamDNS))
		return
	}
	defer upstreamConn.Close()
	upstreamConn.SetDeadline(time.Now().Add(5 * time.Second))

	_, err = upstreamConn.Write(query)
	if err != nil {
		logger.Error(fmt.Sprintf("Error writing to upstream: %s", err.Error()))
		return
	}

	n, err = upstreamConn.Read(response)
	if err != nil {
		logger.Error(fmt.Sprintf("Error reading from upstream: %s", err.Error()))
		response = nil
		return
	}
	copy(response, response[:n])
	ttl = utils.ExtractTTL(response)
	s.cache.Set(cacheKey, response, ttl)
	logger.Info(fmt.Sprintf("Cached %s with TTL: %d seconds", cacheKey, ttl))

	conn.WriteToUDP(response, clientAddr)
}

func (s *DNSServer) Start(ctx context.Context) error {
	var (
		err      error
		errorMsg string
		addr     *net.UDPAddr
		conn     *net.UDPConn
	)
	if err = logger.Init(logger.DefaultPath); err != nil {
		fmt.Fprintln(os.Stderr, "Log couldn't be initialized")
		os.Exit(1)
	}

	addr, err = net.ResolveUDPAddr("udp", s.localAddr)
	if err != nil {
		errorMsg = fmt.Sprintf("Failed to resolve address: %s", err.Error())
		logger.Error(errorMsg)
		return fmt.Errorf("%s", errorMsg)
	}

	conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		errorMsg = fmt.Sprintf("Failed to listen: %s", err.Error())
		logger.Error(errorMsg)
		return fmt.Errorf("%s", errorMsg)
	}
	defer conn.Close()

	logger.Info(fmt.Sprintf("DNS server is Listening on: %s", s.localAddr))
	logger.Info(fmt.Sprintf("DNS server upstream dns: %s", s.upstreamDNS))

	go s.cacheCleanUp(ctx)

	go func() {
		<-ctx.Done()
		logger.Info("Context cancelled, closing UDP connection")
		conn.Close()
	}()

	var buffer []byte = make([]byte, 512)
	for {
		select {
		case <-ctx.Done():
			logger.Info("Server Stopping")
		default:
			var (
				n          int
				clientAddr *net.UDPAddr
			)
			conn.SetReadDeadline(time.Now().Add(1 * time.Second))

			n, clientAddr, err = conn.ReadFromUDP(buffer)
			if err != nil {
				var (
					netErr net.Error
					ok     bool
				)
				if netErr, ok = err.(net.Error); ok && netErr.Timeout() {
					continue
				}

				select {
				case <-ctx.Done():
					return nil
				default:
					errorMsg = fmt.Sprintf("Error reading: %s", err.Error())
					logger.Error(errorMsg)
					continue
				}
			}

			var query []byte = make([]byte, n)
			copy(query, buffer[:n])

			go s.handleQuery(ctx, query, clientAddr, conn)
		}
	}

	return nil
}

func (s *DNSServer) cacheCleanUp(ctx context.Context) {
	var ticker *time.Ticker = time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	logger.Info("Cache Cleanup Started")

	for {
		select {
		case <-ticker.C:
			s.cache.Clean()
		case <-ctx.Done():
			logger.Info("Cache Cleanup Stopped")
			return
		}
	}
}
