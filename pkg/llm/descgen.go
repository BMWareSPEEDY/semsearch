package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ashishpaliwal/semsearch/pkg/chunker"
)

type ChunkDescription struct {
	ChunkID     string
	Description string // one sentence, plain English
}

func GenerateDescription(ctx context.Context, llm LLMClient, chunk chunker.Chunk) (string, error) {
	prompt := fmt.Sprintf(`Describe what this code does in one plain sentence, using the vocabulary a developer would use when searching for it. No code syntax, no variable names unless essential. Focus on purpose and behavior, not implementation detail.

Code:
%s

One-sentence description:`, chunk.Content)

	resp, err := llm.Complete(ctx, prompt, CompletionOpts{MaxTokens: 60, Temperature: 0})
	if err != nil {
		return "", err
	}

	resp = strings.TrimSpace(resp)
	resp = strings.Trim(resp, `"`)
	resp = strings.TrimPrefix(resp, "Description:")
	resp = strings.TrimSpace(resp)
	return resp, nil
}

// DescriptionCache persists generated descriptions keyed by chunk content hash,
// so re-indexing unchanged chunks never regenerates them (hash-gated like watch mode).
type DescriptionCache struct {
	mu      sync.Mutex
	path    string
	entries map[string]string
}

func NewDescriptionCache(indexDir string) *DescriptionCache {
	return &DescriptionCache{
		path:    filepath.Join(indexDir, "desc_cache.json"),
		entries: make(map[string]string),
	}
}

func (c *DescriptionCache) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(c.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &c.entries)
}

func (c *DescriptionCache) Get(contentHash string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	desc, ok := c.entries[contentHash]
	return desc, ok
}

func (c *DescriptionCache) Put(contentHash, desc string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[contentHash] = desc
}

func (c *DescriptionCache) Save() error {
	c.mu.Lock()
	data, err := json.MarshalIndent(c.entries, "", "  ")
	c.mu.Unlock()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(c.path), 0755); err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0644)
}
