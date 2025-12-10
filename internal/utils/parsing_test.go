package utils

import (
	"encoding/binary"
	"testing"
)

// TEST 1: Parse simple domain query
// Tests parsing a basic DNS query for example.com
func TestParseQuery_SimpleDomain(t *testing.T) {
	var (
		query []byte = buildDNSQuery("example.com", 1, 1)
		info  *QueryInfo
		err   error
	)

	info, err = ParseQuery(query)

	if err != nil {
		t.Fatalf("ParseQuery failed: %v", err)
	}
	if info.Domain != "example.com" {
		t.Errorf("Expected domain 'example.com', got '%s'", info.Domain)
	}
	if info.QType != 1 {
		t.Errorf("Expected QType 1 (A), got %d", info.QType)
	}
	if info.QClass != 1 {
		t.Errorf("Expected QClass 1 (IN), got %d", info.QClass)
	}
	if info.CacheKey != "example.com:1" {
		t.Errorf("Expected cache key 'example.com:1', got '%s'", info.CacheKey)
	}
}

// TEST 2: Parse subdomain query
// Tests parsing queries with multiple labels
func TestParseQuery_Subdomain(t *testing.T) {
	var (
		query []byte = buildDNSQuery("api.staging.example.com", 1, 1)
		info  *QueryInfo
		err   error
	)

	info, err = ParseQuery(query)

	if err != nil {
		t.Fatalf("ParseQuery failed: %v", err)
	}
	if info.Domain != "api.staging.example.com" {
		t.Errorf("Expected 'api.staging.example.com', got '%s'", info.Domain)
	}
}

// TEST 3: Parse different query types
// Tests parsing various DNS record types (A, AAAA, MX, etc.)
func TestParseQuery_DifferentTypes(t *testing.T) {
	var tests = []struct {
		domain   string
		qtype    uint16
		expected string
	}{
		{"example.com", 1, "example.com:1"},   // A record
		{"example.com", 28, "example.com:28"}, // AAAA record
		{"example.com", 15, "example.com:15"}, // MX record
		{"example.com", 16, "example.com:16"}, // TXT record
		{"example.com", 5, "example.com:5"},   // CNAME record
	}

	var (
		i    int
		test struct {
			domain   string
			qtype    uint16
			expected string
		}
		query []byte
		info  *QueryInfo
		err   error
	)

	for i, test = range tests {
		query = buildDNSQuery(test.domain, test.qtype, 1)
		info, err = ParseQuery(query)

		if err != nil {
			t.Errorf("Test %d: ParseQuery failed: %v", i, err)
			continue
		}
		if info.CacheKey != test.expected {
			t.Errorf("Test %d: Expected cache key '%s', got '%s'",
				i, test.expected, info.CacheKey)
		}
		if info.QType != test.qtype {
			t.Errorf("Test %d: Expected QType %d, got %d",
				i, test.qtype, info.QType)
		}
	}
}

// TEST 4: Query too short returns error
// Tests that queries shorter than 12 bytes are rejected
func TestParseQuery_TooShort(t *testing.T) {
	var (
		query []byte = make([]byte, 10) // Less than 12 bytes
		info  *QueryInfo
		err   error
	)

	info, err = ParseQuery(query)

	if err == nil {
		t.Error("Expected error for short query")
	}
	if info != nil {
		t.Error("Should return nil QueryInfo on error")
	}
}

// TEST 5: Invalid domain length returns error
// Tests that malformed domain names are detected
func TestParseQuery_InvalidDomainLength(t *testing.T) {
	var (
		query []byte = make([]byte, 12)
		err   error
	)

	// Set up a query with invalid length that exceeds buffer
	query[12-1] = 100                          // Last byte before position 12 would be domain length
	query = append(query, 50)                  // Add a large length value
	query = append(query, make([]byte, 10)...) // But not enough data

	_, err = ParseQuery(query)

	if err == nil {
		t.Error("Expected error for invalid domain length")
	}
}

// TEST 6: Query missing QTYPE/QCLASS returns error
// Tests that incomplete queries are rejected
func TestParseQuery_MissingTypeClass(t *testing.T) {
	var (
		query []byte = make([]byte, 12)
		err   error
	)

	// Add a valid domain terminator but no space for QTYPE/QCLASS
	query = append(query, 0) // Domain ends with 0

	_, err = ParseQuery(query)

	if err == nil {
		t.Error("Expected error for missing QTYPE/QCLASS")
	}
}

// TEST 7: Extract TTL from simple response
// Tests extracting TTL from a response with one answer
func TestExtractTTL_SimpleResponse(t *testing.T) {
	var (
		response []byte = buildDNSResponse("example.com", 1, 1, 3600, []byte{192, 168, 1, 1})
		ttl      uint32
	)

	ttl = ExtractTTL(response)

	if ttl != 3600 {
		t.Errorf("Expected TTL 3600, got %d", ttl)
	}
}

// TEST 8: Extract minimum TTL from multiple answers
// Tests that the minimum TTL is returned when there are multiple records
func TestExtractTTL_MultipleAnswers(t *testing.T) {
	var (
		response []byte
		ttl      uint32
		minTTL   uint32 = 300
	)

	// Build response with multiple answers having different TTLs
	response = buildDNSResponseMultiple("example.com", []uint32{3600, 300, 1800})

	ttl = ExtractTTL(response)

	if ttl != minTTL {
		t.Errorf("Expected minimum TTL %d, got %d", minTTL, ttl)
	}
}

// TEST 9: Extract TTL from short response returns default
// Tests that malformed responses return default TTL
func TestExtractTTL_ShortResponse(t *testing.T) {
	var (
		response   []byte = make([]byte, 10) // Less than 12 bytes
		ttl        uint32
		defaultTTL uint32 = 300
	)

	ttl = ExtractTTL(response)

	if ttl != defaultTTL {
		t.Errorf("Expected default TTL %d, got %d", defaultTTL, ttl)
	}
}

// TEST 10: Extract TTL handles compression pointers
// Tests that DNS name compression (0xC0) is handled correctly
func TestExtractTTL_CompressionPointer(t *testing.T) {
	var (
		response []byte
		ttl      uint32
		expected uint32 = 1800
	)

	// Build response with compression pointer in answer
	response = buildDNSResponseWithCompression("example.com", 1, 1, expected, []byte{10, 0, 0, 1})

	ttl = ExtractTTL(response)

	if ttl != expected {
		t.Errorf("Expected TTL %d, got %d", expected, ttl)
	}
}

// TEST 11: Parse query with compression pointer
// Tests that compression pointers in queries are skipped
func TestParseQuery_WithCompressionPointer(t *testing.T) {
	var (
		query []byte = make([]byte, 12)
		info  *QueryInfo
		err   error
	)

	// Add domain with compression pointer
	query = append(query, 7) // Length of "example"
	query = append(query, []byte("example")...)
	query = append(query, 0xC0, 0x0C) // Compression pointer
	query = append(query, 0)          // End of domain

	// Add QTYPE and QCLASS
	query = append(query, 0, 1) // QTYPE = 1
	query = append(query, 0, 1) // QCLASS = 1

	info, err = ParseQuery(query)

	if err != nil {
		t.Fatalf("ParseQuery failed: %v", err)
	}
	if info.Domain != "example" {
		t.Errorf("Expected domain 'example', got '%s'", info.Domain)
	}
}

// TEST 12: Extract TTL with no answers returns default
// Tests that responses with zero answers return default TTL
func TestExtractTTL_NoAnswers(t *testing.T) {
	var (
		response   []byte = make([]byte, 12)
		ttl        uint32
		defaultTTL uint32 = 3600 // The default when no answers found
	)

	// Set QDCOUNT = 0, ANCOUNT = 0
	binary.BigEndian.PutUint16(response[4:6], 0)
	binary.BigEndian.PutUint16(response[6:8], 0)

	ttl = ExtractTTL(response)

	if ttl != defaultTTL {
		t.Errorf("Expected default TTL %d, got %d", defaultTTL, ttl)
	}
}

// TEST 13: Cache key format with different types
// Tests that cache keys are formatted correctly for different query types
func TestParseQuery_CacheKeyFormat(t *testing.T) {
	var tests = []struct {
		domain      string
		qtype       uint16
		expectedKey string
	}{
		{"google.com", 1, "google.com:1"},
		{"mail.google.com", 15, "mail.google.com:15"},
		{"ipv6.google.com", 28, "ipv6.google.com:28"},
	}

	var (
		i    int
		test struct {
			domain      string
			qtype       uint16
			expectedKey string
		}
		query []byte
		info  *QueryInfo
		err   error
	)

	for i, test = range tests {
		query = buildDNSQuery(test.domain, test.qtype, 1)
		info, err = ParseQuery(query)

		if err != nil {
			t.Errorf("Test %d: ParseQuery failed: %v", i, err)
			continue
		}
		if info.CacheKey != test.expectedKey {
			t.Errorf("Test %d: Expected cache key '%s', got '%s'",
				i, test.expectedKey, info.CacheKey)
		}
	}
}

// ============================================================================
// HELPER FUNCTIONS FOR BUILDING DNS PACKETS
// ============================================================================

// buildDNSQuery creates a minimal DNS query packet
func buildDNSQuery(domain string, qtype uint16, qclass uint16) []byte {
	var (
		query    []byte = make([]byte, 12)
		labels   []string
		i        int
		label    string
		labelLen int
	)

	// DNS Header (12 bytes) - already zero-initialized
	binary.BigEndian.PutUint16(query[0:2], 0x1234) // Transaction ID
	binary.BigEndian.PutUint16(query[4:6], 1)      // QDCOUNT = 1

	// Question section - domain name
	labels = splitDomain(domain)
	for i, label = range labels {
		_ = i
		labelLen = len(label)
		query = append(query, byte(labelLen))
		query = append(query, []byte(label)...)
	}
	query = append(query, 0) // End of domain name

	// QTYPE and QCLASS
	var typeClass []byte = make([]byte, 4)
	binary.BigEndian.PutUint16(typeClass[0:2], qtype)
	binary.BigEndian.PutUint16(typeClass[2:4], qclass)
	query = append(query, typeClass...)

	return query
}

// buildDNSResponse creates a minimal DNS response packet
func buildDNSResponse(domain string, qtype uint16, qclass uint16, ttl uint32, rdata []byte) []byte {
	var (
		response []byte = make([]byte, 12)
		labels   []string
		i        int
		label    string
		labelLen int
	)

	// DNS Header
	binary.BigEndian.PutUint16(response[0:2], 0x1234) // Transaction ID
	binary.BigEndian.PutUint16(response[2:4], 0x8180) // Flags (response)
	binary.BigEndian.PutUint16(response[4:6], 1)      // QDCOUNT = 1
	binary.BigEndian.PutUint16(response[6:8], 1)      // ANCOUNT = 1

	// Question section
	labels = splitDomain(domain)
	for i, label = range labels {
		_ = i
		labelLen = len(label)
		response = append(response, byte(labelLen))
		response = append(response, []byte(label)...)
	}
	response = append(response, 0) // End of domain

	var typeClass []byte = make([]byte, 4)
	binary.BigEndian.PutUint16(typeClass[0:2], qtype)
	binary.BigEndian.PutUint16(typeClass[2:4], qclass)
	response = append(response, typeClass...)

	// Answer section
	response = append(response, 0xC0, 0x0C) // Name pointer to question

	// TYPE, CLASS, TTL, RDLENGTH, RDATA
	var answerData []byte = make([]byte, 10)
	binary.BigEndian.PutUint16(answerData[0:2], qtype)
	binary.BigEndian.PutUint16(answerData[2:4], qclass)
	binary.BigEndian.PutUint32(answerData[4:8], ttl)
	binary.BigEndian.PutUint16(answerData[8:10], uint16(len(rdata)))
	response = append(response, answerData...)
	response = append(response, rdata...)

	return response
}

// buildDNSResponseMultiple creates a response with multiple answers
func buildDNSResponseMultiple(domain string, ttls []uint32) []byte {
	var (
		response []byte = make([]byte, 12)
		labels   []string
		i        int
		label    string
		labelLen int
		ttl      uint32
	)

	// DNS Header
	binary.BigEndian.PutUint16(response[0:2], 0x1234)
	binary.BigEndian.PutUint16(response[2:4], 0x8180)
	binary.BigEndian.PutUint16(response[4:6], 1)
	binary.BigEndian.PutUint16(response[6:8], uint16(len(ttls))) // ANCOUNT

	// Question section
	labels = splitDomain(domain)
	for i, label = range labels {
		_ = i
		labelLen = len(label)
		response = append(response, byte(labelLen))
		response = append(response, []byte(label)...)
	}
	response = append(response, 0)

	var typeClass []byte = make([]byte, 4)
	binary.BigEndian.PutUint16(typeClass[0:2], 1) // Type A
	binary.BigEndian.PutUint16(typeClass[2:4], 1) // Class IN
	response = append(response, typeClass...)

	// Answer sections
	for i, ttl = range ttls {
		_ = i
		response = append(response, 0xC0, 0x0C) // Name pointer

		var answerData []byte = make([]byte, 10)
		binary.BigEndian.PutUint16(answerData[0:2], 1) // Type A
		binary.BigEndian.PutUint16(answerData[2:4], 1) // Class IN
		binary.BigEndian.PutUint32(answerData[4:8], ttl)
		binary.BigEndian.PutUint16(answerData[8:10], 4) // RDLENGTH = 4
		response = append(response, answerData...)
		response = append(response, 192, 168, 1, byte(i+1)) // IP address
	}

	return response
}

// buildDNSResponseWithCompression creates response with compression pointer
func buildDNSResponseWithCompression(domain string, qtype uint16, qclass uint16, ttl uint32, rdata []byte) []byte {
	// This is identical to buildDNSResponse since it already uses compression (0xC00C)
	return buildDNSResponse(domain, qtype, qclass, ttl, rdata)
}

// splitDomain splits a domain into labels
func splitDomain(domain string) []string {
	var (
		labels []string
		start  int = 0
		end    int = 0
		i      int
		ch     rune
	)

	for i, ch = range domain {
		if ch == '.' {
			if i > start {
				labels = append(labels, domain[start:i])
			}
			start = i + 1
		}
		end = i
	}

	if end >= start {
		labels = append(labels, domain[start:end+1])
	}

	return labels
}
