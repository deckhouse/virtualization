/*
Copyright 2025 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cache

import (
	"context"
	"sync"
	"time"
)

// TTLCache represents a cache with Time-To-Live for its elements.
// Elements are automatically removed from the cache after their lifetime expires.
type TTLCache struct {
	mu      sync.RWMutex
	data    map[string]*cacheEntry
	ttl     time.Duration
	running bool
}

// cacheEntry represents a cache element with an expiration time.
type cacheEntry struct {
	entry  any
	expiry time.Time
}

// NewTTLCache creates a new cache instance with the specified element lifetime.
// The cache won't start cleaning up until Start() is called.
func NewTTLCache(ttl time.Duration) *TTLCache {
	return &TTLCache{
		data: make(map[string]*cacheEntry),
		ttl:  ttl,
	}
}

// Start begins the background cleanup process for expired entries.
// The cleanup will continue until the provided context is canceled.
func (c *TTLCache) Start(ctx context.Context) {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.mu.Unlock()

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		defer func() {
			c.mu.Lock()
			c.running = false
			c.mu.Unlock()
		}()

		for {
			select {
			case <-ticker.C:
				c.cleanupExpiredEntries()
			case <-ctx.Done():
				return
			}
		}
	}()
}

// cleanupExpiredEntries performs a single round of cleanup for expired cache entries.
func (c *TTLCache) cleanupExpiredEntries() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.data {
		if now.After(entry.expiry) {
			delete(c.data, key)
		}
	}
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
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.data[key]
	if !exists || time.Now().After(entry.expiry) {
		return nil, false
	}
	return entry.entry, true
}
