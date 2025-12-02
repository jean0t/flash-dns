package server

import (
	"context"
	"dns-server/internal/utils"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

func createDNSQuery(domain string, qtype uint16) []byte {
	var (
		header        []byte = make([]byte, 12)
		question      []byte
		id            uint16 = 1234
		qdcount       uint16 = 1
		labels        []byte = []byte(domain)
		qtypeBytes    []byte = make([]byte, 2)
		qclassBytes   []byte = make([]byte, 2)
		standardQuery uint16 = 0x0100
		start         int
		length        int
	)
	binary.BigEndian.PutUint16(header[0:2], id)
	binary.BigEndian.PutUint16(header[2:4], standardQuery)
	binary.BigEndian.PutUint16(header[4:6], qdcount)

	for i := 0; i <= len(labels); i++ {
		if i == len(labels) || labels[i] == '.' {
			length = i - start
			if length > 0 {
				question = append(question, byte(length))
				question = append(question, labels[start:i]...)
			}
			start = i + 1
		}
	}
	question = append(question, 0)

	binary.BigEndian.PutUint16(qtypeBytes, qtype)
	question = append(question, qtypeBytes...)

	binary.BigEndian.PutUint16(qclassBytes, 1)
	question = append(question, qclassBytes...)

	return append(header, question...)
}

func TestNewDNSServer(t *testing.T) {
	var (
		localAddr    string     = "127.0.0.1"
		upstreamDns  string     = "1.1.1.1"
		expectedAddr string     = "127.0.0.1:53"
		expectedDns  string     = "1.1.1.1:53"
		server       *DNSServer = NewDNSServer(localAddr, upstreamDns)
	)

	if server == nil {
		t.Fatal("Server wasn't created")
	}

	if server.cache == nil {
		t.Error("server.cache is nil, should be initialized")
	}

	if expectedAddr != server.localAddr {
		t.Errorf("server.localAddr = %s, expected %s", server.localAddr, expectedAddr)
	}

	if expectedDns != server.upstreamDNS {
		t.Errorf("server.upstreamDNS = %s, expected %s", server.upstreamDNS, expectedDns)
	}
}

func TestContextCancellation(t *testing.T) {
	var (
		server   *DNSServer = NewDNSServer("127.0.0.1", "1.1.1.1")
		ctx      context.Context
		cancel   context.CancelFunc
		query    []byte = createDNSQuery("mock.com", 1)
		udpAddr  *net.UDPAddr
		mockConn *net.UDPConn
		done     chan bool = make(chan bool)
	)
	ctx, cancel = context.WithCancel(context.Background())
	cancel()

	udpAddr, _ = net.ResolveUDPAddr("udp", "127.0.0.1:39181")
	mockConn, _ = net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	defer mockConn.Close()

	go func() {
		server.handleQuery(ctx, query, udpAddr, mockConn)
		done <- true
	}()

	select {
	case <-done:

	case <-time.After(50 * time.Millisecond):
		t.Error("handleContext did not respect context cancellation")
	}
}

func TestQueryCacheMiss(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test that requires network connection")
	}

	var (
		upstreamDNS      string     = "1.1.1.1"
		localADDR        string     = "127.0.0.1"
		server           *DNSServer = NewDNSServer(localADDR, upstreamDNS)
		err              error
		ctx              context.Context
		cancel           context.CancelFunc
		clientConn       *net.UDPConn
		clientAddr       *net.UDPAddr
		actualClientAddr *net.UDPAddr
		query            []byte
		response         []byte = make([]byte, 512)
		n                int
		queryId          uint16
		responseId       uint16
		flags            uint16
		isResponse       bool
	)

	clientAddr, err = net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal("Failed to resolve client address")
	}

	clientConn, err = net.ListenUDP("udp", clientAddr)
	if err != nil {
		t.Fatal("Connection failed, can't receive response")
	}

	actualClientAddr = clientConn.LocalAddr().(*net.UDPAddr)
	query = createDNSQuery("google.com", 1)

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go server.handleQuery(ctx, query, actualClientAddr, clientConn)
	clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err = clientConn.Read(response)
	if err != nil {
		t.Fatal("Failed to receive a response")
	}

	if n < 12 {
		t.Fatal("Response too short, expects at least 12 bytes")
	}

	queryId = binary.BigEndian.Uint16(query[0:2])
	responseId = binary.BigEndian.Uint16(response[0:2])

	if queryId != responseId {
		t.Errorf("Transaction id mismatch, query= %d, response=%d", queryId, responseId)
	}

	flags = binary.BigEndian.Uint16(response[2:4])
	isResponse = (flags & 0x8000) != 0
	if !isResponse {
		t.Error("response QR bit not set, not valid response")
	}
}

func TestQueryCacheHit(t *testing.T) {
	var (
		localAddr        string     = "127.0.0.1"
		upstreamDns      string     = "1.1.1.1"
		server           *DNSServer = NewDNSServer(localAddr, upstreamDns)
		query            []byte     = createDNSQuery("google.com", 1)
		cacheKey         string     = utils.CreateCacheKey(query)
		response         []byte     = make([]byte, 512)
		fakeResponse     []byte     = make([]byte, len(query)+16)
		responseFlag     uint16     = 0x8180
		ttl              uint32     = 300
		n                int
		err              error
		clientAddr       *net.UDPAddr
		actualClientAddr *net.UDPAddr
		clientConn       *net.UDPConn
		ctx              context.Context
		cancel           context.CancelFunc
		queryId          uint16
		responseId       uint16
	)
	copy(fakeResponse, query)
	binary.BigEndian.PutUint16(fakeResponse[2:4], responseFlag)

	server.cache.Set(cacheKey, fakeResponse, ttl)
	clientAddr, _ = net.ResolveUDPAddr("udp", "127.0.0.1:0")
	clientConn, _ = net.ListenUDP("udp", clientAddr)
	actualClientAddr = clientConn.LocalAddr().(*net.UDPAddr)

	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go server.handleQuery(ctx, query, actualClientAddr, clientConn)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))

	n, err = clientConn.Read(response)
	if err != nil {
		t.Fatal("Failed to receive cached response")
	}

	if n < 12 {
		t.Error("Cached response too short")
	}

	queryId = binary.BigEndian.Uint16(query[0:2])
	responseId = binary.BigEndian.Uint16(response[0:2])

	if queryId != responseId {
		t.Errorf("Cached response id not match query, query= %d, response= %d", queryId, responseId)
	}
}
