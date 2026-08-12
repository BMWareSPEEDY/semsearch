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

func main() {
	repos := map[string][]BenchCase{"gin": ginCases, "cobra": cobraCases, "mux": muxCases, "clap": clapCases, "hugo": hugoCases}
	tok, err := os.ReadFile("scratch/code_minilm/tokenizer.json")
	if err != nil {
		log.Fatal(err)
	}
	emb, err := embedder.NewEmbedder("scratch/code_minilm/model_quantized.onnx", tok)
	if err != nil {
		log.Fatal(err)
	}
	weight, shortlist := 0.75, 50
	for _, rp := range []string{"gin", "cobra", "mux", "clap", "hugo"} {
		store := indexer.NewIndexStore("scratch/external_eval/" + rp)
		if err := store.Load(); err != nil {
			log.Fatal(err)
		}
		for _, tc := range repos[rp] {
			qv, err := emb.EmbedBatch([]string{tc.Query}, "search_query: ")
			if err != nil {
				continue
			}
			raw := store.HybridSearch(qv, tc.Query, shortlist, 0.0, float32(weight), true, false)
			rk := rankIdx(raw, tc.Expected)
			cp := make([]indexer.SearchResult, len(raw))
			copy(cp, raw)
			final := reranker.NewReranker().Rerank(tc.Query, cp, 5)
			fk := rankIdx(final, tc.Expected)
			if rk >= 1 && rk <= 5 && fk == -1 {
				fmt.Printf("DEMOTED %-28s expected=%-30s raw#%d\n", rp+"/"+tc.Query, tc.Expected, rk)
				fmt.Printf("   RAW top5: %s\n", fmtTop(raw[:5]))
				fmt.Printf("   FINAL top5: %s\n", fmtTop(final))
				fmt.Println()
			}
		}
	}
}

func rankIdx(res []indexer.SearchResult, expected string) int {
	for k, rr := range res {
		if strings.HasSuffix(rr.Chunk.FilePath, expected) || strings.Contains(rr.Chunk.FilePath, expected) {
			return k + 1
		}
	}
	return -1
}

func fmtTop(res []indexer.SearchResult) string {
	parts := make([]string, 0, len(res))
	for _, rr := range res {
		parts = append(parts, fmt.Sprintf("[%.3f %s]", rr.Score, rr.Chunk.FilePath))
	}
	return strings.Join(parts, " ")
}
