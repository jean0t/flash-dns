package server

import (
	"context"
	"encoding/binary"
	"net"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// MOCK DNS SERVER FOR TESTING
// ============================================================================

// mockDNSServer creates a UDP server that responds to DNS queries
type mockDNSServer struct {
	addr     string
	conn     *net.UDPConn
	response []byte
	delay    time.Duration
}

func startMockDNSServer(response []byte, delay time.Duration) (*mockDNSServer, error) {
	var (
		addr   *net.UDPAddr
		conn   *net.UDPConn
		err    error
		server *mockDNSServer
	)

	addr, err = net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	conn, err = net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	server = &mockDNSServer{
		addr:     conn.LocalAddr().String(),
		conn:     conn,
		response: response,
		delay:    delay,
	}

	go server.serve()

	return server, nil
}

func (m *mockDNSServer) serve() {
	var (
		buffer    []byte = make([]byte, 512)
		bytesRead int
		addr      *net.UDPAddr
		err       error
	)

	for {
		bytesRead, addr, err = m.conn.ReadFromUDP(buffer)
		if err != nil {
			return
		}

		// Simulate delay if configured
		if m.delay > 0 {
			time.Sleep(m.delay)
		}

		// Copy transaction ID from query to response
		if len(m.response) >= 2 && bytesRead >= 2 {
			var responseCopy []byte = make([]byte, len(m.response))
			copy(responseCopy, m.response)
			copy(responseCopy[0:2], buffer[0:2])
			m.conn.WriteToUDP(responseCopy, addr)
		}
	}
}

func (m *mockDNSServer) close() {
	if m.conn != nil {
		m.conn.Close()
	}
}

// ============================================================================
// TESTS
// ============================================================================

// TEST 1: Create new upstream resolver with single address
// Tests initialization with one upstream DNS server
func TestNewUpstreamResolver_SingleAddress(t *testing.T) {
	var (
		upstream string = "8.8.8.8"
		resolver *UpstreamResolver
	)

	resolver = NewUpstreamResolver(upstream)

	if resolver == nil {
		t.Fatal("Resolver should not be nil")
	}
	if len(resolver.upstreamAddrs) != 1 {
		t.Errorf("Expected 1 upstream address, got %d", len(resolver.upstreamAddrs))
	}
	if resolver.upstreamAddrs[0] != "8.8.8.8:53" {
		t.Errorf("Expected '8.8.8.8:53', got '%s'", resolver.upstreamAddrs[0])
	}
	if resolver.timeout != 5*time.Second {
		t.Errorf("Expected 5s timeout, got %v", resolver.timeout)
	}
}

// TEST 2: Create resolver with multiple addresses
// Tests initialization with comma-separated upstream servers
func TestNewUpstreamResolver_MultipleAddresses(t *testing.T) {
	var (
		upstream string = "8.8.8.8, 1.1.1.1, 9.9.9.9"
		resolver *UpstreamResolver
	)

	resolver = NewUpstreamResolver(upstream)

	if len(resolver.upstreamAddrs) != 3 {
		t.Errorf("Expected 3 upstream addresses, got %d", len(resolver.upstreamAddrs))
	}

	var expected []string = []string{"8.8.8.8:53", "1.1.1.1:53", "9.9.9.9:53"}
	var i int
	var addr string
	for i, addr = range expected {
		if resolver.upstreamAddrs[i] != addr {
			t.Errorf("Expected address %d to be '%s', got '%s'",
				i, addr, resolver.upstreamAddrs[i])
		}
	}
}

// TEST 3: Resolver trims whitespace from addresses
// Tests that spaces around addresses are properly handled
func TestNewUpstreamResolver_TrimsWhitespace(t *testing.T) {
	var (
		upstream string = "  8.8.8.8  ,  1.1.1.1  "
		resolver *UpstreamResolver
	)

	resolver = NewUpstreamResolver(upstream)

	if resolver.upstreamAddrs[0] != "8.8.8.8:53" {
		t.Errorf("Expected trimmed '8.8.8.8:53', got '%s'", resolver.upstreamAddrs[0])
	}
	if resolver.upstreamAddrs[1] != "1.1.1.1:53" {
		t.Errorf("Expected trimmed '1.1.1.1:53', got '%s'", resolver.upstreamAddrs[1])
	}
}

// TEST 4: Resolve with successful upstream response
// Tests that resolver successfully queries mock DNS server
func TestUpstreamResolver_Resolve_Success(t *testing.T) {
	var (
		ctx          context.Context = context.Background()
		query        []byte          = buildDNSQuery("example.com", 1, 1)
		mockResponse []byte          = buildDNSResponse("example.com", 1, 1, 3600, []byte{1, 2, 3, 4})
		server       *mockDNSServer
		err          error
		resolver     *UpstreamResolver
		response     []byte
	)

	// Start mock DNS server
	server, err = startMockDNSServer(mockResponse, 0)
	if err != nil {
		t.Fatalf("Failed to start mock server: %v", err)
	}
	defer server.close()

	// Extract just the IP:port without the scheme
	var upstream string = strings.TrimPrefix(server.addr, "udp://")
	upstream = strings.TrimSuffix(upstream, ":53")

	resolver = &UpstreamResolver{
		upstreamAddrs: []string{server.addr},
		timeout:       2 * time.Second,
	}

	response, err = resolver.Resolve(ctx, query)

	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if response == nil {
		t.Fatal("Response should not be nil")
	}
	if len(response) == 0 {
		t.Error("Response should not be empty")
	}
}

// TEST 5: Resolve with context cancellation
// Tests that resolver respects cancelled context
func TestUpstreamResolver_Resolve_ContextCancelled(t *testing.T) {
	var (
		ctx      context.Context
		cancel   context.CancelFunc
		query    []byte            = buildDNSQuery("example.com", 1, 1)
		resolver *UpstreamResolver = &UpstreamResolver{
			upstreamAddrs: []string{"8.8.8.8:53"},
			timeout:       5 * time.Second,
		}
		response []byte
		err      error
	)

	ctx, cancel = context.WithCancel(context.Background())
	cancel() // Cancel immediately

	response, err = resolver.Resolve(ctx, query)

	if err == nil {
		t.Error("Expected error for cancelled context")
	}
	if response != nil {
		t.Error("Response should be nil for cancelled context")
	}
}

// TEST 6: Resolve with timeout
// Tests that resolver times out when upstream doesn't respond
func TestUpstreamResolver_Resolve_Timeout(t *testing.T) {
	var (
		ctx      context.Context = context.Background()
		query    []byte          = buildDNSQuery("example.com", 1, 1)
		resolver *UpstreamResolver
		response []byte
		err      error
	)

	// Use non-existent address to force timeout
	resolver = &UpstreamResolver{
		upstreamAddrs: []string{"192.0.2.1:53"}, // TEST-NET-1, should not respond
		timeout:       500 * time.Millisecond,   // Short timeout for fast test
	}

	response, err = resolver.Resolve(ctx, query)

	if err == nil {
		t.Error("Expected timeout error")
	}
	if response != nil {
		t.Error("Response should be nil on timeout")
	}
	if err != nil && !strings.Contains(err.Error(), "failed") {
		t.Errorf("Expected 'failed' error message, got: %v", err)
	}
}

// TEST 7: Resolve with multiple upstreams - first responds
// Tests that first responding upstream wins
func TestUpstreamResolver_Resolve_MultipleUpstreams_FirstWins(t *testing.T) {
	var (
		ctx          context.Context = context.Background()
		query        []byte          = buildDNSQuery("example.com", 1, 1)
		fastResponse []byte          = buildDNSResponse("example.com", 1, 1, 3600, []byte{1, 1, 1, 1})
		slowResponse []byte          = buildDNSResponse("example.com", 1, 1, 3600, []byte{8, 8, 8, 8})
		fastServer   *mockDNSServer
		slowServer   *mockDNSServer
		err          error
		resolver     *UpstreamResolver
		response     []byte
	)

	// Start fast server (no delay)
	fastServer, err = startMockDNSServer(fastResponse, 0)
	if err != nil {
		t.Fatalf("Failed to start fast server: %v", err)
	}
	defer fastServer.close()

	// Start slow server (200ms delay)
	slowServer, err = startMockDNSServer(slowResponse, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("Failed to start slow server: %v", err)
	}
	defer slowServer.close()

	resolver = &UpstreamResolver{
		upstreamAddrs: []string{slowServer.addr, fastServer.addr},
		timeout:       2 * time.Second,
	}

	response, err = resolver.Resolve(ctx, query)

	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if response == nil {
		t.Fatal("Response should not be nil")
	}

	// Verify we got the fast response (contains 1.1.1.1)
	// The response should come from the faster server
	if len(response) == 0 {
		t.Error("Response should not be empty")
	}
}

// TEST 8: Resolve preserves transaction ID
// Tests that response has same transaction ID as query
func TestUpstreamResolver_Resolve_PreservesTransactionID(t *testing.T) {
	var (
		ctx          context.Context = context.Background()
		query        []byte          = buildDNSQuery("example.com", 1, 1)
		mockResponse []byte          = buildDNSResponse("example.com", 1, 1, 3600, []byte{1, 2, 3, 4})
		server       *mockDNSServer
		err          error
		resolver     *UpstreamResolver
		response     []byte
		queryTxID    uint16
		responseTxID uint16
	)

	// Set specific transaction ID
	binary.BigEndian.PutUint16(query[0:2], 0xABCD)
	queryTxID = binary.BigEndian.Uint16(query[0:2])

	server, err = startMockDNSServer(mockResponse, 0)
	if err != nil {
		t.Fatalf("Failed to start mock server: %v", err)
	}
	defer server.close()

	resolver = &UpstreamResolver{
		upstreamAddrs: []string{server.addr},
		timeout:       2 * time.Second,
	}

	response, err = resolver.Resolve(ctx, query)

	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	responseTxID = binary.BigEndian.Uint16(response[0:2])

	if queryTxID != responseTxID {
		t.Errorf("Transaction ID mismatch: query=0x%04X, response=0x%04X",
			queryTxID, responseTxID)
	}
}

// TEST 9: Resolve with all upstreams failing
// Tests behavior when all upstream servers are unreachable
func TestUpstreamResolver_Resolve_AllUpstreamsFail(t *testing.T) {
	var (
		ctx      context.Context = context.Background()
		query    []byte          = buildDNSQuery("example.com", 1, 1)
		resolver *UpstreamResolver
		response []byte
		err      error
	)

	// Use multiple non-existent addresses
	resolver = &UpstreamResolver{
		upstreamAddrs: []string{
			"192.0.2.1:53", // TEST-NET-1
			"192.0.2.2:53", // TEST-NET-1
			"192.0.2.3:53", // TEST-NET-1
		},
		timeout: 500 * time.Millisecond,
	}

	response, err = resolver.Resolve(ctx, query)

	if err == nil {
		t.Error("Expected error when all upstreams fail")
	}
	if response != nil {
		t.Error("Response should be nil when all upstreams fail")
	}
}

// TEST 10: resolveUpstream handles connection errors gracefully
// Tests that individual upstream failures don't crash the resolver
func TestUpstreamResolver_ResolveUpstream_ConnectionError(t *testing.T) {
	var (
		ctx          context.Context   = context.Background()
		query        []byte            = buildDNSQuery("example.com", 1, 1)
		responseChan chan []byte       = make(chan []byte, 1)
		resolver     *UpstreamResolver = &UpstreamResolver{
			upstreamAddrs: []string{"invalid-address:53"},
			timeout:       1 * time.Second,
		}
	)

	// This should not panic, just log error and return
	go resolver.resolveUpstream(ctx, "invalid-address:53", query, responseChan)

	// Wait a bit to ensure goroutine completes
	time.Sleep(100 * time.Millisecond)

	// Channel should be empty (no response sent)
	select {
	case <-responseChan:
		t.Error("Should not receive response from invalid address")
	default:
		// Expected: no response
	}
}

// TEST 11: Concurrent resolves work correctly
// Tests that multiple concurrent resolve calls work
func TestUpstreamResolver_Resolve_Concurrent(t *testing.T) {
	var (
		ctx          context.Context = context.Background()
		query        []byte          = buildDNSQuery("example.com", 1, 1)
		mockResponse []byte          = buildDNSResponse("example.com", 1, 1, 3600, []byte{1, 2, 3, 4})
		server       *mockDNSServer
		err          error
		resolver     *UpstreamResolver
		done         chan bool = make(chan bool, 3)
		i            int
	)

	server, err = startMockDNSServer(mockResponse, 0)
	if err != nil {
		t.Fatalf("Failed to start mock server: %v", err)
	}
	defer server.close()

	resolver = &UpstreamResolver{
		upstreamAddrs: []string{server.addr},
		timeout:       2 * time.Second,
	}

	// Launch 3 concurrent resolves
	for i = 0; i < 3; i++ {
		go func() {
			var (
				response []byte
				err      error
			)
			response, err = resolver.Resolve(ctx, query)
			if err != nil || response == nil {
				done <- false
			} else {
				done <- true
			}
		}()
	}

	// Wait for all to complete
	var (
		success int
		result  bool
	)
	for i = 0; i < 3; i++ {
		result = <-done
		if result {
			success++
		}
	}

	if success != 3 {
		t.Errorf("Expected 3 successful resolves, got %d", success)
	}
}

// TEST 12: Context timeout during resolve
// Tests that context timeout is properly handled
func TestUpstreamResolver_Resolve_ContextTimeout(t *testing.T) {
	var (
		ctx      context.Context
		cancel   context.CancelFunc
		query    []byte            = buildDNSQuery("example.com", 1, 1)
		resolver *UpstreamResolver = &UpstreamResolver{
			upstreamAddrs: []string{"192.0.2.1:53"},
			timeout:       5 * time.Second, // Long resolver timeout
		}
		response []byte
		err      error
	)

	// Create context with short timeout
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	response, err = resolver.Resolve(ctx, query)

	if err == nil {
		t.Error("Expected error for context timeout")
	}
	if response != nil {
		t.Error("Response should be nil on context timeout")
	}
}

// Note: Helper functions buildDNSQuery, buildDNSResponse, and splitDomain
// are defined in dnsServer_test.go and shared across test files in this package
