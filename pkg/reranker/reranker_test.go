package reranker

import (
	"os"
	"testing"

	"github.com/ashishpaliwal/semsearch/pkg/chunker"
	"github.com/ashishpaliwal/semsearch/pkg/indexer"
)

func mkChunk(path, sym, content string, score float32) indexer.SearchResult {
	return indexer.SearchResult{
		Chunk: chunker.Chunk{
			FilePath:   path,
			SymbolName: sym,
			Content:    content,
		},
		Score: score,
	}
}

func TestRerankDedupsByFile(t *testing.T) {
	os.Setenv("MFD", "")
	defer os.Unsetenv("MFD")
	r := NewReranker()
	cands := []indexer.SearchResult{
		mkChunk("/repo/a.go", "", "content a1", 0.9),
		mkChunk("/repo/a.go", "", "content a2", 0.8),
		mkChunk("/repo/b.go", "", "content b", 0.7),
	}
	terms := indexer.TokenizeTerms("unrelated query xyz")
	if len(terms) == 0 {
		t.Fatal("expected nonzero terms")
	}
	res := r.Rerank("unrelated query xyz", cands, 5)
	files := map[string]bool{}
	for _, rr := range res {
		if files[rr.Chunk.FilePath] {
			t.Errorf("duplicate file after rerank: %s", rr.Chunk.FilePath)
		}
		files[rr.Chunk.FilePath] = true
	}
	if len(res) != 2 {
		t.Errorf("expected 2 unique files, got %d", len(res))
	}
	if res[0].Chunk.FilePath != "/repo/a.go" {
		t.Errorf("expected highest-scoring chunk of a.go first, got %s", res[0].Chunk.FilePath)
	}
}

func TestRerankSymbolBoost(t *testing.T) {
	os.Setenv("SYM", "0.10")
	defer os.Unsetenv("SYM")
	os.Setenv("PATW", "0")
	defer os.Unsetenv("PATW")
	os.Setenv("CONT", "0")
	defer os.Unsetenv("CONT")
	r := NewReranker()
	cands := []indexer.SearchResult{
		mkChunk("/repo/x.go", "", "no match here", 0.6),
		mkChunk("/repo/y.go", "matrix", "no match", 0.55),
	}
	res := r.Rerank("matrix multiplication", cands, 2)
	if res[0].Chunk.FilePath != "/repo/y.go" {
		t.Errorf("symbol match should win, got %s first", res[0].Chunk.FilePath)
	}
}

func TestRerankSmallInput(t *testing.T) {
	r := NewReranker()
	if got := r.Rerank("", nil, 5); got != nil {
		t.Errorf("empty query should return candidates unchanged, got %v", got)
	}
}