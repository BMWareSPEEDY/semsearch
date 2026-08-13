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

func diagFail() {
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
	buckets := map[string]int{"not_in_shortlist": 0, "in_shortlist_no_promote": 0, "demoted": 0}
	fmt.Printf("== DIAG at w=%.2f sl=%d ==\n", weight, sl)
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
			if finalRank > 0 && finalRank <= 5 {
				continue
			}
			kind := "not_in_shortlist"
			if rawRank > 0 && rawRank <= 5 {
				kind = "demoted"
			} else if rawRank > 0 {
				kind = "in_shortlist_no_promote"
			}
			buckets[kind]++
			fmt.Printf("[%s] %-8s raw#%2d final#%2d | %-58s -> %s\n", kind, rp.key, rawRank, finalRank, tc.Query, tc.Expected)
			for k, rr := range final {
				fmt.Printf("      F%d(%.4f) %s [%s] L%d-%d\n", k+1, rr.Score, rr.Chunk.FilePath, rr.Chunk.SymbolName, rr.Chunk.LineStart, rr.Chunk.LineEnd)
			}
			fmt.Println()
		}
	}
	fmt.Printf("== buckets: %v ==\n", buckets)
}