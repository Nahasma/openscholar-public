package web

import (
	"testing"
	"time"
)

func TestCache_SetGet(t *testing.T) {
	c := NewCache(1024*1024, 15*time.Minute)

	c.Set("https://example.com", &CacheEntry{
		Content:     "hello world",
		ContentType: "text/html",
		Bytes:       11,
		Code:        200,
	})

	entry := c.Get("https://example.com")
	if entry == nil {
		t.Fatal("expected cache hit")
	}
	if entry.Content != "hello world" {
		t.Errorf("content = %q, want %q", entry.Content, "hello world")
	}
}

func TestCache_Miss(t *testing.T) {
	c := NewCache(1024*1024, 15*time.Minute)

	if entry := c.Get("https://nonexistent.com"); entry != nil {
		t.Error("expected cache miss")
	}
}

func TestCache_TTLExpiry(t *testing.T) {
	c := NewCache(1024*1024, 1*time.Millisecond)

	c.Set("https://example.com", &CacheEntry{Content: "data", Bytes: 4})
	time.Sleep(5 * time.Millisecond)

	if entry := c.Get("https://example.com"); entry != nil {
		t.Error("expected entry to be expired")
	}
}

func TestCache_Eviction(t *testing.T) {
	// Max 20 bytes
	c := NewCache(20, 15*time.Minute)

	c.Set("https://a.com", &CacheEntry{Content: "1234567890", Bytes: 10, Size: 10})
	c.Set("https://b.com", &CacheEntry{Content: "1234567890", Bytes: 10, Size: 10})

	// Both should fit exactly (10+10 = 20)
	if c.Get("https://a.com") == nil {
		t.Error("expected a.com to be cached")
	}
	if c.Get("https://b.com") == nil {
		t.Error("expected b.com to be cached")
	}

	// Adding a third should evict the oldest (a.com)
	c.Set("https://c.com", &CacheEntry{Content: "12345", Bytes: 5, Size: 5})
	if c.Get("https://a.com") != nil {
		t.Error("expected a.com to be evicted")
	}
	if c.Get("https://c.com") == nil {
		t.Error("expected c.com to be cached")
	}
}

func TestCache_Clear(t *testing.T) {
	c := NewCache(1024*1024, 15*time.Minute)
	c.Set("https://example.com", &CacheEntry{Content: "data"})
	c.Clear()

	if entry := c.Get("https://example.com"); entry != nil {
		t.Error("expected cache to be empty after clear")
	}
}
