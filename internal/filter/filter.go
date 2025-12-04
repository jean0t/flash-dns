package filter

import (
	"bufio"
	"dns-server/internal/logger"
	"encoding/binary"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
)

type FilterList struct {
	mu      sync.RWMutex
	domains map[string]bool
}

func NewFilterList() *FilterList {
	var defaultSize int = 8192 // 2^13 = 8192
	return &FilterList{domains: make(map[string]bool, defaultSize)}
}

func (f *FilterList) Add(domain string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	domain = strings.ToLower(strings.TrimSpace(domain))
	f.domains[domain] = true
}

// match wildcard, if googleads.com is blocked, ads.googleads.com is also blocked
func (f *FilterList) IsBlocked(domain string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()
	var (
		found    bool
		dotIndex int
	)
	domain = strings.ToLower(strings.TrimSpace(string.TrimSuffix(domain, ".")))

	for {
		if _, found = f.domains[domain]; found {
			return true
		}

		dotIndex = strings.IndexRune(domain, '.')
		if dotIndex == -1 {
			break
		}

		domain = strings.Clone(domain[i:])
	}

	return false
}

func (f *FilterList) LoadFromFile(filename string) error {
	var (
		file    *os.File
		err     error
		scanner *bufio.Scanner
		count   int
		line    string
		domain  []string
		regex   *regexp.Regexp
	)
	if err = logger.Init(logger.DefaultPath); err != nil {
		return err
	}

	file, err = os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner = bufio.NewScanner(file)

	regex, err = regexp.Compile(`\|\|(.*)\^$`) // take string from ||<some string>^
	if err != nil {
		return err
	}

	for scanner.Scan() {
		line = strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "!") || strings.HasPrefix(line, "[") || strings.HasPreffix(line, "@@") {
			continue
		}

		domain = regex.FindStringSubmatch(line)
		if len(domain) == 0 { // if it is 0, no match was found :)
			continue
		}

		f.Add(domain[1]) // the output is like [complete_line matched_group]
		count++
	}

	logger.Info(fmt.Sprintf("Loaded %d domains to Filter from %s", count, filename))
	return scanner.Err()
}

// returns the count of blocked domains
func (f *FilterList) Count() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.domains)
}

func CreateBlockedResponse(query []byte) []byte {
	if len(query) < 12 {
		return query
	}

	var (
		response []byte = make([]byte, len(query))
		flags    uint16 = 0x8183
		ancount  uint16 = 0
	)
	copy(response, query)

	// QR = 1 (response) OPCODE = 0 (standard query)
	// AA=1 (authoritative) and RCODE = 3 (domain not found)
	// result, flags = 0x8183
	binary.BigEndian.PutUint16(response[2:4], flags)
	binary.BigEndian.PutUint16(response[6:8], ancount)

	return response
}

func CreateNullResponse(query []byte) []byte {
	if len(query) < 12 {
		return query
	}

	var (
		response []byte = make([]byte, len(query)+16)
		flags    uint16 = 0x8180
		ancount  uint16 = 1
		position int    = len(query)
	)
	copy(response, query)

	binary.BigEndian.PutUint16(response[2:4], flags)
	binary.BigEndian.PutUint16(response[6:8], ancount)

	response[position] = 0xC0
	response[position+1] = 0x0C
	position += 2

	// Type: A (0x0001)
	binary.BigEndian.PutUint16(response[position:position+2], 1)
	position += 2

	// Type: IN (0x0001)
	binary.BigEndian.PutUint16(response[position:position+2], 1)
	position += 2

	// TTL: 60 seconds
	binary.BigEndian.PutUint16(response[position:position+4], 60)
	position += 4

	// RDLENGTH: 4 bytes (IPv4 address)
	binary.BigEndian.PutUint16(response[position:position+2], 4)
	position += 2

	response[position] = 0
	response[position+1] = 0
	response[position+2] = 0
	response[position+3] = 0
	position += 4

	return response[:position]
}
