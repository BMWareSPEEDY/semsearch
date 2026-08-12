package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ashishpaliwal/semsearch/pkg/chunker"
	"github.com/ashishpaliwal/semsearch/pkg/embedder"
	"github.com/ashishpaliwal/semsearch/pkg/indexer"
	"github.com/ashishpaliwal/semsearch/pkg/llm"
)

// llm_desc_subset generates LLM descriptions for chunks whose file path matches
// one of the given substrings, re-embeds only the changed description vectors,
// and saves the index. Usage: go run . <index-dir> <llm-model> <substr>,<substr>...
func main() {
	if len(os.Args) < 4 {
		fmt.Println("usage: <index-dir> <model> <substr,substr,...>")
		os.Exit(1)
	}
	dir := os.Args[1]
	model := os.Args[2]
	subs := strings.Split(os.Args[3], ",")

	store := indexer.NewIndexStore(dir)
	if err := store.Load(); err != nil {
		panic(err)
	}
	m := store.Manifest
	dim := int(m.VectorDimensions)

	tokenizerJSON, err := os.ReadFile("scratch/code_minilm/tokenizer.json")
	if err != nil {
		panic(err)
	}
	emb, err := embedder.NewEmbedder("scratch/code_minilm/model_quantized.onnx", tokenizerJSON)
	if err != nil {
		panic(err)
	}
	client := llm.NewOllamaClient(model)
	if !client.IsAvailable() {
		panic("llm not available")
	}

	// Select chunks in target files.
	sel := make([]int, 0)
	keep := make([]bool, len(m.Chunks))
	for i, ch := range m.Chunks {
		for _, s := range subs {
			if strings.Contains(ch.FilePath, s) {
				sel = append(sel, i)
				keep[i] = true
				break
			}
		}
	}
	fmt.Printf("selected %d chunks for LLM desc generation\n", len(sel))

	// Generate descs (hash-gated cache in the index dir).
	cache := llm.NewDescriptionCache(filepath.Join(dir, ".semsearch"))
	chunks := make([]chunker.Chunk, len(sel))
	for k, i := range sel {
		chunks[k] = m.Chunks[i]
	}
	descs, n := llm.GenerateDescriptions(context.Background(), client, cache, chunks, func(i int) bool { return false }, 8)
	fmt.Printf("generated %d new descs\n", n)

	// Re-embed changed descs and splice.
	newDescs := make([][]float32, len(m.Chunks))
	toEmbed := make([]string, 0)
	for k, i := range sel {
		if descs[k] == "" {
			continue
		}
		newDescs[i] = make([]float32, dim)
		toEmbed = append(toEmbed, descs[k])
	}
	vecs, err := emb.EmbedBatch(toEmbed, "search_document: ")
	if err != nil {
		panic(err)
	}
	idx := 0
	for i := range newDescs {
		if newDescs[i] == nil {
			continue
		}
		copy(newDescs[i], vecs[idx*dim:(idx+1)*dim])
		idx++
	}

	outDesc := make([]float32, len(m.Chunks)*dim)
	copy(outDesc, store.DescVectors)
	for i, v := range newDescs {
		if v == nil {
			continue
		}
		copy(outDesc[i*dim:(i+1)*dim], v)
	}

	if err := store.SaveDual(m, store.Vectors, outDesc); err != nil {
		panic(err)
	}
	fmt.Printf("saved updated index at %s\n", filepath.Join(dir, ".semsearch"))
}
