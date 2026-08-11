package chunker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ashishpaliwal/semsearch/pkg/chunker"
	"github.com/ashishpaliwal/semsearch/pkg/scanner"
)

func TestFileScannerAndChunker(t *testing.T) {
	tempDir := t.TempDir()

	// Setup mock workspace
	os.WriteFile(filepath.Join(tempDir, ".gitignore"), []byte("ignored_dir/\n*.tmp\n"), 0644)
	os.MkdirAll(filepath.Join(tempDir, "ignored_dir"), 0755)
	os.WriteFile(filepath.Join(tempDir, "ignored_dir", "secret.txt"), []byte("secret"), 0644)

	os.MkdirAll(filepath.Join(tempDir, "src"), 0755)
	os.WriteFile(filepath.Join(tempDir, "src", "main.go"), []byte("package main\n\nfunc main() {\n\tprintln(\"hello world\")\n}"), 0644)
	os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("# Title\nWelcome to project\n\n## Installation\nRun go build"), 0644)

	// Test Scanner
	s, err := scanner.NewFileScanner(tempDir)
	if err != nil {
		t.Fatalf("Failed to create scanner: %v", err)
	}

	files, err := s.Scan()
	if err != nil {
		t.Fatalf("Scanner failed: %v", err)
	}

	if len(files) != 2 {
		t.Errorf("Expected 2 files scanned, got %d: %v", len(files), files)
	}

	// Test Chunker
	c := chunker.NewDefaultChunker()
	mdChunks, err := c.ChunkFile(filepath.Join(tempDir, "README.md"))
	if err != nil {
		t.Fatalf("Chunker failed on markdown: %v", err)
	}

	if len(mdChunks) != 2 {
		t.Errorf("Expected 2 markdown section chunks, got %d", len(mdChunks))
	}

	goChunks, err := c.ChunkFile(filepath.Join(tempDir, "src", "main.go"))
	if err != nil {
		t.Fatalf("Chunker failed on go file: %v", err)
	}

	if len(goChunks) == 0 {
		t.Errorf("Expected at least 1 code chunk, got 0")
	}
}
