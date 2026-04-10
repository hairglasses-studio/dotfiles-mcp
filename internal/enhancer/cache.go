package enhancer

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// PromptCache is an in-memory cache for LLM-improved prompts.
// SHA-256 keyed on prompt+options, 100 entries max, 10min TTL.
type PromptCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	maxSize int
	ttl     time.Duration
}

type cacheEntry struct {
	result    *ImproveResult
	createdAt time.Time
}

// NewPromptCache creates a cache with default settings.
func NewPromptCache() *PromptCache {
	return &PromptCache{
		entries: make(map[string]*cacheEntry),
		maxSize: 100,
		ttl:     10 * time.Minute,
	}
}

// Get retrieves a cached result. Returns nil if miss or expired.
func (c *PromptCache) Get(prompt string, opts ImproveOptions) *ImproveResult {
	key := c.key(prompt, opts)

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return nil
	}
	if time.Since(entry.createdAt) > c.ttl {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil
	}
	return entry.result
}

// Put stores a result in the cache. Evicts oldest entry if at capacity.
func (c *PromptCache) Put(prompt string, opts ImproveOptions, result *ImproveResult) {
	key := c.key(prompt, opts)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if at capacity
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[key] = &cacheEntry{
		result:    result,
		createdAt: time.Now(),
	}
}

func (c *PromptCache) key(prompt string, opts ImproveOptions) string {
	h := sha256.New()
	h.Write([]byte(prompt))
	if opts.ThinkingEnabled {
		h.Write([]byte(":thinking"))
	}
	if opts.TaskType != "" {
		h.Write([]byte(":" + string(opts.TaskType)))
	}
	if opts.Feedback != "" {
		h.Write([]byte(":" + opts.Feedback))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c *PromptCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.createdAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.createdAt
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}
