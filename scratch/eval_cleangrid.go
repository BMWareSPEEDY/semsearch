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

func cleanGrid() {
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
	r := reranker.NewReranker()

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

	eval := func(shortlist int, useRerank bool, weight float64) (int, int) {
		totalHits := 0
		total := 0
		for _, d := range datasets {
			for _, sc := range d.cases {
				total++
				res := d.store.HybridSearch(sc.qvec, sc.tc.Query, shortlist, 0.0, float32(weight), true, false)
				if useRerank {
					res = r.Rerank(sc.tc.Query, res, 5)
				} else if len(res) > 5 {
					res = res[:5]
				}
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

	weights := []float64{0.60, 0.65, 0.70, 0.72, 0.75, 0.78, 0.80, 0.85, 0.90, 0.95, 1.00}
	shortlists := []int{15, 20, 30, 40, 50}

	bestRaw, bestRR := 0, 0
	var bestRawCfg, bestRRCfg string
	for _, w := range weights {
		for _, sl := range shortlists {
			rh, t := eval(sl, false, w)
			rrh, _ := eval(sl, true, w)
			fmt.Printf("w=%.2f sl=%3d raw=%3d/%d (%.1f%%) rerank=%3d/%d (%.1f%%)\n", w, sl, rh, t, float64(rh)/float64(t)*100, rrh, t, float64(rrh)/float64(t)*100)
			if rh > bestRaw {
				bestRaw, bestRawCfg = rh, fmt.Sprintf("w=%.2f sl=%d", w, sl)
			}
			if rrh > bestRR {
				bestRR, bestRRCfg = rrh, fmt.Sprintf("w=%.2f sl=%d", w, sl)
			}
		}
	}
	fmt.Printf("BEST raw=%d cfg=%s\n", bestRaw, bestRawCfg)
	fmt.Printf("BEST rerank=%d cfg=%s\n", bestRR, bestRRCfg)
}