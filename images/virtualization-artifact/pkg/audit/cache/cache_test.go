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
	"testing"
	"time"
)

func TestTTLCache_Add(t *testing.T) {
	cache := NewTTLCache(100 * time.Millisecond)
	defer cache.Stop()

	cache.Add("test-key", "test-value")

	value, exists := cache.Get("test-key")
	if !exists {
		t.Error("Expected item to exist in cache immediately after adding")
	}
	if value != "test-value" {
		t.Errorf("Expected value to be 'test-value', got %v", value)
	}
}

func TestTTLCache_Get_NonExistent(t *testing.T) {
	cache := NewTTLCache(100 * time.Millisecond)
	defer cache.Stop()

	_, exists := cache.Get("non-existent-key")
	if exists {
		t.Error("Expected non-existent item to not exist in cache")
	}
}

func TestTTLCache_Expiration(t *testing.T) {
	cache := NewTTLCache(50 * time.Millisecond)
	defer cache.Stop()

	cache.Add("expiring-key", "expiring-value")

	_, exists := cache.Get("expiring-key")
	if !exists {
		t.Error("Expected item to exist in cache immediately after adding")
	}

	time.Sleep(100 * time.Millisecond)

	_, exists = cache.Get("expiring-key")
	if exists {
		t.Error("Expected item to be removed from cache after TTL expiration")
	}
}

func TestTTLCache_Overwrite(t *testing.T) {
	cache := NewTTLCache(100 * time.Millisecond)
	defer cache.Stop()

	cache.Add("key", "value1")

	cache.Add("key", "value2")

	value, exists := cache.Get("key")
	if !exists {
		t.Error("Expected item to exist in cache")
	}
	if value != "value2" {
		t.Errorf("Expected value to be 'value2', got %v", value)
	}
}

func TestTTLCache_CleanupRoutine(t *testing.T) {
	cache := NewTTLCache(50 * time.Millisecond)
	defer cache.Stop()

	for i := range 10 {
		cache.Add(string(rune('a'+i)), i)
	}

	time.Sleep(200 * time.Millisecond)

	for i := range 10 {
		_, exists := cache.Get(string(rune('a' + i)))
		if exists {
			t.Errorf("Expected item %c to be removed by cleanup routine", 'a'+i)
		}
	}
}

func TestTTLCache_Stop(t *testing.T) {
	cache := NewTTLCache(100 * time.Millisecond)

	cache.Stop()

	cache.Add("key", "value")

	value, exists := cache.Get("key")
	if !exists {
		t.Error("Expected item to exist in cache even after stopping")
	}

	if value != "value" {
		t.Errorf("Expected value to be 'value', got %v", value)
	}
}
