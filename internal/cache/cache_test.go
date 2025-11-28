package cache

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestNewDnsCache(t *testing.T) {
	var cache *DNSCache = NewDNSCache()

	if cache == nil {
		t.Fatal("NewDNSCache returned nil")
	}

	if cache.entries == nil {
		t.Error("cache.entries is nil, should be initialized")
	}

	if cache.maxSize != CACHE_MAX_SIZE {
		t.Errorf("cache.maxSize returned %d, expected %d", cache.maxSize, CACHE_MAX_SIZE)
	}

	if len(cache.entries) != 0 {
		t.Errorf("cache.entries has %d entries, should be 0", len(cache.entries))
	}
}

func TestCacheGetAndSet(t *testing.T) {
	var (
		cache    *DNSCache = NewDNSCache()
		key      string    = "google.com:1"
		response []byte    = []byte{192, 168, 1, 22}
		data     []byte
		found    bool
		ttl      uint32 = 300
	)

	cache.Set(key, response, ttl)

	data, found = cache.Get(key)
	if !found {
		t.Fatal("cache.Get() returned false, expected true")
	}

	if len(data) != len(response) {
		t.Error("response length different from the retrieved data length")
	}

	for i := range response {
		if data[i] != response[i] {
			t.Errorf("data[%d] = %x, expected %x", i, data[i], response[i])
		}
	}
}

func TestCacheNonExistentEntry(t *testing.T) {
	var (
		cache *DNSCache = NewDNSCache()
		data  []byte
		found bool
	)

	data, found = cache.Get("dont-exist.com:1")

	if found {
		t.Error("cache.Get() returned true to non existent key, want false")
	}

	if data != nil {
		t.Errorf("cache.Get() returned data for non existent key, want nil")
	}
}

func TestCacheExpiration(t *testing.T) {
	var (
		cache    *DNSCache = NewDNSCache()
		key      string    = "testing-expiration.com:28"
		response []byte    = []byte{192, 128, 255, 28}
		data     []byte
		found    bool
		ttl      uint32 = 1 // 1 second of existence
	)
	cache.Set(key, response, ttl)

	data, found = cache.Get(key)
	if !found {
		t.Fatal("cache.Get() immediately after cache.Set() returned false, want true")
	}

	if len(data) != len(response) {
		t.Error("data length mismatch immediately after Set()")
	}

	time.Sleep(1200 * time.Millisecond)
	data, found = cache.Get(key)

	if found {
		t.Error("cache.Get() found equals to true for expired entry, wants false")
	}

	if data != nil {
		t.Error("cache.Get() returned data for expired entry, wants nil")
	}

	cache.mu.Lock()
	_, stillExists := cache.entries[key]
	cache.mu.Unlock()

	if stillExists {
		t.Error("expired entry still exists in cache.entries, should be deleted")
	}
}

func TestCacheClean(t *testing.T) {
	var (
		cache    *DNSCache = NewDNSCache()
		keys     []string  = []string{"first-one.com:1", "second-one.com:28", "third-case.com:1"}
		response []byte    = []byte{192, 128, 255, 35}
		ttls     []uint32  = []uint32{1, 300, 1}
	)

	for i := range keys {
		cache.Set(keys[i], response, ttls[i])
	}

	if len(cache.entries) != 3 {
		t.Fatalf("cache.entries has %d entries, wants 3", len(cache.entries))
	}

	time.Sleep(1200 * time.Millisecond)
	cache.Clean()

	if len(cache.entries) != 1 {
		t.Errorf("cache.Clean() didn't clean the 2 expired entries, len(cache.entries) is %d, expected 1", len(cache.entries))
	}
}

func TestCacheMaxSize(t *testing.T) {
	var (
		cache    *DNSCache = NewDNSCache()
		response []byte    = []byte{192, 128, 255, 28}
		ttl      uint32    = 300
		key      string
		newKey   string = "amazing-website.com:1"
		found    bool
	)

	for i := 0; i < CACHE_MAX_SIZE; i++ {
		key = fmt.Sprintf("%d.com:1", i)
		cache.Set(key, response, ttl)
	}

	if len(cache.entries) != 1024 {
		t.Fatalf("cache.entries is %d, expected %d", len(cache.entries), CACHE_MAX_SIZE)
	}

	cache.Set(newKey, response, ttl)
	_, found = cache.Get(newKey)
	if !found {
		t.Errorf("Newly added entry not found after eviction")
	}
}

func TestCacheUpdatingExistingKey(t *testing.T) {
	var (
		cache          *DNSCache = NewDNSCache()
		key            string    = "cool-website.com:1"
		firstResponse  []byte    = []byte{121, 125, 255, 20}
		secondResponse []byte    = []byte{192, 255, 198, 35}
		data1, data2   []byte
		found1, found2 bool
		ttl            uint32 = 300
	)

	cache.Set(key, firstResponse, ttl)
	data1, found1 = cache.Get(key)
	if !found1 {
		t.Fatal("cache.Get() did't retrieve after cache.Set()")
	}

	cache.Set(key, secondResponse, ttl)
	data2, found2 = cache.Get(key)
	if !found2 {
		t.Fatal("cache.Get() didn't retrieve key after updating existing key with cache.Set()")
	}

	if bytes.Equal(data1, data2) {
		t.Error("expected data1 and data2 to be different, Set() didn't update the values of the key.")
	}
}

func TestCacheConcurrentSetAndGetOperations(t *testing.T) {
	var (
		cache      *DNSCache = NewDNSCache()
		iterations int       = 100
		goroutines int       = 10
		ttl        uint32    = 300
		wg         sync.WaitGroup
		mu         sync.RWMutex
	)

	// set the keys
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				var key string = fmt.Sprintf("%d-%d.com:1", id, j)
				cache.Set(key, []byte{byte(j)}, ttl)
			}
		}(i)
	}

	wg.Wait()
	var incoherence bool = false
	// get the data
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				var key string = fmt.Sprintf("%d-%d.com:1", id, j)
				data, _ := cache.Get(key)
				if !bytes.Equal(data, []byte{byte(j)}) {
					mu.Lock()
					incoherence = true
					mu.Unlock()
				}
			}
		}(i)
	}

	wg.Wait()
	if incoherence {
		t.Error("cache.Get() didn't get the correct data response in concurrence")
	}

	t.Log("Concurrent operations completed successfully")
}
