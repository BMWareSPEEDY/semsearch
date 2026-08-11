package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/ashishpaliwal/semsearch/pkg/embedder"
	"github.com/ashishpaliwal/semsearch/pkg/indexer"
	"github.com/ashishpaliwal/semsearch/pkg/reranker"
)

type TestCase struct {
	Query               string `json:"query"`
	ExpectedFileSubstring string `json:"expected_file_substring"`
	ExpectedMinLine     int    `json:"expected_min_line,omitempty"`
	ExpectedMaxLine     int    `json:"expected_max_line,omitempty"`
}

var testCases = []TestCase{
	{
		Query:               "recursive gitignore and file scanner walk",
		ExpectedFileSubstring: "pkg/scanner/scanner.go",
	},
	{
		Query:               "fsnotify filesystem event watcher debouncing",
		ExpectedFileSubstring: "pkg/watcher/watcher.go",
	},
	{
		Query:               "query history logging and opened result action",
		ExpectedFileSubstring: "pkg/history/history.go",
	},
	{
		Query:               "ONNX inference session execution and pooling",
		ExpectedFileSubstring: "pkg/embedder/embedder.go",
	},
	{
		Query:               "tree sitter Go Python JavaScript AST parsing",
		ExpectedFileSubstring: "pkg/chunker/ast_chunker.go",
	},
	{
		Query:               "binary vector float32 array file storage and mmap",
		ExpectedFileSubstring: "pkg/indexer/indexer.go",
	},
	{
		Query:               "second pass cross encoder term overlap reranking",
		ExpectedFileSubstring: "pkg/reranker/reranker.go",
	},
	{
		Query:               "NewASTChunker",
		ExpectedFileSubstring: "pkg/chunker/ast_chunker.go",
	},
	{
		Query:               "NewHistoryManager",
		ExpectedFileSubstring: "pkg/history/history.go",
	},
	{
		Query:               "NewIndexWatcher",
		ExpectedFileSubstring: "pkg/watcher/watcher.go",
	},
	{
		Query:               "NewEmbedder",
		ExpectedFileSubstring: "pkg/embedder/embedder.go",
	},
	{
		Query:               "openInEditor launcher VS Code vim sublime",
		ExpectedFileSubstring: "main.go",
	},
	{
		Query:               "printExpandedContext line numbers padding",
		ExpectedFileSubstring: "main.go",
	},
	{
		Query:               "markdown header header section chunking",
		ExpectedFileSubstring: "pkg/chunker/chunker.go",
	},
	{
		Query:               "BM25 term frequency TF IDF keyword scoring",
		ExpectedFileSubstring: "pkg/indexer/indexer.go",
	},
	{
		Query:               "UpdateFileChunks incremental manifest update",
		ExpectedFileSubstring: "pkg/indexer/indexer.go",
	},
	{
		Query:               "local HTTP REST API serve endpoint handlers health search",
		ExpectedFileSubstring: "main.go",
	},
	{
		Query:               "WordPiece tokenization subword tokenizer JSON",
		ExpectedFileSubstring: "pkg/embedder/embedder.go",
	},
}

func main() {
	rootDir := "."
	store := indexer.NewIndexStore(rootDir)
	if err := store.Load(); err != nil {
		log.Fatalf("Failed to load index: %v", err)
	}

	tokenizerJSON, err := os.ReadFile("scratch/model/tokenizer.json")
	if err != nil {
		log.Fatalf("Failed to read tokenizer: %v", err)
	}

	emb, err := embedder.NewEmbedder("scratch/model/model_quantized.onnx", tokenizerJSON)
	if err != nil {
		log.Fatalf("Failed to initialize embedder: %v", err)
	}

	r := reranker.NewReranker()

	top1Correct := 0
	top3Correct := 0
	top5Correct := 0
	total := len(testCases)

	fmt.Printf("==============================================================\n")
	fmt.Printf("   `semsearch` SYSTEMATIC GROUND-TRUTH ACCURACY TEST SUITE     \n")
	fmt.Printf("   Total Evaluated Queries: %d\n", total)
	fmt.Printf("==============================================================\n\n")

	for i, tc := range testCases {
		queryVec, err := emb.EmbedBatch([]string{tc.Query}, "search_query: ")
		if err != nil {
			log.Fatalf("Embedding error on query '%s': %v", tc.Query, err)
		}

		rawCandidates := store.HybridSearch(queryVec, tc.Query, 20, 0.0, 0.6)
		results := r.Rerank(tc.Query, rawCandidates, 5)

		hitRank := -1
		for rank, res := range results {
			if containsSubstring(res.Chunk.FilePath, tc.ExpectedFileSubstring) {
				hitRank = rank + 1
				break
			}
		}

		status := "FAIL"
		if hitRank == 1 {
			top1Correct++
			top3Correct++
			top5Correct++
			status = "TOP-1 PASS"
		} else if hitRank > 1 && hitRank <= 3 {
			top3Correct++
			top5Correct++
			status = fmt.Sprintf("TOP-%d PASS", hitRank)
		} else if hitRank > 3 && hitRank <= 5 {
			top5Correct++
			status = fmt.Sprintf("TOP-%d PASS", hitRank)
		}

		topFile := "None"
		if len(results) > 0 {
			topFile = relPath(rootDir, results[0].Chunk.FilePath)
		}

		fmt.Printf("[%2d/%2d] Query: \"%s\"\n", i+1, total, tc.Query)
		fmt.Printf("       Expected: %s | Top Match: %s | Result: [%s]\n", tc.ExpectedFileSubstring, topFile, status)
	}

	fmt.Printf("\n==============================================================\n")
	fmt.Printf("                    FINAL ACCURACY METRICS                    \n")
	fmt.Printf("==============================================================\n")
	fmt.Printf("Top-1 Accuracy : %2d / %2d  (%.1f%%)\n", top1Correct, total, float64(top1Correct)/float64(total)*100)
	fmt.Printf("Top-3 Accuracy : %2d / %2d  (%.1f%%)\n", top3Correct, total, float64(top3Correct)/float64(total)*100)
	fmt.Printf("Top-5 Accuracy : %2d / %2d  (%.1f%%)\n", top5Correct, total, float64(top5Correct)/float64(total)*100)
	fmt.Printf("==============================================================\n")

	reportData := map[string]interface{}{
		"total_cases":    total,
		"top1_correct":   top1Correct,
		"top3_correct":   top3Correct,
		"top5_correct":   top5Correct,
		"top1_accuracy":  float64(top1Correct) / float64(total) * 100,
		"top3_accuracy":  float64(top3Correct) / float64(total) * 100,
		"top5_accuracy":  float64(top5Correct) / float64(total) * 100,
	}

	outData, _ := json.MarshalIndent(reportData, "", "  ")
	os.WriteFile("scratch/test_suite_results.json", outData, 0644)
}

func containsSubstring(path, target string) bool {
	return filepath.Clean(path) == filepath.Clean(target) || filepath.Base(path) == filepath.Base(target) || len(path) >= len(target) && path[len(path)-len(target):] == target
}

func relPath(base, path string) string {
	r, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return r
}
