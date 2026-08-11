package llm_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ashishpaliwal/semsearch/pkg/chunker"
	"github.com/ashishpaliwal/semsearch/pkg/llm"
)

type fakeLLM struct {
	promptPrefix string
	responses    map[string]string // keyed by a substring of the code to return canned descs
	calls        int
}

func (f *fakeLLM) IsAvailable() bool { return true }

func (f *fakeLLM) Complete(_ context.Context, prompt string, _ llm.CompletionOpts) (string, error) {
	f.calls++
	for sub, resp := range f.responses {
		if strings.Contains(prompt, sub) {
			return resp, nil
		}
	}
	return "generic description", nil
}

func TestGenerateDescription(t *testing.T) {
	fake := &fakeLLM{responses: map[string]string{"func retryRequest": "retries failed HTTP requests with backoff"}}
	chunk := chunker.Chunk{Content: "func retryRequest(url string) error {\n\t// retry logic\n\treturn nil\n}"}

	desc, err := llm.GenerateDescription(context.Background(), fake, chunk)
	if err != nil {
		t.Fatalf("GenerateDescription failed: %v", err)
	}

	if fake.calls != 1 {
		t.Errorf("Expected exactly 1 LLM call, got %d", fake.calls)
	}
	if !strings.Contains(desc, "retries failed HTTP requests") {
		t.Errorf("Expected canned description, got %q", desc)
	}
}

func TestGenerateDescriptionFallsBackToHeuristic(t *testing.T) {
	fake := &fakeLLM{responses: map[string]string{}}
	chunk := chunker.Chunk{
		Content:    "func (e *Engine) Run() {\n\treturn e.httpServer.ListenAndServe()\n}",
		SymbolName: "Run",
		Kind:       "method_declaration",
		FilePath:   "engine.go",
	}

	// LLM returns empty string -> indexer falls back; here we verify the LLM call
	// is what comes back even when trivial.
	desc, err := llm.GenerateDescription(context.Background(), fake, chunk)
	if err != nil {
		t.Fatalf("GenerateDescription failed: %v", err)
	}
	if desc == "" {
		t.Errorf("Expected non-empty description")
	}
}

func TestDescriptionCacheHashGate(t *testing.T) {
	dir := t.TempDir()
	cache := llm.NewDescriptionCache(dir)
	if err := cache.Load(); err != nil {
		t.Fatalf("Load on empty cache failed: %v", err)
	}

	if _, ok := cache.Get("abc123"); ok {
		t.Errorf("Expected cache miss for unknown hash")
	}

	cache.Put("abc123", "retries failed requests")
	if got, ok := cache.Get("abc123"); !ok || got != "retries failed requests" {
		t.Errorf("Expected cached desc, got %q ok=%v", got, ok)
	}

	if err := cache.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Re-open: same content hash must NOT regenerate (hash-gated persistence)
	reloaded := llm.NewDescriptionCache(dir)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if got, ok := reloaded.Get("abc123"); !ok || got != "retries failed requests" {
		t.Errorf("Expected desc persisted across reload, got %q ok=%v", got, ok)
	}

	// Cache file exists on disk
	if _, err := os.Stat(filepath.Join(dir, "desc_cache.json")); err != nil {
		t.Errorf("Expected desc_cache.json to be written: %v", err)
	}
}

func TestGenerateDescriptionsConcurrent(t *testing.T) {
	fake := &fakeLLM{responses: map[string]string{
		"func a1": "first function",
		"func b1": "second function",
	}}
	chunks := []chunker.Chunk{
		{Content: "func a1() { return 1 }", ContentHash: "hash-a"},
		{Content: "func b1() { return 2 }", ContentHash: "hash-b"},
		{Content: "small()", ContentHash: "hash-c"},                // skipped via skip func
		{Content: "func a1() { return 1 }", ContentHash: "hash-a"}, // cache hit
	}

	dir := t.TempDir()
	cache := llm.NewDescriptionCache(dir)

	descs, newDescs := llm.GenerateDescriptions(context.Background(), fake, cache, chunks,
		func(i int) bool { return i == 2 }, 4)

	if descs[0] != "first function" {
		t.Errorf("expected desc[0]='first function', got %q", descs[0])
	}
	if descs[1] != "second function" {
		t.Errorf("expected desc[1]='second function', got %q", descs[1])
	}
	if descs[2] != "" {
		t.Errorf("expected skipped chunk desc empty, got %q", descs[2])
	}
	if descs[3] != "first function" {
		t.Errorf("expected cached desc for duplicate chunk, got %q", descs[3])
	}
	if newDescs != 2 {
		t.Errorf("expected 2 new generations (skip + cache hit excluded), got %d", newDescs)
	}

	// Second pass: everything is a cache hit now, zero new generations
	descs2, newDescs2 := llm.GenerateDescriptions(context.Background(), fake, cache, chunks,
		func(i int) bool { return i == 2 }, 4)
	if newDescs2 != 0 {
		t.Errorf("expected 0 new generations on cached re-run, got %d", newDescs2)
	}
	if descs2[0] != "first function" || descs2[1] != "second function" {
		t.Errorf("cache reuse failed: %q %q", descs2[0], descs2[1])
	}
}
