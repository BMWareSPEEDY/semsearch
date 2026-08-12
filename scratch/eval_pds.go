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

func pdSweep() {
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

	total, both, promoted, demoted, notSeen := 0, 0, 0, 0, 0
	for _, d := range datasets {
		for _, sc := range d.cases {
			total++
			raw := d.store.HybridSearch(sc.qvec, sc.tc.Query, shortlist, 0.0, float32(weight), true, false)
			rawRank := rank(raw, sc.tc.Expected)
			cp := make([]indexer.SearchResult, len(raw))
			copy(cp, raw)
			final := reranker.NewReranker().Rerank(sc.tc.Query, cp, 5)
			finalRank := rank(final, sc.tc.Expected)
			if rawRank > 0 && rawRank <= 5 && finalRank > 0 && finalRank <= 5 {
				both++
			} else if rawRank > 5 && finalRank > 0 && finalRank <= 5 {
				promoted++
			} else if rawRank > 0 && rawRank <= 5 && finalRank == -1 {
				demoted++
			} else {
				notSeen++
			}
		}
	}
	fmt.Printf("total=%d both=%d promoted=%d demoted=%d notSeen=%d final=%d\n",
		total, both, promoted, demoted, notSeen, both+promoted)
}