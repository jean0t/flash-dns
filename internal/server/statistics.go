package server

import (
	"flash-dns/internal/logger"
	"fmt"
	"sync/atomic"
)

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
		blockRate    float64
		CacheHitRate float64
	)

	blocked, allowed, cacheHits, cacheMisses = s.GetStats()
	total = blocked + allowed

	blockRate = float64(blocked) / float64(total) * 100
	CacheHitRate = float64(cacheHits) / float64(cacheHits+cacheMisses) * 100

	logger.Info(fmt.Sprintf("Status - Total: %d | Blocked: %d (%.1f%%) | Cache Hit Rate: %.1f%%", total, blocked, blockRate, CacheHitRate))
}
