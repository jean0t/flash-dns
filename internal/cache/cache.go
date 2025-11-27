package cache

import (
	"sync"
	"time"
)

type CacheEntry struct {
	Response  []byte
	ExpiresAt time.Time
}

type DNSCache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
}

func NewDNSCache() *DNSCache {
	return &DNSCache{
		entries: make(map[string]*CacheEntry, 1024),
	}
}

func (c *DNSCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var (
		exists bool
		entry  *CacheEntry
	)

	entry, exists = c.entries[key]
	if !exists {
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		return nil, false
	}

	return entry.Response, true
}

func (c *DNSCache) Set(key string, response []byte, ttl uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &CacheEntry{
		Response:  response,
		ExpiresAt: time.Now().Add(time.Duration(ttl) * time.Second),
	}
}

func (c *DNSCache) Clean() {
	c.mu.Lock()
	defer c.mu.Unlock()

	var now time.Time = time.Now()
	for key, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			delete(c.entries, key)
		}
	}
}
