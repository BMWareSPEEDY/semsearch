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

func diagProm() {
	type repo struct{ key string; cases []BenchCase }
	repos := []repo{
		{"gin", ginCases}, {"cobra", cobraCases}, {"mux", muxCases},
		{"clap", clapCases}, {"hugo", hugoCases},
	}
	tok, err := os.ReadFile("scratch/code_minilm/tokenizer.json")
	if err != nil {
		log.Fatal(err)
	}
	emb, err := embedder.NewEmbedder("scratch/code_minilm/model_quantized.onnx", tok)
	if err != nil {
		log.Fatal(err)
	}
	r := reranker.NewReranker()
	rank := func(res []indexer.SearchResult, expected string) int {
		for k, rr := range res {
			if strings.HasSuffix(rr.Chunk.FilePath, expected) || strings.Contains(rr.Chunk.FilePath, expected) {
				return k + 1
			}
		}
		return -1
	}
	weight, sl := 0.65, 30
	if v := os.Getenv("W"); v != "" {
		fmt.Sscanf(v, "%f", &weight)
	}
	if v := os.Getenv("SL"); v != "" {
		fmt.Sscanf(v, "%d", &sl)
	}
	for _, rp := range repos {
		store := indexer.NewIndexStore("scratch/external_eval/" + rp.key)
		if err := store.Load(); err != nil {
			log.Fatal(err)
		}
		for _, tc := range rp.cases {
			qv, err := emb.EmbedBatch([]string{tc.Query}, "search_query: ")
			if err != nil {
				continue
			}
			raw := store.HybridSearch(qv, tc.Query, sl, 0.0, float32(weight), true, false)
			rawRank := rank(raw, tc.Expected)
			final := r.Rerank(tc.Query, append([]indexer.SearchResult(nil), raw...), 5)
			finalRank := rank(final, tc.Expected)
			if finalRank > 0 && finalRank <= 5 || rawRank == -1 {
				continue
			}
			// target retrieved but not promoted
			fmt.Printf("=== %-8s raw#%d | %-55s -> %s\n", rp.key, rawRank, tc.Query, tc.Expected)
			fmt.Printf("    raw shortlist (top %d):\n", sl)
			for k, rr := range raw {
				tag := "  "
				if strings.Contains(rr.Chunk.FilePath, tc.Expected) {
					tag = "=>"
				}
				sym := rr.Chunk.SymbolName
				if sym == "" {
					sym = "-"
				}
				fmt.Printf("    %s R%2d (%.4f) %-70s [%s] L%d-%d\n", tag, k+1, rr.Score, shortPath(rr.Chunk.FilePath), sym, rr.Chunk.LineStart, rr.Chunk.LineEnd)
			}
			fmt.Printf("    FINAL:\n")
			for k, rr := range final {
				sym := rr.Chunk.SymbolName
				if sym == "" {
					sym = "-"
				}
				fmt.Printf("    F%d (%.4f) %-70s [%s] L%d-%d\n", k+1, rr.Score, shortPath(rr.Chunk.FilePath), sym, rr.Chunk.LineStart, rr.Chunk.LineEnd)
			}
			fmt.Println()
		}
	}
}

func shortPath(p string) string {
	i := strings.LastIndex(p, "external_eval/")
	if i >= 0 {
		return p[i+len("external_eval/"):]
	}
	return p
}