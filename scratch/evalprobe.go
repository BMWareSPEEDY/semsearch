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

func checkRes(res []indexer.SearchResult, exp string) bool {
	for _, rr := range res {
		if strings.HasSuffix(rr.Chunk.FilePath, exp) || strings.Contains(rr.Chunk.FilePath, exp) {
			return true
		}
	}
	return false
}

func probeMain() {
	queries := []string{
		"applying default configuration values",
		"loading configuration from files into the runtime",
		"reading the site language configuration",
		"building a root command for a command line application",
		"type Command struct",
	}
	tok, err := os.ReadFile("scratch/code_minilm/tokenizer.json")
	if err != nil {
		log.Fatal(err)
	}
	emb, err := embedder.NewEmbedder("scratch/code_minilm/model_quantized.onnx", tok)
	if err != nil {
		log.Fatal(err)
	}
	r := reranker.NewReranker()
	repos := map[string]string{
		"applying default configuration values":              "hugo",
		"loading configuration from files into the runtime":  "hugo",
		"reading the site language configuration":            "hugo",
		"building a root command for a command line application": "cobra",
		"type Command struct":                                "cobra",
	}
	cands := map[string]string{
		"applying default configuration values":              "config/allconfig/load.go",
		"loading configuration from files into the runtime":  "config/configLoader.go",
		"reading the site language configuration":            "langs/config.go",
		"building a root command for a command line application": "command.go",
		"type Command struct":                                "command.go",
	}
	stores := map[string]*indexer.IndexStore{}
	for _, repo := range repos {
		st := indexer.NewIndexStore("scratch/external_eval/" + repo)
		if err := st.Load(); err != nil {
			log.Fatal(err)
		}
		stores[repo] = st
	}
	for _, q := range queries {
		repo := repos[q]
		qv, err := emb.EmbedBatch([]string{q}, "search_query: ")
		if err != nil {
			continue
		}
		raw := stores[repo].HybridSearch(qv, q, 30, 0.0, 0.65, true, false)
		final := r.Rerank(q, append([]indexer.SearchResult(nil), raw...), 5)
		fmt.Printf("== %s (candidate %s in top5: %v)\n", q, cands[q], checkRes(final, cands[q]))
		for k, rr := range final {
			fmt.Printf("   F%d(%.4f) %s [%s]\n", k+1, rr.Score, rr.Chunk.FilePath, rr.Chunk.SymbolName)
		}
		sp := -1
		for k, rr := range final {
			if strings.HasSuffix(rr.Chunk.FilePath, cands[q]) || strings.Contains(rr.Chunk.FilePath, cands[q]) {
				sp = k + 1
			}
		}
		fmt.Printf("   candidate rank: %d\n\n", sp)
	}
}