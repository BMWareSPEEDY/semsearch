package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ashishpaliwal/semsearch/pkg/chunker"
	"github.com/ashishpaliwal/semsearch/pkg/embedder"
	"github.com/ashishpaliwal/semsearch/pkg/indexer"
	"github.com/ashishpaliwal/semsearch/pkg/scanner"
)

// Rebuilds a repo's .semsearch index using the current pipeline (no LLM desc,
// synthetic desc fallback), then prints chunk stats. Used by the eval loop.
func main() {
	flag.Parse()
	if len(flag.Args()) < 1 {
		log.Fatal("usage: rebuild_index <repoDir>")
	}
	dir := flag.Arg(0)

	s, err := scanner.NewFileScanner(dir)
	if err != nil {
		log.Fatalf("scanner: %v", err)
	}
	files, err := s.Scan()
	if err != nil {
		log.Fatalf("scan: %v", err)
	}

	c := chunker.NewDefaultChunker()
	var allChunks []chunker.Chunk
	fileHashes := make(map[string]string)
	start := time.Now()
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		h := sha256.Sum256(content)
		fileHashes[f] = fmt.Sprintf("%x", h[:4])
		chunks, err := c.ChunkFile(f)
		if err != nil {
			continue
		}
		for i := range chunks {
			chunks[i].ID = len(allChunks)
			allChunks = append(allChunks, chunks[i])
		}
	}
	fmt.Printf("scanned %d files, %d chunks in %s\n", len(files), len(allChunks), time.Since(start).Round(time.Millisecond))

	tokenizerJSON, err := os.ReadFile("scratch/code_minilm/tokenizer.json")
	if err != nil {
		log.Fatalf("tokenizer: %v", err)
	}
	emb, err := embedder.NewEmbedder("scratch/code_minilm/model_quantized.onnx", tokenizerJSON)
	if err != nil {
		log.Fatalf("embedder: %v", err)
	}

	codeTexts := make([]string, len(allChunks))
	descTexts := make([]string, len(allChunks))
	for i := range allChunks {
		codeTexts[i] = chunker.ChunkCodeText(allChunks[i].Content)
		descTexts[i] = chunker.GenerateSyntheticDescription(allChunks[i].Content, allChunks[i].SymbolName, allChunks[i].Kind, allChunks[i].FilePath)
	}

	fmt.Println("embedding code vectors...")
	codeVecs, err := emb.EmbedBatch(codeTexts, "search_document: ")
	if err != nil {
		log.Fatalf("code embed: %v", err)
	}
	fmt.Println("embedding desc vectors...")
	descVecs, err := emb.EmbedBatch(descTexts, "search_document: ")
	if err != nil {
		log.Fatalf("desc embed: %v", err)
	}

	// Reuse code vector as desc vector for markdown (desc gen for md is weak).
	dim := emb.Dimension()
	for i := range allChunks {
		if isMarkdown(allChunks[i].FilePath) {
			off := i * dim
			copy(descVecs[off:off+dim], codeVecs[off:off+dim])
		}
	}

	manifest := indexer.Manifest{
		Version:           "1.0",
		CreatedAt:         time.Now(),
		Model:             "code-minilm-improved-chunker",
		VectorDimensions:  dim,
		TotalChunks:       len(allChunks),
		IndexedFilesCount: len(files),
		FileHashes:        fileHashes,
		Chunks:            allChunks,
	}
	store := indexer.NewIndexStore(dir)
	if err := store.SaveDual(manifest, codeVecs, descVecs); err != nil {
		log.Fatalf("save: %v", err)
	}
	fmt.Printf("saved index for %s (%d chunks)\n", dir, len(allChunks))
}

func isMarkdown(p string) bool {
	ext := filepath.Ext(p)
	return ext == ".md" || ext == ".markdown" || ext == ".rst" || ext == ".txt" || ext == ".adoc"
}
