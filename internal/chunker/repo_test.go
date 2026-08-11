package chunker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChunkSemsearchRepo(t *testing.T) {
	c := NewTreeSitterChunker()

	repoPath := "/Users/ashishpaliwal/CLI_apps/semsearch"
	var goFiles []string

	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".go" {
			// Skip test files and vendor
			if !filepath.HasPrefix(path, repoPath+"/.git") &&
				!filepath.HasPrefix(path, repoPath+"/scratch") &&
				!filepath.HasPrefix(path, repoPath+"/.semsearch") {
				goFiles = append(goFiles, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	totalChunks := 0
	byLang := make(map[string]int)
	totalTokens := 0
	chunkCount := 0

	for _, file := range goFiles {
		chunks, err := c.ChunkFile(file)
		if err != nil {
			t.Logf("Error chunking %s: %v", file, err)
			continue
		}

		ext := filepath.Ext(file)
		lang := ext
		if lang == "" {
			lang = "unknown"
		} else {
			lang = lang[1:]
		}

		byLang[lang] += len(chunks)
		totalChunks += len(chunks)

		for _, chunk := range chunks {
			tokenEstimate := len(chunk.Content) / 4
			totalTokens += tokenEstimate
			chunkCount++
		}

		t.Logf("%s: %d chunks", file, len(chunks))
	}

	avgTokens := 0
	if chunkCount > 0 {
		avgTokens = totalTokens / chunkCount
	}

	t.Logf("Total chunks: %d", totalChunks)
	t.Logf("By language: %v", byLang)
	t.Logf("Avg tokens per chunk: %d", avgTokens)
}