package cache

import (
	"sync"
	"time"
)

// TTLCache represents a cache with Time-To-Live for its elements.
// Elements are automatically removed from the cache after their lifetime expires.
type TTLCache struct {
	mu     sync.Mutex
	data   map[string]*cacheEntry
	ttl    time.Duration
	stopCh chan struct{}
}

// cacheEntry represents a cache element with an expiration time.
type cacheEntry struct {
	entry  any
	expiry time.Time
}

// NewTTLCache creates a new cache instance with the specified element lifetime.
// It starts a background goroutine for cleaning up expired elements.
func NewTTLCache(ttl time.Duration) *TTLCache {
	cache := &TTLCache{
		data:   make(map[string]*cacheEntry),
		ttl:    ttl,
		stopCh: make(chan struct{}),
	}
	go cache.cleanupExpiredEntries()
	return cache
}

// Add adds an element to the cache with the given key.
// The element will be automatically removed after the cache's TTL expires.
func (c *TTLCache) Add(key string, obj any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = &cacheEntry{
		entry:  obj,
		expiry: time.Now().Add(c.ttl),
	}
}

// Get returns an element from the cache by key.
// Returns the element itself and a flag indicating whether the element was found and has not expired.
func (c *TTLCache) Get(key string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.data[key]
	if !exists || time.Now().After(entry.expiry) {
		return nil, false
	}
	return entry.entry, true
}

// cleanupExpiredEntries runs periodic cleanup of expired cache elements.
// It runs in a separate goroutine and terminates when Stop() is called.
func (c *TTLCache) cleanupExpiredEntries() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			for key, entry := range c.data {
				if time.Now().After(entry.expiry) {
					delete(c.data, key)
				}
			}
			c.mu.Unlock()
		case <-c.stopCh:
			return
		}
	}
}

// Stop terminates the background cache cleanup goroutine.
// Should be called for proper cache shutdown.
func (c *TTLCache) Stop() {
	close(c.stopCh)
}
