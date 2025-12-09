package cache

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	CACHE_MAX_SIZE       int           = 1024
	GRACE_PERIOD         time.Duration = 5 * time.Minute // How long to accept expired entries
	POPULARITY_THRESHOLD int64         = 5               // lower than that triggers eviction
	PREFETCH_THRESHOLD   float64       = 0.8             // 80%
)

// CACHE ENTRY
type CacheEntry struct {
	CreatedAt   time.Time
	ExpiresAt   time.Time
	Response    []byte
	LastAccess  atomic.Int64
	popularity  atomic.Int64 // internal metric
	originalTTL uint32
}

func (ce *CacheEntry) IsPopular() bool {
	return ce.popularity.Load() >= POPULARITY_THRESHOLD
}

func (ce *CacheEntry) IsStale(now time.Time) bool {
	return now.After(ce.ExpiresAt) && now.Before(ce.ExpiresAt.Add(GRACE_PERIOD))
}

func (ce *CacheEntry) IsCompletelyExpired() bool {
	var now time.Time = time.Now()
	return now.After(ce.ExpiresAt.Add(GRACE_PERIOD))
}

func (ce *CacheEntry) ShouldPrefetch() bool {
	if !ce.IsPopular() {
		return false
	}

	var (
		now time.Time     = time.Now()
		age time.Duration = now.Sub(ce.CreatedAt)
		ttl time.Duration = time.Duration(ce.originalTTL) * time.Second
	)

	return age >= time.Duration(float64(ttl)*PREFETCH_THRESHOLD)
}

func (ce *CacheEntry) increasePopularity() {
	_ = ce.popularity.Add(1)
}

func (ce *CacheEntry) decreasePopularity() {
	_ = ce.popularity.Add(-1)
}

func (ce *CacheEntry) TimeSinceLastAccess() time.Duration {
	var lastAccess time.Time = time.Unix(ce.LastAccess.Load(), 0)
	return time.Since(lastAccess)
}

// DNS CACHE
type DNSCache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
	maxSize int
}

func NewDNSCache() *DNSCache {
	return &DNSCache{
		entries: make(map[string]*CacheEntry, CACHE_MAX_SIZE),
		maxSize: CACHE_MAX_SIZE,
	}
}

func (c *DNSCache) Get(key string) ([]byte, bool, bool) {
	var (
		entry        *CacheEntry = nil
		found        bool        = false
		needsRefresh bool        = false
		now          time.Time   = time.Now()
	)
	c.mu.RLock()
	entry, found = c.entries[key]
	c.mu.RUnlock()

	if !found {
		return nil, found, needsRefresh
	}

	// update statistics
	entry.increasePopularity()
	entry.LastAccess.Store(now.Unix())

	if entry.IsCompletelyExpired() {
		c.mu.Lock()
		delete(c.entries, key)
		found = false
		c.mu.Unlock()

		return nil, found, needsRefresh
	}

	if entry.IsStale(now) {
		needsRefresh = true
	}

	if entry.ShouldPrefetch() {
		needsRefresh = true
	}

	return entry.Response, found, needsRefresh
}

func (c *DNSCache) Set(key string, response []byte, ttl uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= c.maxSize {
		var exists bool
		if _, exists = c.entries[key]; !exists {
			c.evictOne()
		}
	}

	var (
		now time.Time = time.Now()
	)
	c.entries[key] = &CacheEntry{
		Response:    response,
		CreatedAt:   now,
		ExpiresAt:   now.Add(time.Duration(ttl) * time.Second),
		originalTTL: ttl,
	}

	c.entries[key].LastAccess.Store(now.Unix())
	c.entries[key].popularity.Store(1)
}

func (c *DNSCache) Clean() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.entries {
		if entry.IsCompletelyExpired() {
			delete(c.entries, key)
		}
	}
}

func (c *DNSCache) evictOne() {
	var (
		worstKey        string
		worstScore      float64 = -1
		timeSinceAccess float64
		popularity      float64
		score           float64
	)
	for k, v := range c.entries {
		timeSinceAccess = v.TimeSinceLastAccess().Seconds()
		popularity = float64(v.popularity.Load())
		score = timeSinceAccess / (popularity + 1)

		if score > worstScore {
			worstScore = score
			worstKey = k
		}
	}

	if worstKey != "" {
		delete(c.entries, worstKey)
	}
}
