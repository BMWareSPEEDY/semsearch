package chunker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestTreeSitterChunker(t *testing.T) {
	c := NewTreeSitterChunker()

	// Find all test files
	testDir := "testdata"
	entries, err := os.ReadDir(testDir)
	if err != nil {
		t.Fatalf("Failed to read testdata: %v", err)
	}

	type testResult struct {
		FilePath    string
		Chunks      []Chunk
		ParseFailed bool
	}

	var results []testResult
	totalChunks := 0
	parseFailures := 0
	byLang := make(map[string]int)
	totalTokens := 0
	chunkCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filePath := filepath.Join(testDir, entry.Name())
		chunks, err := c.ChunkFile(filePath)
		if err != nil {
			t.Logf("Error chunking %s: %v", filePath, err)
			parseFailures++
			results = append(results, testResult{
				FilePath:    filePath,
				ParseFailed: true,
			})
			continue
		}

		ext := filepath.Ext(filePath)
		lang := ext
		if lang == "" {
			lang = "unknown"
		} else {
			lang = lang[1:] // remove leading dot
		}

		byLang[lang] += len(chunks)
		totalChunks += len(chunks)

		for _, chunk := range chunks {
			// Rough token estimate: ~4 chars per token
			tokenEstimate := len(chunk.Content) / 4
			totalTokens += tokenEstimate
			chunkCount++
		}

		results = append(results, testResult{
			FilePath: filePath,
			Chunks:   chunks,
		})
	}

	// Print results
	for _, r := range results {
		t.Logf("\n=== %s ===", r.FilePath)
		if r.ParseFailed {
			t.Logf("  PARSE FAILED")
			continue
		}
		t.Logf("  Chunks: %d", len(r.Chunks))
		for _, chunk := range r.Chunks {
			t.Logf("  [%s] %s (lines %d-%d): %s", chunk.Kind, chunk.SymbolName, chunk.LineStart, chunk.LineEnd, chunk.Preview)
		}
	}

	avgTokens := 0
	if chunkCount > 0 {
		avgTokens = totalTokens / chunkCount
	}

	output := map[string]interface{}{
		"chunksTotal":    totalChunks,
		"byLang":         byLang,
		"avgChunkTokens": avgTokens,
		"parseFailures":  parseFailures,
	}

	jsonOutput, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(jsonOutput))
}