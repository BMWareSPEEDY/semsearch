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

type sweepCase struct {
	tc   BenchCase
	qvec []float32
}

type BenchCase struct {
	Query    string
	Category string
	Expected string
}

func sweepFast() {
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

	// Precompute per-repo data: store + query vectors.
	type repoData struct {
		key   string
		index int
		store *indexer.IndexStore
		cases []sweepCase
	}
	var datasets []repoData
	for _, rp := range repos {
		store := indexer.NewIndexStore("scratch/external_eval/" + rp.key)
		if err := store.Load(); err != nil {
			log.Fatalf("load %s: %v", rp.key, err)
		}
		var cases []sweepCase
		for _, tc := range rp.cases {
			qv, err := emb.EmbedBatch([]string{tc.Query}, "search_query: ")
			if err != nil {
				continue
			}
			cases = append(cases, sweepCase{tc, qv})
		}
		datasets = append(datasets, repoData{key: rp.key, store: store, cases: cases})
	}

	// For a given weight, fullResult returns the fully-sorted (top 200) list for
	// a case. Cached so shortlist truncation reuses the expensive scoring pass.
	type cacheKey struct{ r, c int; w float64 }
	fullCache := map[cacheKey][]indexer.SearchResult{}

	fullResult := func(d *repoData, ci int, weight float64) []indexer.SearchResult {
		k := cacheKey{d.index, ci, weight}
		if res, ok := fullCache[k]; ok {
			return res
		}
		sc := d.cases[ci]
		res := d.store.HybridSearch(sc.qvec, sc.tc.Query, 200, 0.0, float32(weight), true, false)
		fullCache[k] = res
		return res
	}
	// Track dataset index in repoData.
	for i := range datasets {
		datasets[i].index = i
	}

	eval := func(shortlist int, useRerank bool, weight float64) (int, int) {
		totalHits := 0
		total := 0
		for di, d := range datasets {
			for ci := range d.cases {
				total++
				all := fullResult(&d, ci, weight)
				cp := make([]indexer.SearchResult, len(all))
				copy(cp, all)
				res := cp
				if len(res) > shortlist {
					res = res[:shortlist]
				}
				if useRerank {
					res = r.Rerank(d.cases[ci].tc.Query, res, 5)
				} else if len(res) > 5 {
					res = res[:5]
				}
				hit := false
				for _, rr := range res {
					if strings.HasSuffix(rr.Chunk.FilePath, d.cases[ci].tc.Expected) || strings.Contains(rr.Chunk.FilePath, d.cases[ci].tc.Expected) {
						hit = true
						break
					}
				}
				if hit {
					totalHits++
				}
				_ = di
			}
		}
		return totalHits, total
	}

	weights := []float64{0.7, 0.72, 0.74, 0.75, 0.76, 0.78, 0.8}
	shortlists := []int{40, 50, 60, 75}
	best := 0
	bestCfg := ""
	for _, rr := range []bool{false, true} {
		for _, w := range weights {
			for _, sl := range shortlists {
				h, t := eval(sl, rr, w)
				pct := float64(h) / float64(t) * 100
				fmt.Printf("w=%.2f shortlist=%3d %-6s: %3d/%d (%.1f%%)\n", w, sl, map[bool]string{true: "rerank", false: "raw"}[rr], h, t, pct)
				if h > best {
					best = h
					bestCfg = fmt.Sprintf("w=%.2f shortlist=%d rerank=%v", w, sl, rr)
				}
			}
		}
	}
	fmt.Printf("BEST: %d/173 cfg=%s\n", best, bestCfg)
}