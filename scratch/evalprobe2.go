package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/ashishpaliwal/semsearch/pkg/embedder"
	"github.com/ashishpaliwal/semsearch/pkg/indexer"
)

func probe2() {
	cases := []struct {
		repo, q, exp string
	}{
		{"clap", "defining a short and long flag for an option", "clap_builder/src/builder/arg.rs"},
		{"clap", "returning a clap error when parsing fails", "clap_builder/src/error"},
		{"hugo", "rendering the site to its output files", "hugolib/site_render.go"},
		{"hugo", "converting markdown content to html", "markup/goldmark/convert.go"},
		{"hugo", "minifying css and js output assets", "minifiers/minifiers.go"},
		{"hugo", "publishing rendered pages to disk", "publisher/publisher.go"},
		{"hugo", "handling the hugo serve command with hot reload", "commands/hugobuilder.go"},
	}
	tok, err := os.ReadFile("scratch/code_minilm/tokenizer.json")
	if err != nil { log.Fatal(err) }
	emb, err := embedder.NewEmbedder("scratch/code_minilm/model_quantized.onnx", tok)
	if err != nil { log.Fatal(err) }
	stores := map[string]*indexer.IndexStore{}
	for _, c := range cases {
		if _, ok := stores[c.repo]; !ok {
			st := indexer.NewIndexStore("scratch/external_eval/" + c.repo)
			if err := st.Load(); err != nil { log.Fatal(err) }
			stores[c.repo] = st
		}
	}
	for _, c := range cases {
		qv, err := emb.EmbedBatch([]string{c.q}, "search_query: ")
		if err != nil { continue }
		raw := stores[c.repo].HybridSearch(qv, c.q, 30, 0.0, 0.65, true, false)
		fmt.Printf("== %s | %s | exp=%s\n", c.repo, c.q, c.exp)
		for i, rr := range raw[:min(10, len(raw))] {
			mark := ""
			if strings.Contains(rr.Chunk.FilePath, c.exp) { mark = "  <<< CORRECT" }
			fmt.Printf("   #%d %.4f %s [%s]%s\n", i+1, rr.Score, rr.Chunk.FilePath, rr.Chunk.SymbolName, mark)
		}
	}
}
func main() { probe2() }
