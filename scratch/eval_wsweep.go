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

func weightSweep() {
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

	eval := func(shortlist int, weight float64) (int, int) {
		r := reranker.NewReranker()
		totalHits := 0
		total := 0
		for _, d := range datasets {
			for _, sc := range d.cases {
				total++
				res := d.store.HybridSearch(sc.qvec, sc.tc.Query, shortlist, 0.0, float32(weight), true, false)
				res = r.Rerank(sc.tc.Query, res, 5)
				hit := false
				for _, rr := range res {
					if strings.HasSuffix(rr.Chunk.FilePath, sc.tc.Expected) || strings.Contains(rr.Chunk.FilePath, sc.tc.Expected) {
						hit = true
						break
					}
				}
				if hit {
					totalHits++
				}
			}
		}
		return totalHits, total
	}

	// Sweep env-driven boost params at the best hybrid config.
	for _, sl := range []int{40, 50} {
		for _, w := range []float64{0.72, 0.75, 0.78} {
			h, t := eval(sl, w)
			fmt.Printf("base w=%.2f sl=%d: %d/%d\n", w, sl, h, t)
		}
	}
}