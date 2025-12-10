package utils

import (
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
)

var builderPool = sync.Pool{
	New: func() interface{} {
		return &strings.Builder{}
	},
}

type QueryInfo struct {
	Domain   string
	CacheKey string
	QType    uint16
	QClass   uint16
}

func ParseQuery(query []byte) (*QueryInfo, error) {
	var queryLength int = len(query)
	if queryLength < 12 {
		return nil, fmt.Errorf("query too short: %d bytes", len(query))
	}

	var (
		builder  *strings.Builder = builderPool.Get().(*strings.Builder)
		position int              = 12
		length   int              = 0
		domain   string
		qtype    uint16
		qclass   uint16
		cacheKey string
	)
	builder.Reset()
	defer builderPool.Put(builder)

	for position < queryLength {
		length = int(query[position])

		if length == 0 {
			position++
			break
		}

		if length >= 192 {
			position += 2
			continue
		}

		if builder.Len() > 0 {
			builder.WriteRune('.')
		}
		position++

		if position+length > queryLength {
			return nil, fmt.Errorf("invalid domain name length")
		}

		builder.Write(query[position : position+length])
		position += length
	}
	domain = builder.String()

	if position+4 > queryLength {
		return nil, fmt.Errorf("query too short for QTYPE/QCLASS")
	}

	qtype = binary.BigEndian.Uint16(query[position : position+2])
	qclass = binary.BigEndian.Uint16(query[position+2 : position+4])

	cacheKey = fmt.Sprintf("%s:%d", domain, qtype)

	return &QueryInfo{Domain: domain, QType: qtype, QClass: qclass, CacheKey: cacheKey}, nil
}

func ExtractTTL(response []byte) uint32 {
	if len(response) < 12 {
		return 300
	}

	var (
		questions, answers uint16
		position           int
	)
	questions = binary.BigEndian.Uint16(response[4:6])
	answers = binary.BigEndian.Uint16(response[6:8])

	// start after header
	position := 12

	// skip questions
	for q := 0; q < int(questions); q++ {
		skipName(response, &position)
		position += 4 // QTYPE + QCLASS
	}

	minTTL := uint32(3600)

	// read answers
	for a := 0; a < int(answers); a++ {
		skipName(response, &position)

		if position+10 > len(response) {
			break
		}

		// TYPE, CLASS, TTL
		ttl := binary.BigEndian.Uint32(response[position+4 : position+8])
		if ttl < minTTL {
			minTTL = ttl
		}

		rdlen := binary.BigEndian.Uint16(response[position+8 : position+10])
		position += 10 + int(rdlen)
	}

	return minTTL
}

func skipName(p []byte, position *int) {
	var (
		length int
	)
	for {
		if *position >= len(p) {
			return
		}
		b := p[*position]

		// pointer
		if b&0xC0 == 0xC0 {
			*position += 2
			return
		}

		// zero terminator
		if b == 0 {
			*position++
			return
		}

		// label
		length = int(b)
		*position += 1 + length
	}
}
