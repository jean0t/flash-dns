package server

import (
	"context"
	"flash-dns/internal/cache"
	"flash-dns/internal/filter"
	"flash-dns/internal/logger"
	"flash-dns/internal/utils"
	"fmt"
	"net"
	"strings"
	"time"
)

// Interfaces to be used in the server
// they will divide work and make code more organized :)
type Resolver interface {
	Resolve(ctx context.Context, query []byte) ([]byte, error)
}

type Filter interface {
	IsBlocked(domain string) bool
	Count() int
}

type Cache interface {
	Get(key string) ([]byte, bool)
	Set(key string, response []byte, ttl uint32)
	Clean()
}

type ServerStatistics interface {
	incrementBlocked()
	incrementAllowed()
	incrementCacheHits()
	incrementCacheMisses()
	GetStats() (blocked, allowed, cacheHits, cacheMisses uint64)
	Log()
}

type Config struct {
	LocalAddr   string
	UpstreamDns string
	FilterMode  string // nxdomain or null, default to nxdomain
}

// server implementation
// orchestrate all the interfaces from before
type DNSServer struct {
	config     Config
	cache      Cache
	filter     Filter
	resolver   Resolver
	statistics ServerStatistics
}

func NewDNSServer(config Config, resolver Resolver, filterList *filter.FilterList) *DNSServer {
	var statistics *Statistics = &Statistics{}
	return &DNSServer{
		cache:      cache.NewDNSCache(),
		config:     config,
		filter:     filterList,
		resolver:   resolver,
		statistics: statistics,
	}
}

func (s *DNSServer) handleQuery(ctx context.Context, query []byte, clientAddr *net.UDPAddr, conn *net.UDPConn) {
	select {
	case <-ctx.Done():
		return
	default:
	}

	// filtering the query
	var (
		queryInfo *utils.QueryInfo
		err       error
		response  []byte = make([]byte, 512)
		blocked   bool
	)
	queryInfo, err = utils.ParseQuery(query)
	if err != nil {
		logger.Error(fmt.Sprintf("failed to parse query: %v", err))
		return
	}

	if blocked = s.filterDomain(queryInfo.Domain); blocked {
		copy(response, s.createBlockedResponse(query))
		conn.WriteToUDP(response, clientAddr)
		return
	}
	s.statistics.incrementAllowed()

	// response from cache immediately
	var (
		cachedResponse []byte = make([]byte, 512)
		found          bool
	)
	if cachedResponse, found = s.getCache(queryInfo.CacheKey, queryInfo.Domain); found {
		copy(response, cachedResponse)
		copy(response[0:2], query[0:2])

		conn.WriteToUDP(response, clientAddr)
		return
	}

	s.statistics.incrementCacheMisses()
	logger.Info(fmt.Sprintf("CACHE MISS: %s - querying Upstream", queryInfo.Domain))

	// if miss, query upstream
	var (
		ttl uint32
	)
	response, err = s.resolver.Resolve(ctx, query)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to Resolve: %s - %v", queryInfo.Domain, err))
		return
	}

	ttl = utils.ExtractTTL(response)
	s.cache.Set(queryInfo.CacheKey, response, ttl)
	logger.Info(fmt.Sprintf("CACHED: %s (TTl: %ds)", queryInfo.Domain, ttl))

	conn.WriteToUDP(response, clientAddr)
}

func (s *DNSServer) filterDomain(domain string) bool {
	if s.filter != nil && s.filter.IsBlocked(domain) {
		s.statistics.incrementBlocked()
		logger.Info(fmt.Sprintf("BLOCKED: %s", domain))
		return true
	}

	return false
}

func (s *DNSServer) getCache(cacheKey, domain string) ([]byte, bool) {
	var (
		cachedResponse []byte
		found          bool
	)
	cachedResponse, found = s.cache.Get(cacheKey)
	if !found {
		return nil, false
	}

	s.statistics.incrementCacheHits()
	logger.Info(fmt.Sprintf("CACHE HIT: %s", domain))

	return cachedResponse, true
}

func (s *DNSServer) createBlockedResponse(query []byte) []byte {
	if strings.EqualFold(s.config.FilterMode, "null") {
		return filter.CreateNullResponse(query)
	}

	return filter.CreateBlockedResponse(query)
}

func (s *DNSServer) Start(ctx context.Context) error {
	var (
		err    error
		addr   *net.UDPAddr
		conn   *net.UDPConn
		buffer []byte = make([]byte, 512)
	)
	addr, err = net.ResolveUDPAddr("udp", s.config.LocalAddr)
	if err != nil {
		return fmt.Errorf("Failed to resolve address: %w", err)
	}

	conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("Failed to listen: %s", err.Error())
	}
	defer conn.Close()

	logger.Info(fmt.Sprintf("DNS server is Listening on: %s", s.config.LocalAddr))
	logger.Info(fmt.Sprintf("DNS server upstream dns: %s", s.config.UpstreamDns))

	if s.filter != nil {
		logger.Info(fmt.Sprintf("Filter Loaded: %d domains", s.filter.Count()))
	}

	go s.cacheCleanUp(ctx)
	go s.statsReporter(ctx)
	go s.shutdownHandler(ctx, conn)

	for {
		select {
		case <-ctx.Done():
			logger.Info("Server Stopping")
			return nil
		default:
		}

		var (
			bytesRead  int
			clientAddr *net.UDPAddr
			query      []byte = make([]byte, 512)
		)
		conn.SetReadDeadline(time.Now().Add(3 * time.Second))

		bytesRead, clientAddr, err = conn.ReadFromUDP(buffer)
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
				logger.Error(fmt.Sprintf("Error reading: %v", err))
				continue
			}
		}
		copy(query, buffer[:bytesRead])
		go s.handleQuery(ctx, query, clientAddr, conn)
	}
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

func (s *DNSServer) statsReporter(ctx context.Context) {
	var ticker *time.Ticker = time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.statistics.Log()
		case <-ctx.Done():
			s.statistics.Log()
			return
		}
	}
}

func (s *DNSServer) shutdownHandler(ctx context.Context, conn *net.UDPConn) {
	<-ctx.Done()
	logger.Info("Shutdown signal received, closing the server.")
	conn.Close()
}
