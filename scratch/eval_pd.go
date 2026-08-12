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

func promoteDemote() {
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

	rank := func(res []indexer.SearchResult, expected string) int {
		for k, rr := range res {
			if strings.HasSuffix(rr.Chunk.FilePath, expected) || strings.Contains(rr.Chunk.FilePath, expected) {
				return k + 1
			}
		}
		return -1
	}

	weight := 0.75
	shortlist := 50
	r := reranker.NewReranker()

	demoted := 0
	promoted := 0
	both5 := 0
	notSeen := 0
	fmt.Printf("=== cases where raw top-5 was correct but rerank lost it ===\n")
	for _, d := range datasets {
		for _, sc := range d.cases {
			raw := d.store.HybridSearch(sc.qvec, sc.tc.Query, shortlist, 0.0, float32(weight), true, false)
			rawRank := rank(raw, sc.tc.Expected)
			cp := make([]indexer.SearchResult, len(raw))
			copy(cp, raw)
			final := r.Rerank(sc.tc.Query, cp, 5)
			finalRank := rank(final, sc.tc.Expected)
			if rawRank > 0 && rawRank <= 5 && finalRank == -1 {
				demoted++
				fmt.Printf("  DEMOTED: %q -> %s (raw #%d)\n", sc.tc.Query, sc.tc.Expected, rawRank)
			} else if rawRank > 5 && rawRank <= shortlist && finalRank > 0 && finalRank <= 5 {
				promoted++
				fmt.Printf("  PROMOTED: %q -> %s (raw #%d -> final #%d)\n", sc.tc.Query, sc.tc.Expected, rawRank, finalRank)
			} else if rawRank > 0 && rawRank <= 5 && finalRank > 0 && finalRank <= 5 {
				both5++
			} else if rawRank > shortlist || rawRank == -1 {
				notSeen++
			}
		}
	}
	fmt.Printf("\nsummary: raw5_correct_stays=%d promoted_into_top5=%d demoted_out_of_top5=%d expected_not_in_shortlist=%d\n",
		both5, promoted, demoted, notSeen)
}