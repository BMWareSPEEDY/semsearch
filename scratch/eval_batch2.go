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

func batchSweep2() {
	type repo struct {
		key   string
		cases []BenchCase
	}
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

	type ds struct {
		store *indexer.IndexStore
		cases []struct {
			tc   BenchCase
			qvec []float32
		}
	}
	var datasets []ds
	for _, rp := range repos {
		store := indexer.NewIndexStore("scratch/external_eval/" + rp.key)
		if err := store.Load(); err != nil {
			log.Fatal(err)
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
		datasets = append(datasets, ds{store, cases})
	}

	type cfg struct {
		sym, path, cont float64
		seg             int
	}
	var cfgs []cfg
	for _, seg := range []int{0, 2, 3} {
		for _, sym := range []float64{0.10, 0.15, 0.20, 0.30, 0.40} {
			for _, path := range []float64{0.0, 0.05, 0.10} {
				for _, cont := range []float64{0.01, 0.02, 0.04} {
					cfgs = append(cfgs, cfg{sym, path, cont, seg})
				}
			}
		}
	}

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

	type res struct{ both, prom, dem int }
	m := map[cfg]res{}
	for _, c := range cfgs {
		m[c] = res{}
	}
	for _, d := range datasets {
		for _, sc := range d.cases {
			raw := d.store.HybridSearch(sc.qvec, sc.tc.Query, sl, 0.0, float32(weight), true, false)
			rawRank := rank(raw, sc.tc.Expected)
			for _, c := range cfgs {
				os.Setenv("SYM", fmt.Sprintf("%v", c.sym))
				os.Setenv("PATW", fmt.Sprintf("%v", c.path))
				os.Setenv("CONT", fmt.Sprintf("%v", c.cont))
				os.Setenv("SEG", fmt.Sprintf("%d", c.seg))
				cp := make([]indexer.SearchResult, len(raw))
				copy(cp, raw)
				final := reranker.NewReranker().Rerank(sc.tc.Query, cp, 5)
				fr := rank(final, sc.tc.Expected)
				r := m[c]
				if rawRank > 0 && rawRank <= 5 && fr > 0 && fr <= 5 {
					r.both++
				} else if rawRank > 5 && fr > 0 && fr <= 5 {
					r.prom++
				} else if rawRank > 0 && rawRank <= 5 && fr == -1 {
					r.dem++
				}
				m[c] = r
			}
		}
	}
	best := 0
	for _, c := range cfgs {
		r := m[c]
		fin := r.both + r.prom
		tag := "  "
		if fin > best {
			best = fin
			tag = ">>"
		}
		fmt.Printf("%s SEG=%d SYM=%.2f PATW=%.2f CONT=%.2f both=%d prom=%d dem=%d final=%d\n",
			tag, c.seg, c.sym, c.path, c.cont, r.both, r.prom, r.dem, fin)
	}
	fmt.Printf("BEST final=%d\n", best)
}