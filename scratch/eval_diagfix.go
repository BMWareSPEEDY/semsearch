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

func diagFixable() {
	type repo struct {
		key   string
		cases []BenchCase
	}
	repos := []repo{
		{"gin", ginCases},
		{"cobra", cobraCases},
		{"mux", muxCases},
		{"clap", clapCases},
		{"hugo", hugoCases},
	}

	tokenizerJSON, err := os.ReadFile("scratch/code_minilm/tokenizer.json")
	if err != nil {
		log.Fatalf("tokenizer: %v", err)
	}
	emb, err := embedder.NewEmbedder("scratch/code_minilm/model_quantized.onnx", tokenizerJSON)
	if err != nil {
		log.Fatalf("embedder: %v", err)
	}

	type dataset struct {
		key   string
		store *indexer.IndexStore
		cases []struct {
			tc   BenchCase
			qvec []float32
		}
	}
	var datasets []dataset
	for _, rp := range repos {
		store := indexer.NewIndexStore("scratch/external_eval/" + rp.key)
		if err := store.Load(); err != nil {
			log.Fatalf("load %s: %v", rp.key, err)
		}
		var cases []struct {
			tc   BenchCase
			qvec []float32
		}
		for _, tc := range rp.cases {
			qv, err := emb.EmbedBatch([]string{tc.Query}, "search_query: ")
			if err != nil {
				continue
			}
			cases = append(cases, struct {
				tc   BenchCase
				qvec []float32
			}{tc, qv})
		}
		datasets = append(datasets, dataset{rp.key, store, cases})
	}

	hit := func(res []indexer.SearchResult, expected string) int {
		for k, rr := range res {
			if strings.HasSuffix(rr.Chunk.FilePath, expected) || strings.Contains(rr.Chunk.FilePath, expected) {
				return k + 1
			}
		}
		return -1
	}

	weight := 0.75
	shortlist := 60

	prev := reranker.NewStubReranker()
	_ = prev

	r := reranker.NewReranker()
	for _, d := range datasets {
		for _, sc := range d.cases {
			base := d.store.HybridSearch(sc.qvec, sc.tc.Query, shortlist, 0.0, float32(weight), true, false)
			// Expected file rank in raw candidates.
			expRank := hit(base, sc.tc.Expected)
			if expRank < 0 || expRank == 1 {
				continue
			}
			if expRank > 20 {
				continue
			}
			copyRanked := make([]indexer.SearchResult, len(base))
			copy(copyRanked, base)
			final := r.Rerank(sc.tc.Query, copyRanked, 5)
			if hit(final, sc.tc.Expected) > 0 {
				continue
			}
			fmt.Printf("\nQUERY: %q\n", sc.tc.Query)
			fmt.Printf("  expected: %s  (raw rank %d of %d)\n", sc.tc.Expected, expRank, len(base))
			terms := indexer.TokenizeTerms(sc.tc.Query)
			fmt.Printf("  terms: %v\n", terms)
			// find the expected chunk
			for _, rr := range base {
				if strings.HasSuffix(rr.Chunk.FilePath, sc.tc.Expected) || strings.Contains(rr.Chunk.FilePath, sc.tc.Expected) {
					sym := rr.Chunk.SymbolName
					if sym == "" {
						sym = "-"
					}
					fmt.Printf("  EXPECTED CHUNK: score=%.4f [%s] L%d-%d\n", rr.Score, sym, rr.Chunk.LineStart, rr.Chunk.LineEnd)
					fmt.Printf("    path=%s\n", rr.Chunk.FilePath)
					break
				}
			}
			fmt.Printf("  after rerank top-5:\n")
			for k, rr := range final {
				sym := rr.Chunk.SymbolName
				if sym == "" {
					sym = "-"
				}
				mark := ""
				if strings.HasSuffix(rr.Chunk.FilePath, sc.tc.Expected) || strings.Contains(rr.Chunk.FilePath, sc.tc.Expected) {
					mark = " <== EXPECTED"
				}
				fmt.Printf("    %d. %.4f [%s] %s%s\n", k+1, rr.Score, sym, rr.Chunk.FilePath, mark)
			}
		}
	}
}