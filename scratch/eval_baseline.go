package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ashishpaliwal/semsearch/pkg/chunker"
	"github.com/ashishpaliwal/semsearch/pkg/embedder"
	"github.com/ashishpaliwal/semsearch/pkg/indexer"
	"github.com/ashishpaliwal/semsearch/pkg/reranker"
	"github.com/ashishpaliwal/semsearch/pkg/scanner"
)

type baselineCase struct {
	Query    string
	Category string
	Expected string
}

var baselineCases = []baselineCase{
	{"where is the HTTP request router tree matching logic", "conceptual", "tree.go"},
	{"how to handle JSON body binding and validation errors", "conceptual", "binding/json.go"},
	{"recovering from panics and printing stack trace middleware", "conceptual", "recovery.go"},
	{"writing colored HTTP access logs to stdout or file", "conceptual", "logger.go"},
	{"parsing multiparts file upload and saving to disk", "conceptual", "context.go"},
	{"cross origin resource sharing CORS header settings", "conceptual", "mode.go"},
	{"where do we render HTML templates and static files", "conceptual", "render/html.go"},
	{"serving static assets and directory index files", "conceptual", "fs.go"},
	{"setting response cookie with security attributes", "conceptual", "context.go"},
	{"how to abort request pipeline processing early", "conceptual", "context.go"},
	{"where are custom validator tag rules registered", "conceptual", "binding/default_validator.go"},
	{"protobuf protocol buffer response rendering", "conceptual", "render/protobuf.go"},
	{"YAML payload request binding validator", "conceptual", "binding/yaml.go"},
	{"how to redirect incoming HTTP request to another URL", "conceptual", "context.go"},
	{"where is gin release mode debug mode configured", "conceptual", "mode.go"},
	{"Engine struct definition", "exact_symbol", "gin.go"},
	{"Context struct definition", "exact_symbol", "context.go"},
	{"func (engine *Engine) Run", "exact_symbol", "gin.go"},
	{"func (c *Context) JSON", "exact_symbol", "context.go"},
	{"func (c *Context) BindJSON", "exact_symbol", "context.go"},
	{"type HandlerFunc func(*Context)", "exact_symbol", "gin.go"},
	{"func RecoveryWithWriter", "exact_symbol", "recovery.go"},
	{"func LoggerWithConfig", "exact_symbol", "logger.go"},
	{"type RouterGroup struct", "exact_symbol", "routergroup.go"},
	{"func (group *RouterGroup) GET", "exact_symbol", "routergroup.go"},
}

// Builds a code-only single-vector index (no description vectors) for the gin
// repo and runs the identical 25-query eval. This is the TRUE pre-change
// baseline, uncontaminated by any earlier dual-vector/description work.
func main() {
	repoDir := "scratch/external_eval/gin"
	outDir := "scratch/external_eval/gin_baseline"

	store := indexer.NewIndexStore(outDir)
	if err := store.Load(); err != nil {
		buildCodeOnlyIndex(repoDir, outDir)
		store = indexer.NewIndexStore(outDir)
		if err := store.Load(); err != nil {
			log.Fatalf("baseline index load failed: %v", err)
		}
	}

	tokenizerJSON, _ := os.ReadFile("scratch/code_minilm/tokenizer.json")
	emb, err := embedder.NewEmbedder("scratch/code_minilm/model_quantized.onnx", tokenizerJSON)
	if err != nil {
		log.Fatalf("embedder: %v", err)
	}

	r := reranker.NewReranker()

	var cT1, cT3, cT5, cN, eT1, eT3, eT5, eN int
	fmt.Printf("CODE-ONLY SINGLE-VECTOR BASELINE (CodeMiniLM, %d chunks)\n", store.Manifest.TotalChunks)
	for _, tc := range baselineCases {
		qv, err := emb.EmbedBatch([]string{tc.Query}, "search_query: ")
		if err != nil {
			continue
		}
		raw := store.HybridSearch(qv, tc.Query, 20, 0.0, 0.6, true, false)
		res := r.Rerank(tc.Query, raw, 5)
		hit := -1
		for k, rr := range res {
			if strings.HasSuffix(rr.Chunk.FilePath, tc.Expected) || strings.Contains(rr.Chunk.FilePath, tc.Expected) {
				hit = k + 1
				break
			}
		}
		if tc.Category == "conceptual" {
			cN++
			if hit >= 1 && hit <= 5 {
				cT5++
			}
			if hit >= 1 && hit <= 3 {
				cT3++
			}
			if hit == 1 {
				cT1++
			}
		} else {
			eN++
			if hit >= 1 && hit <= 5 {
				eT5++
			}
			if hit >= 1 && hit <= 3 {
				eT3++
			}
			if hit == 1 {
				eT1++
			}
		}
	}

	fmt.Printf("  CONCEPTUAL top1=%d/%d top3=%d/%d top5=%d/%d\n", cT1, cN, cT3, cN, cT5, cN)
	fmt.Printf("  EXACT      top1=%d/%d top3=%d/%d top5=%d/%d\n", eT1, eN, eT3, eN, eT5, eN)

	out, _ := json.MarshalIndent(map[string]interface{}{
		"mode":            "code-only-single-vector",
		"chunks":          store.Manifest.TotalChunks,
		"conceptual_top1": cT1, "conceptual_top3": cT3, "conceptual_top5": cT5,
		"conceptual_count": cN,
		"exact_top1":       eT1, "exact_top3": eT3, "exact_top5": eT5,
		"exact_count": eN,
	}, "", "  ")
	os.WriteFile("scratch/baseline_results.json", out, 0644)
	fmt.Println("wrote scratch/baseline_results.json")
}

func buildCodeOnlyIndex(repoDir, outDir string) {
	sc, err := scanner.NewFileScanner(repoDir)
	if err != nil {
		log.Fatalf("scanner: %v", err)
	}
	files, err := sc.Scan()
	if err != nil {
		log.Fatalf("scan: %v", err)
	}

	var allChunks []chunker.Chunk
	chunker := chunker.NewDefaultChunker()
	for _, f := range files {
		chunks, _ := chunker.ChunkFile(f)
		allChunks = append(allChunks, chunks...)
	}

	tokenizerJSON, _ := os.ReadFile("scratch/code_minilm/tokenizer.json")
	emb, err := embedder.NewEmbedder("scratch/code_minilm/model_quantized.onnx", tokenizerJSON)
	if err != nil {
		log.Fatalf("embedder: %v", err)
	}

	var texts []string
	for _, ch := range allChunks {
		texts = append(texts, ch.Content)
	}
	vectors, err := emb.EmbedBatch(texts, "search_document: ")
	if err != nil {
		log.Fatalf("embed: %v", err)
	}

	manifest := indexer.Manifest{
		Version:           "1.0",
		CreatedAt:         time.Now(),
		Model:             "code-only-baseline-code-minilm",
		VectorDimensions:  emb.Dimension(),
		TotalChunks:       len(allChunks),
		IndexedFilesCount: len(files),
		Chunks:            allChunks,
	}
	store := indexer.NewIndexStore(outDir)
	if err := store.Save(manifest, vectors); err != nil {
		log.Fatalf("save baseline: %v", err)
	}
}
