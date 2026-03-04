package server2

import (
	"fmt"
	"sync"
	"testing"

	"github.com/onflow/cadence/sema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLRUCheckerCache_PutAndGet(t *testing.T) {
	cache := NewLRUCheckerCache(10)

	entry := &CheckerEntry{Checker: &sema.Checker{}, Valid: true}
	cache.Put("key1", entry)

	got, ok := cache.Get("key1")
	require.True(t, ok)
	assert.Equal(t, entry, got)
}

func TestLRUCheckerCache_GetMissing(t *testing.T) {
	cache := NewLRUCheckerCache(10)

	got, ok := cache.Get("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestLRUCheckerCache_EvictionAtCapacity(t *testing.T) {
	cache := NewLRUCheckerCache(2)

	e1 := &CheckerEntry{Checker: &sema.Checker{}, Valid: true}
	e2 := &CheckerEntry{Checker: &sema.Checker{}, Valid: true}
	e3 := &CheckerEntry{Checker: &sema.Checker{}, Valid: true}

	cache.Put("key1", e1)
	cache.Put("key2", e2)
	cache.Put("key3", e3) // should evict key1

	_, ok := cache.Get("key1")
	assert.False(t, ok, "key1 should have been evicted")

	got2, ok := cache.Get("key2")
	assert.True(t, ok)
	assert.Equal(t, e2, got2)

	got3, ok := cache.Get("key3")
	assert.True(t, ok)
	assert.Equal(t, e3, got3)
}

func TestLRUCheckerCache_AccessRefreshesOrder(t *testing.T) {
	cache := NewLRUCheckerCache(2)

	e1 := &CheckerEntry{Checker: &sema.Checker{}, Valid: true}
	e2 := &CheckerEntry{Checker: &sema.Checker{}, Valid: true}
	e3 := &CheckerEntry{Checker: &sema.Checker{}, Valid: true}

	cache.Put("key1", e1)
	cache.Put("key2", e2)

	// Access key1 to refresh it — key2 becomes LRU
	cache.Get("key1")

	cache.Put("key3", e3) // should evict key2 (LRU)

	_, ok := cache.Get("key2")
	assert.False(t, ok, "key2 should have been evicted")

	got1, ok := cache.Get("key1")
	assert.True(t, ok, "key1 should still be present")
	assert.Equal(t, e1, got1)

	got3, ok := cache.Get("key3")
	assert.True(t, ok, "key3 should be present")
	assert.Equal(t, e3, got3)
}

func TestLRUCheckerCache_Delete(t *testing.T) {
	cache := NewLRUCheckerCache(10)

	e1 := &CheckerEntry{Checker: &sema.Checker{}, Valid: true}
	e2 := &CheckerEntry{Checker: &sema.Checker{}, Valid: true}

	cache.Put("key1", e1)
	cache.Put("key2", e2)

	cache.Delete("key1")

	_, ok := cache.Get("key1")
	assert.False(t, ok, "key1 should have been deleted")

	got2, ok := cache.Get("key2")
	assert.True(t, ok, "key2 should still exist")
	assert.Equal(t, e2, got2)
}

func TestLRUCheckerCache_DeleteNonexistent(t *testing.T) {
	cache := NewLRUCheckerCache(10)
	// Should not panic
	cache.Delete("nonexistent")
}

func TestLRUCheckerCache_DeleteByPrefix(t *testing.T) {
	cache := NewLRUCheckerCache(10)

	cache.Put("s3://bucket/file1.cdc", &CheckerEntry{Valid: true})
	cache.Put("s3://bucket/file2.cdc", &CheckerEntry{Valid: true})
	cache.Put("file:///local/file3.cdc", &CheckerEntry{Valid: true})
	cache.Put("s3://other/file4.cdc", &CheckerEntry{Valid: true})

	cache.DeleteByPrefix("s3://bucket/")

	_, ok := cache.Get("s3://bucket/file1.cdc")
	assert.False(t, ok, "file1 should have been removed")

	_, ok = cache.Get("s3://bucket/file2.cdc")
	assert.False(t, ok, "file2 should have been removed")

	_, ok = cache.Get("file:///local/file3.cdc")
	assert.True(t, ok, "file3 should still exist")

	_, ok = cache.Get("s3://other/file4.cdc")
	assert.True(t, ok, "file4 should still exist")
}

func TestLRUCheckerCache_DeleteByPrefixNoMatch(t *testing.T) {
	cache := NewLRUCheckerCache(10)
	cache.Put("key1", &CheckerEntry{Valid: true})

	cache.DeleteByPrefix("nomatch")

	_, ok := cache.Get("key1")
	assert.True(t, ok, "key1 should still exist")
}

func TestLRUCheckerCache_PutOverwrite(t *testing.T) {
	cache := NewLRUCheckerCache(10)

	e1 := &CheckerEntry{Valid: true}
	e2 := &CheckerEntry{Valid: false}

	cache.Put("key1", e1)
	cache.Put("key1", e2) // overwrite

	got, ok := cache.Get("key1")
	require.True(t, ok)
	assert.Equal(t, e2, got, "should return the updated entry")
}

func TestLRUCheckerCache_ConcurrentAccess(t *testing.T) {
	cache := NewLRUCheckerCache(50)

	var wg sync.WaitGroup
	wg.Add(100)

	for i := 0; i < 100; i++ {
		go func(i int) {
			defer wg.Done()
			key := CacheKey(fmt.Sprintf("key%d", i))
			entry := &CheckerEntry{Valid: true}

			cache.Put(key, entry)
			cache.Get(key)

			if i%3 == 0 {
				cache.Delete(key)
			}
			if i%7 == 0 {
				cache.DeleteByPrefix("key1")
			}
		}(i)
	}

	wg.Wait()
	// If we get here without a race condition panic, the test passes
}
