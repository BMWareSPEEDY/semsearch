package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ashishpaliwal/semsearch/pkg/embedder"
	"github.com/ashishpaliwal/semsearch/pkg/indexer"
	"github.com/ashishpaliwal/semsearch/pkg/reranker"
)

type BenchCase struct {
	Query    string
	Category string
	Expected string
}

func diagRepo(key string, cases []BenchCase) {
	store := indexer.NewIndexStore("scratch/external_eval/" + key)
	if err := store.Load(); err != nil {
		log.Fatalf("load %s: %v", key, err)
	}
	tokenizerJSON, err := os.ReadFile("scratch/code_minilm/tokenizer.json")
	if err != nil {
		log.Fatalf("tokenizer: %v", err)
	}
	emb, err := embedder.NewEmbedder("scratch/code_minilm/model_quantized.onnx", tokenizerJSON)
	if err != nil {
		log.Fatalf("embedder: %v", err)
	}
	r := reranker.NewReranker()

	fmt.Printf("\n========== %s (%d chunks) ==========\n", key, store.Manifest.TotalChunks)
	for _, tc := range cases {
		qv, err := emb.EmbedBatch([]string{tc.Query}, "search_query: ")
		if err != nil {
			continue
		}
		raw := store.HybridSearch(qv, tc.Query, 200, 0.0, 0.75, true, false)
		res := r.Rerank(tc.Query, raw, 5)
		hit := -1
		for k, rr := range res {
			if strings.HasSuffix(rr.Chunk.FilePath, tc.Expected) || strings.Contains(rr.Chunk.FilePath, tc.Expected) {
				hit = k + 1
				break
			}
		}
		if hit > 0 {
			continue
		}
		fmt.Printf("\nFAIL %q\n  expected: %s\n", tc.Query, tc.Expected)
		expRank := -1
		for k, rr := range raw {
			if strings.HasSuffix(rr.Chunk.FilePath, tc.Expected) || strings.Contains(rr.Chunk.FilePath, tc.Expected) {
				expRank = k + 1
				break
			}
		}
		fmt.Printf("  expected rank in full list: %d\n", expRank)
		for k, rr := range raw[:min(5, len(raw))] {
			sym := rr.Chunk.SymbolName
			if sym == "" {
				sym = "-"
			}
			fmt.Printf("  raw%d (%6.4f) %s [%s] L%d-%d\n", k+1, rr.Score, rr.Chunk.FilePath, sym, rr.Chunk.LineStart, rr.Chunk.LineEnd)
		}
		if expRank != -1 && expRank <= 20 {
			fmt.Printf("  -- in top-20, after rerank --\n")
			for k, rr := range res {
				sym := rr.Chunk.SymbolName
				if sym == "" {
					sym = "-"
				}
				fmt.Printf("  fe%d (%6.4f) %s [%s] L%d-%d\n", k+1, rr.Score, rr.Chunk.FilePath, sym, rr.Chunk.LineStart, rr.Chunk.LineEnd)
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}