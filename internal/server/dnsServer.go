package server

import (
	"bytes"
	"context"
	"flash-dns/internal/cache"
	"flash-dns/internal/filter"
	"flash-dns/internal/logger"
	"flash-dns/internal/utils"
	"fmt"
	"net"
	"os"
	"sync/atomic"
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

type Statistics struct {
	blockedCount atomic.Uint64
	allowedCount atomic.Uint64
	cacheHits    atomic.Uint64
	cacheMisses  atomic.Uint64
}

func (s *Statistics) incrementBlocked() {
	_ = s.blockedCount.Add(1)
}

func (s *Statistics) incrementAllowed() {
	_ = s.allowedCount.Add(1)
}

func (s *Statistics) incrementCacheHits() {
	_ = s.cacheHits.Add(1)
}

func (s *Statistics) incrementCacheMisses() {
	_ = s.cacheMisses.Add(1)
}

func (s *Statistics) GetStats() (blocked, allowed, cacheHits, cacheMisses uint64) {
	return s.blockedCount.Load(), s.allowedCount.Load(), s.cacheHits.Load(), s.cacheMisses.Load()
}

func (s *Statistics) Log() {
	var (
		blocked      uint64
		allowed      uint64
		cacheHits    uint64
		cacheMisses  uint64
		total        uint64
		blockRate    float
		CacheHitRate float
	)

	blocked, allowed, cacheHits, cacheMisses = s.GetStats()
	total = blocked + allowed

	blockRate = float(blocked) / float(total) * 100
	CacheHitRate = float(cacheHits) / float(cacheHits+cacheMisses) * 100

	logger.Info(fmt.Sprintf("Status - Total: %d | Blocked: %d (%.1f%%) | Cache Hit Rate: %.1f%%", total, blocked, blockRate, CacheHitRate))
}

type UpstreamResolver struct {
	upstreamAddr string
	timeout      time.Duration
}

func NewUpstreamResolver(upstreamAddr string) *UpstreamResolver {
	return &UpstreamResolver{
		upstreamAddr: upstreamAddr + ":53",
		timeout:      5 * time.Second,
	}
}

func (u *upstreamResolver) Resolve(ctx context.Context, query []byte) ([]byte, error) {
	var (
		conn      net.Conn
		err       error
		deadLine  time.Duration
		response  []byte = make([]byte, 512)
		bytesRead int
	)
	conn, err = net.Dial("udp", s.upstreamAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to upstream: %w", err)
	}
	defer conn.Close()

	deadline = time.Now().Add(s.timeout)
	conn.SetDeadline(deadline)

	if _, err = conn.Write(query); err != nil {
		return nil, fmt.Errorf("failed to write query: %w", err)
	}

	bytesRead, err = conn.Read(response)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	response = bytes.Clone(response[:bytesRead])
	return response, nil
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
	statistics Statistics
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
		cacheKey   string = utils.CreateCacheKey(query)
		domainName string = utils.ParseDomainName(query)
		response   []byte = make([]byte, 512)
	)
	if s.filter != nil && s.filter.IsBlocked(domainName) {
		s.statistics.incrementBlocked()
		logger.Info(fmt.Sprintf("BLOCKED: %s", domainName))

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
	if cachedResponse, found = s.cache.Get(cacheKey); found {
		s.statistics.incrementCacheHits()
		logger.Info(fmt.Sprintf("CACHE HIT: %s", domainName))

		copy(response, cachedResponse)
		copy(response[0:2], query[0:2])

		conn.WriteToUDP(response, clientAddr)
		return
	}

	s.statistics.incrementCacheMisses()
	logger.Info(fmt.Sprintf("CACHE MISS: %s - querying Upstream", domainName))

	// if miss, query upstream
	var (
		err error
		ttl uint32
	)
	response, err = s.resolver.Resolve(ctx, query)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to Resolve: %s - %v", domainName, err))
	}

	ttl = utils.ExtractTTL(response)
	s.cache.Set(cacheKey, response, ttl)
	logger.Info(fmt.Sprintf("CACHED: %s (TTl: %ds)", domainName, ttl))

	conn.WriteToUDP(response, clientAddr)
}

func (s *DNSServer) createBlockedResponse(query []byte) []byte {
	if strings.EqualFold(s.config.FilterMode, "null") {
		return filter.CreateNullResponse(query)
	}

	return filter.CreateBlockedResponse(query)
}

func (s *DNSServer) Start(ctx context.Context) error {
	var (
		err      error
		errorMsg string
		addr     *net.UDPAddr
		conn     *net.UDPConn
		buffer   []byte = make([]byte, 512)
	)
	addr, err = net.ResolveUDPAddr("udp", s.localAddr)
	if err != nil {
		return fmt.Errorf("Failed to resolve address: %w", err)
	}

	conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return fmt.Errorf("Failed to listen: %s", err.Error())
	}
	defer conn.Close()

	logger.Info(fmt.Sprintf("DNS server is Listening on: %s", s.localAddr))
	logger.Info(fmt.Sprintf("DNS server upstream dns: %s", s.upstreamDNS))

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
		default:
		}

		var (
			bytesRead  int
			clientAddr *net.UDPAddr
			query      []byte = make([]byte, bytesRead)
		)
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))

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
				return fmt.Errorf("Error reading: %w", err)
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

func (s *DNSServer) statsReporter(ctx) {
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

func (s *DNSServer) shutdownHandler(ctx, conn) {
	<-ctx.Done()
	logger.Info("Shutdown signal received, closing the server.")
	conn.Close()
}
