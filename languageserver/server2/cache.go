package server2

import (
	"container/list"
	"strings"
	"sync"

	"github.com/onflow/cadence/sema"
)

// CacheKey is the key type for the LRU checker cache (typically a document URI).
type CacheKey = string

// CheckerEntry holds a semantic checker and its validity state.
type CheckerEntry struct {
	Checker *sema.Checker
	Valid   bool
}

// lruItem is stored as the Value in each list.Element.
type lruItem struct {
	key   CacheKey
	entry *CheckerEntry
}

// LRUCheckerCache is a thread-safe LRU cache for Cadence semantic checkers.
// It uses container/list for O(1) promotion and eviction.
type LRUCheckerCache struct {
	mu       sync.Mutex
	capacity int
	items    map[CacheKey]*list.Element
	order    *list.List // front = most recently used
}

// NewLRUCheckerCache creates a new LRU cache with the given capacity.
// Capacity must be at least 1.
func NewLRUCheckerCache(capacity int) *LRUCheckerCache {
	if capacity < 1 {
		capacity = 1
	}
	return &LRUCheckerCache{
		capacity: capacity,
		items:    make(map[CacheKey]*list.Element, capacity),
		order:    list.New(),
	}
}

// Get retrieves an entry by key and moves it to the front (most recently used).
// Returns nil and false if the key is not found.
func (c *LRUCheckerCache) Get(key CacheKey) (*CheckerEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		return nil, false
	}

	c.order.MoveToFront(elem)
	return elem.Value.(*lruItem).entry, true
}

// Put inserts or updates an entry. If the cache is at capacity, the least
// recently used entry is evicted.
func (c *LRUCheckerCache) Put(key CacheKey, entry *CheckerEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry
	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		elem.Value.(*lruItem).entry = entry
		return
	}

	// Evict LRU if at capacity
	if c.order.Len() >= c.capacity {
		back := c.order.Back()
		if back != nil {
			c.order.Remove(back)
			delete(c.items, back.Value.(*lruItem).key)
		}
	}

	// Insert new entry at front
	elem := c.order.PushFront(&lruItem{key: key, entry: entry})
	c.items[key] = elem
}

// Delete removes a specific key from the cache.
func (c *LRUCheckerCache) Delete(key CacheKey) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		return
	}

	c.order.Remove(elem)
	delete(c.items, key)
}

// DeleteByPrefix removes all entries whose key starts with the given prefix.
func (c *LRUCheckerCache) DeleteByPrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, elem := range c.items {
		if strings.HasPrefix(key, prefix) {
			c.order.Remove(elem)
			delete(c.items, key)
		}
	}
}
