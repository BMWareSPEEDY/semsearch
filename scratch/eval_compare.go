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

func compareRerankers() {
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

	// Variant B: original + segment-matching for symbol and path (exact segment
	// match only, no leniency), content stays exact-word. Mirrors
	// segmentize/hasSegment from the earlier iteration.
	passOrig := func(res []indexer.SearchResult, expected string) bool {
		for _, rr := range res {
			if strings.HasSuffix(rr.Chunk.FilePath, expected) || strings.Contains(rr.Chunk.FilePath, expected) {
				return true
			}
		}
		return false
	}

	weight := 0.75
	shortlist := 50
	origFails := map[string]string{}
	newFails := map[string]string{}

	b := reranker.NewReranker()
	_ = b

	for _, d := range datasets {
		for _, sc := range d.cases {
			res := d.store.HybridSearch(sc.qvec, sc.tc.Query, shortlist, 0.0, float32(weight), true, false)
			resB := make([]indexer.SearchResult, len(res))
			copy(resB, res)
			okA := passOrig(resB, sc.tc.Expected)
			if !okA {
				origFails[sc.tc.Query] = sc.tc.Expected
			} else {
				_ = resB
			}
		}
	}
	fmt.Printf("original failures: %d\n", len(origFails))
	for q, e := range origFails {
		fmt.Printf("  %q -> %s\n", q, e)
	}
	_ = newFails
}