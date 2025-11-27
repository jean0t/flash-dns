package server

import (
	"dns-server/internal/cache"
	"dns-server/internal/logger"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

type DNSServer struct {
	cache       *cache.DNSCache
	upstreamDNS string
	localAddr   string
}

func NewDNSServer(localAddr, upstreamDNS string) *DNSServer {
	if err := logger.Init(logger.DefaultPath); err != nil {
		panic("Log couldn't be initialized")
	}

	return &DNSServer{
		cache:       cache.NewDNSCache(),
		upstreamDNS: upstreamDNS,
		localAddr:   localAddr,
	}
}

var builderPool = sync.Pool{
	New: func() interface{} {
		return &strings.Builder{}
	},
}

func parseDomainName(data []byte, offset int) (string, int) {
	var builder *strings.Builder = builderPool.Get().(*strings.Builder)
	builder.Reset()
	defer builderPool.Put(builder)

	var (
		position int = offset
		length   int
	)
	for {
		length = int(data[position])
		if length == 0 { // if zero, then it is the null terminator
			position++
			break
		}

		if builder.Len() > 0 {
			builder.WriteRune('.')
		}

		position++ // skips the length byte
		builder.Write(data[position : position+length])
		position += length // goes to the next length byte
	}

	return builder.String(), position
}

func extractTTL(response []byte) uint32 {
	var result uint32 = 300 //default to 5 minutes (300 seconds)

	if len(response) < 12 {
		return result
	}

	// skip header (12 bytes)
	var position int = 12

	// skip question section
	var qdcount uint16 = binary.BigEndian.Uint16(response[4:6])
	for i := 0; i < int(qdcount); i++ {

		//skip domain name
		for position < len(response) && response[position] != 0 {
			if response[position] >= 192 { //compression pointer
				position += 2
				break
			}
			position += int(response[position]) + 1
		}

		if response[position] == 0 {
			position++
		}

		position += 4 // skip QTYPE and QCLASS
	}

	// read answer section to find TTL
	var ancount uint16 = binary.BigEndian.Uint16(response[6:8])
	var minTTL uint32 = uint32(3600) // default 1 hour

	for i := 0; i < int(ancount) && position+10 < len(response); i++ {
		//skip name
		for position < len(response) && response[position] != 0 {
			if response[position] >= 192 { // compression pointer
				position += 2
				break
			}

			position += int(response[position]) + 1
		}

		if position < len(response) && response[position] == 0 {
			position++
		}

		if position+10 > len(response) {
			break
		}

		var ttl uint32 = binary.BigEndian.Uint32(response[position+4 : position+8])
		if ttl < minTTL {
			minTTL = ttl
		}

		// skip type, class, ttl and rdlength
		var rdlength uint16 = binary.BigEndian.Uint16(response[position+8 : position+10])
		position += 10 + int(rdlength)
	}

	return minTTL
}

func createCacheKey(query []byte) string {
	var (
		domain           string
		position         int
		qtype            uint16
		skipHeaderOffset int = 12
	)
	if len(query) < 12 {
		return ""
	}

	domain, position = parseDomainName(query, skipHeaderOffset)
	if position > len(query) {
		return domain
	}

	qtype = binary.BigEndian.Uint16(query[position : position+2])
	return fmt.Sprintf("%s:%d", domain, qtype)
}

func (s *DNSServer) handleQuery(query []byte, clientAddr *net.UDPAddr, conn *net.UDPConn) {
	var (
		cacheKey string = createCacheKey(query)
	)

	if cachedResponse, found := s.cache.Get(cacheKey); found {
		logger.Info(fmt.Sprintf("Cache Hit: %s", cacheKey))

		var response []byte = make([]byte, len(cachedResponse))
		copy(response, cachedResponse)
		copy(response[0:2], query[0:2])

		conn.WriteToUDP(response, clientAddr)
		return
	}

	logger.Warn(fmt.Sprintf("Cache miss: %s - Querying upstream dns"))
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
	ttl = extractTTL(response)
	s.cache.Set(cacheKey, response, ttl)
	logger.Info(fmt.Sprintf("Cached %s with TTL: %d seconds", cacheKey, ttl))

	conn.WriteToUDP(response, clientAddr)
}

func (s *DNSServer) Start() error {
	var (
		err      error
		errorMsg string
		addr     *net.UDPAddr
		conn     *net.UDPConn
	)
	addr, err = net.ResolveUDPAddr("udp", s.localAddr)
	if err != nil {
		errorMsg = fmt.Sprintf("Failed to resolve address: %s", err.Error())
		logger.Error(errorMsg)
		return fmt.Errorf(errorMsg)
	}

	conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		errorMsg = fmt.Sprintf("Failed to listen: %s", err.Error())
		logger.Error(errorMsg)
		return fmt.Errorf(errorMsg)
	}
	defer conn.Close()

	logger.Info(fmt.Sprintf("DNS server is Listening on: %s", s.localAddr))
	logger.Info(fmt.Sprintf("DNS server upstream dns: %s", s.upstreamDNS))

	go func() {
		var ticker *time.Ticker = time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.cache.Clean()
		}
	}()

	var buffer []byte = make([]byte, 512)
	for {
		var (
			n          int
			clientAddr *net.UDPAddr
		)
		n, clientAddr, err = conn.ReadFromUDP(buffer)
		if err != nil {
			errorMsg = fmt.Sprintf("Error reading: %s", err.Error())
			logger.Error(errorMsg)
			continue
		}

		var query []byte = make([]byte, n)
		copy(query, buffer[:n])

		go s.handleQuery(query, clientAddr, conn)
	}

	return nil
}
