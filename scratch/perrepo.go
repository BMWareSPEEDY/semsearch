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
	for _, rp := range []string{"gin", "cobra", "mux", "clap", "hugo"} {
		store := indexer.NewIndexStore("scratch/external_eval/" + rp)
		if err := store.Load(); err != nil {
			log.Fatal(err)
		}
		hit := 0
		for _, tc := range repos[rp] {
			qv, err := emb.EmbedBatch([]string{tc.Query}, "search_query: ")
			if err != nil {
				continue
			}
			raw := store.HybridSearch(qv, tc.Query, 50, 0.0, 0.75, true, false)
			cp := make([]indexer.SearchResult, len(raw))
			copy(cp, raw)
			final := reranker.NewReranker().Rerank(tc.Query, cp, 5)
			for _, rr := range final {
				if strings.HasSuffix(rr.Chunk.FilePath, tc.Expected) || strings.Contains(rr.Chunk.FilePath, tc.Expected) {
					hit++
					break
				}
			}
		}
		fmt.Printf("%-6s %d/%d\n", rp, hit, len(repos[rp]))
	}
}
