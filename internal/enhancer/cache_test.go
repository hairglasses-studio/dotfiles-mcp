package enhancer

import (
	"testing"
	"time"
)

func TestPromptCache_MissOnEmpty(t *testing.T) {
	c := NewPromptCache()
	result := c.Get("test prompt", ImproveOptions{})
	if result != nil {
		t.Error("expected nil on cache miss")
	}
}

func TestPromptCache_HitAfterPut(t *testing.T) {
	c := NewPromptCache()
	opts := ImproveOptions{ThinkingEnabled: true}
	expected := &ImproveResult{Enhanced: "improved", TaskType: "code"}

	c.Put("test prompt", opts, expected)
	result := c.Get("test prompt", opts)

	if result == nil {
		t.Fatal("expected cache hit")
	}
	if result.Enhanced != "improved" {
		t.Errorf("expected 'improved', got %q", result.Enhanced)
	}
}

func TestPromptCache_DifferentOptsMiss(t *testing.T) {
	c := NewPromptCache()
	expected := &ImproveResult{Enhanced: "improved"}

	c.Put("test prompt", ImproveOptions{ThinkingEnabled: true}, expected)
	result := c.Get("test prompt", ImproveOptions{ThinkingEnabled: false})

	if result != nil {
		t.Error("expected miss for different options")
	}
}

func TestPromptCache_TTLExpiration(t *testing.T) {
	c := &PromptCache{
		entries: make(map[string]*cacheEntry),
		maxSize: 100,
		ttl:     10 * time.Millisecond,
	}

	opts := ImproveOptions{}
	c.Put("test", opts, &ImproveResult{Enhanced: "improved"})

	if c.Get("test", opts) == nil {
		t.Error("expected hit before TTL")
	}

	time.Sleep(15 * time.Millisecond)

	if c.Get("test", opts) != nil {
		t.Error("expected miss after TTL expiration")
	}
}

func TestPromptCache_EvictsOldestAtCapacity(t *testing.T) {
	c := &PromptCache{
		entries: make(map[string]*cacheEntry),
		maxSize: 2,
		ttl:     10 * time.Minute,
	}

	opts := ImproveOptions{}
	c.Put("first", opts, &ImproveResult{Enhanced: "1"})
	time.Sleep(time.Millisecond) // ensure different timestamps
	c.Put("second", opts, &ImproveResult{Enhanced: "2"})
	time.Sleep(time.Millisecond)
	c.Put("third", opts, &ImproveResult{Enhanced: "3"}) // evicts "first"

	if c.Get("first", opts) != nil {
		t.Error("expected 'first' to be evicted")
	}
	if c.Get("second", opts) == nil {
		t.Error("expected 'second' to still be cached")
	}
	if c.Get("third", opts) == nil {
		t.Error("expected 'third' to be cached")
	}
}

func TestPromptCache_FeedbackAffectsKey(t *testing.T) {
	c := NewPromptCache()
	c.Put("prompt", ImproveOptions{Feedback: "be concise"}, &ImproveResult{Enhanced: "concise"})
	c.Put("prompt", ImproveOptions{Feedback: "be verbose"}, &ImproveResult{Enhanced: "verbose"})

	r1 := c.Get("prompt", ImproveOptions{Feedback: "be concise"})
	r2 := c.Get("prompt", ImproveOptions{Feedback: "be verbose"})

	if r1 == nil || r1.Enhanced != "concise" {
		t.Error("expected hit for 'be concise'")
	}
	if r2 == nil || r2.Enhanced != "verbose" {
		t.Error("expected hit for 'be verbose'")
	}
}
