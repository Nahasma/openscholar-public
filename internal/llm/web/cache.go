package web

import (
	"sync"
	"time"
)

// Cache provides an LRU + TTL cache for fetched URL content.
// Aligned with CC's WebFetchTool: 15-minute TTL, 50MB max size.
type Cache struct {
	mu        sync.Mutex
	entries   map[string]*CacheEntry
	order     []string // LRU order: oldest first
	totalSize int
	maxSize   int
	ttl       time.Duration
}

// NewCache creates a cache with the given max size (bytes) and TTL.
func NewCache(maxSize int, ttl time.Duration) *Cache {
	return &Cache{
		entries: make(map[string]*CacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get retrieves a cached entry, returning nil if not found or expired.
func (c *Cache) Get(url string) *CacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[url]
	if !ok {
		return nil
	}

	// Check TTL
	if time.Since(entry.CreatedAt) > c.ttl {
		c.removeLocked(url)
		return nil
	}

	// Move to end (most recently used)
	c.moveToEndLocked(url)
	return entry
}

// Set stores an entry, evicting old entries if necessary.
func (c *Cache) Set(url string, entry *CacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove existing entry if present
	if _, ok := c.entries[url]; ok {
		c.removeLocked(url)
	}

	entrySize := entry.Size
	if entrySize <= 0 {
		entrySize = len(entry.Content)
	}

	// Evict expired entries first
	c.evictExpiredLocked()

	// Evict LRU entries until we have room
	for c.totalSize+entrySize > c.maxSize && len(c.order) > 0 {
		oldest := c.order[0]
		c.removeLocked(oldest)
	}

	entry.Size = entrySize
	entry.CreatedAt = time.Now()
	c.entries[url] = entry
	c.order = append(c.order, url)
	c.totalSize += entrySize
}

// Clear removes all cached entries.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*CacheEntry)
	c.order = nil
	c.totalSize = 0
}

// removeLocked removes an entry by URL. Caller must hold c.mu.
func (c *Cache) removeLocked(url string) {
	entry, ok := c.entries[url]
	if !ok {
		return
	}
	c.totalSize -= entry.Size
	delete(c.entries, url)

	for i, u := range c.order {
		if u == url {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
}

// moveToEndLocked moves a URL to the end of the order slice. Caller must hold c.mu.
func (c *Cache) moveToEndLocked(url string) {
	for i, u := range c.order {
		if u == url {
			c.order = append(c.order[:i], c.order[i+1:]...)
			c.order = append(c.order, url)
			break
		}
	}
}

// evictExpiredLocked removes all entries past their TTL. Caller must hold c.mu.
func (c *Cache) evictExpiredLocked() {
	now := time.Now()
	expired := make([]string, 0)
	for url, entry := range c.entries {
		if now.Sub(entry.CreatedAt) > c.ttl {
			expired = append(expired, url)
		}
	}
	for _, url := range expired {
		c.removeLocked(url)
	}
}
