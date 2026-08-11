package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ashishpaliwal/semsearch/pkg/chunker"
	"github.com/ashishpaliwal/semsearch/pkg/embedder"
	"github.com/ashishpaliwal/semsearch/pkg/indexer"
	"github.com/ashishpaliwal/semsearch/pkg/reranker"
)

type TestCase struct {
	Query                 string `json:"query"`
	Category              string `json:"category"`
	ExpectedFileSubstring string `json:"expected_file_substring"`
}

var expandedTestCases = []TestCase{
	// --- CONCEPTUAL QUERIES (Natural Language - 20 Cases) ---
	{Query: "where is the HTTP request router tree matching logic", Category: "conceptual", ExpectedFileSubstring: "tree.go"},
	{Query: "how to handle JSON body binding and validation errors", Category: "conceptual", ExpectedFileSubstring: "binding/json.go"},
	{Query: "recovering from panics and printing stack trace middleware", Category: "conceptual", ExpectedFileSubstring: "recovery.go"},
	{Query: "writing colored HTTP access logs to stdout or file", Category: "conceptual", ExpectedFileSubstring: "logger.go"},
	{Query: "parsing multiparts file upload and saving to disk", Category: "conceptual", ExpectedFileSubstring: "context.go"},
	{Query: "cross origin resource sharing CORS header settings", Category: "conceptual", ExpectedFileSubstring: "mode.go"},
	{Query: "where do we render HTML templates and static files", Category: "conceptual", ExpectedFileSubstring: "render/html.go"},
	{Query: "serving static assets and directory index files", Category: "conceptual", ExpectedFileSubstring: "fs.go"},
	{Query: "setting response cookie with security attributes", Category: "conceptual", ExpectedFileSubstring: "context.go"},
	{Query: "how to abort request pipeline processing early", Category: "conceptual", ExpectedFileSubstring: "context.go"},
	{Query: "where are custom validator tag rules registered", Category: "conceptual", ExpectedFileSubstring: "binding/default_validator.go"},
	{Query: "protobuf protocol buffer response rendering", Category: "conceptual", ExpectedFileSubstring: "render/protobuf.go"},
	{Query: "YAML payload request binding validator", Category: "conceptual", ExpectedFileSubstring: "binding/yaml.go"},
	{Query: "how to redirect incoming HTTP request to another URL", Category: "conceptual", ExpectedFileSubstring: "context.go"},
	{Query: "where is gin release mode debug mode configured", Category: "conceptual", ExpectedFileSubstring: "mode.go"},
	{Query: "extracting query parameters from URL path", Category: "conceptual", ExpectedFileSubstring: "context.go"},
	{Query: "binding XML payload request body", Category: "conceptual", ExpectedFileSubstring: "binding/xml.go"},
	{Query: "TOML configuration file request binding", Category: "conceptual", ExpectedFileSubstring: "binding/toml.go"},
	{Query: "custom HTTP status code json error response", Category: "conceptual", ExpectedFileSubstring: "context.go"},
	{Query: "writing binary stream data response", Category: "conceptual", ExpectedFileSubstring: "render/data.go"},

	// --- EXACT SYMBOL QUERIES (Identifiers - 15 Cases) ---
	{Query: "Engine struct definition", Category: "exact_symbol", ExpectedFileSubstring: "gin.go"},
	{Query: "Context struct definition", Category: "exact_symbol", ExpectedFileSubstring: "context.go"},
	{Query: "func (engine *Engine) Run", Category: "exact_symbol", ExpectedFileSubstring: "gin.go"},
	{Query: "func (c *Context) JSON", Category: "exact_symbol", ExpectedFileSubstring: "context.go"},
	{Query: "func (c *Context) BindJSON", Category: "exact_symbol", ExpectedFileSubstring: "context.go"},
	{Query: "type HandlerFunc func(*Context)", Category: "exact_symbol", ExpectedFileSubstring: "gin.go"},
	{Query: "func RecoveryWithWriter", Category: "exact_symbol", ExpectedFileSubstring: "recovery.go"},
	{Query: "func LoggerWithConfig", Category: "exact_symbol", ExpectedFileSubstring: "logger.go"},
	{Query: "type RouterGroup struct", Category: "exact_symbol", ExpectedFileSubstring: "routergroup.go"},
	{Query: "func (group *RouterGroup) GET", Category: "exact_symbol", ExpectedFileSubstring: "routergroup.go"},
	{Query: "type node struct", Category: "exact_symbol", ExpectedFileSubstring: "tree.go"},
	{Query: "func (c *Context) Status", Category: "exact_symbol", ExpectedFileSubstring: "context.go"},
	{Query: "func (c *Context) Redirect", Category: "exact_symbol", ExpectedFileSubstring: "context.go"},
	{Query: "func (c *Context) SetCookie", Category: "exact_symbol", ExpectedFileSubstring: "context.go"},
	{Query: "func Default() *Engine", Category: "exact_symbol", ExpectedFileSubstring: "gin.go"},
}

type ModelConfig struct {
	Name          string
	ModelPath     string
	TokenizerPath string
	Dim           int
}

func main() {
	rootDir := "scratch/external_eval/gin"

	models := []ModelConfig{
		{
			Name:          "Model 1: Nomic-Embed-Text-v1.5 (Prose Baseline, 768-dim, 137 MB)",
			ModelPath:     "scratch/model/model_quantized.onnx",
			TokenizerPath: "scratch/model/tokenizer.json",
			Dim:           768,
		},
		{
			Name:          "Model 2: CodeMiniLM-v2 (Code Search Fine-tune, 384-dim, 22 MB)",
			ModelPath:     "scratch/code_minilm/model_quantized.onnx",
			TokenizerPath: "scratch/code_minilm/tokenizer.json",
			Dim:           384,
		},
		{
			Name:          "Model 3: CodeBERT-Base (Multi-Language Code BERT, 768-dim, 108 MB)",
			ModelPath:     "scratch/codebert_model/model_quantized.onnx",
			TokenizerPath: "scratch/codebert_model/tokenizer.json",
			Dim:           768,
		},
	}

	crossReranker, errRerank := reranker.NewCrossEncoderReranker("scratch/cross_encoder/model_quantized.onnx", "scratch/cross_encoder/tokenizer.json")
	if errRerank != nil {
		fmt.Printf("Notice: Cross-encoder reranker init: %v (falling back to term-match boost)\n", errRerank)
	}

	heuristicReranker := reranker.NewReranker()

	fmt.Printf("=========================================================================================\n")
	fmt.Printf("   `semsearch` 3-MODEL BENCHMARK & MULTI-STAGE RETRIEVAL EVALUATION SUITE                 \n")
	fmt.Printf("   Evaluated Repository: `gin-gonic/gin` | Total Test Queries: %d                          \n", len(expandedTestCases))
	fmt.Printf("=========================================================================================\n\n")

	for _, m := range models {
		fmt.Printf(">>> EVALUATING %s <<<\n", m.Name)

		tokenizerJSON, err := os.ReadFile(m.TokenizerPath)
		if err != nil {
			log.Fatalf("Failed to read tokenizer: %v", err)
		}

		emb, err := embedder.NewEmbedder(m.ModelPath, tokenizerJSON)
		if err != nil {
			log.Fatalf("Failed to load embedder: %v", err)
		}

		// Re-index target directory with model
		store := indexer.NewIndexStore(rootDir)
		var files []string
		filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && !strings.Contains(path, "/.semsearch") && !strings.HasPrefix(info.Name(), ".") {
				files = append(files, path)
			}
			return nil
		})

		var allChunks []chunker.Chunk
		defaultChunker := chunker.NewDefaultChunker()
		for _, f := range files {
			chunks, _ := defaultChunker.ChunkFile(f)
			allChunks = append(allChunks, chunks...)
		}

		var texts []string
		for _, ch := range allChunks {
			texts = append(texts, ch.Content)
		}

		vectors, err := emb.EmbedBatch(texts, "search_document: ")
		if err != nil {
			fmt.Printf("Skipping %s: embedding failed (%v)\n\n", m.Name, err)
			continue
		}

		manifest := indexer.Manifest{
			Version:           "1.0",
			Model:             m.Name,
			VectorDimensions:  m.Dim,
			TotalChunks:       len(allChunks),
			IndexedFilesCount: len(files),
			Chunks:            allChunks,
		}
		if err := store.Save(manifest, vectors); err != nil {
			fmt.Printf("Skipping %s: store save failed (%v)\n\n", m.Name, err)
			continue
		}

		evalModel(emb, store, m, heuristicReranker, crossReranker, expandedTestCases)
		fmt.Printf("\n")
	}
}

func evalModel(emb *embedder.Embedder, store *indexer.IndexStore, m ModelConfig, hReranker *reranker.Reranker, cReranker *reranker.CrossEncoderReranker, cases []TestCase) {
	if err := store.Load(); err != nil {
		fmt.Printf("Skipping %s: failed to load index (%v)\n", m.Name, err)
		return
	}

	var conceptualTop1, conceptualTop3, conceptualTop5, conceptualCount int
	var exactTop1, exactTop3, exactTop5, exactCount int

	for _, tc := range cases {
		queryVec, err := emb.EmbedBatch([]string{tc.Query}, "search_query: ")
		if err != nil {
			continue
		}

		rawCandidates := store.HybridSearch(queryVec, tc.Query, 20, 0.0, 0.6, false, false)

		// Apply term match boost & cross-encoder reranker
		results := hReranker.Rerank(tc.Query, rawCandidates, 10)
		if cReranker != nil {
			results = cReranker.Rerank(tc.Query, results, 5)
		} else if len(results) > 5 {
			results = results[:5]
		}

		hitRank := -1
		for rank, res := range results {
			if strings.HasSuffix(res.Chunk.FilePath, tc.ExpectedFileSubstring) || strings.Contains(res.Chunk.FilePath, tc.ExpectedFileSubstring) {
				hitRank = rank + 1
				break
			}
		}

		if tc.Category == "conceptual" {
			conceptualCount++
			if hitRank == 1 {
				conceptualTop1++
				conceptualTop3++
				conceptualTop5++
			} else if hitRank > 1 && hitRank <= 3 {
				conceptualTop3++
				conceptualTop5++
			} else if hitRank > 3 && hitRank <= 5 {
				conceptualTop5++
			}
		} else {
			exactCount++
			if hitRank == 1 {
				exactTop1++
				exactTop3++
				exactTop5++
			} else if hitRank > 1 && hitRank <= 3 {
				exactTop3++
				exactTop5++
			} else if hitRank > 3 && hitRank <= 5 {
				exactTop5++
			}
		}
	}

	fmt.Printf("   1. CONCEPTUAL QUERIES (N=%d):\n", conceptualCount)
	fmt.Printf("      Top-1: %2d/%2d (%5.1f%%) | Top-3: %2d/%2d (%5.1f%%) | Top-5: %2d/%2d (%5.1f%%)\n",
		conceptualTop1, conceptualCount, float64(conceptualTop1)/float64(conceptualCount)*100,
		conceptualTop3, conceptualCount, float64(conceptualTop3)/float64(conceptualCount)*100,
		conceptualTop5, conceptualCount, float64(conceptualTop5)/float64(conceptualCount)*100)

	fmt.Printf("   2. EXACT SYMBOL QUERIES (N=%d):\n", exactCount)
	fmt.Printf("      Top-1: %2d/%2d (%5.1f%%) | Top-3: %2d/%2d (%5.1f%%) | Top-5: %2d/%2d (%5.1f%%)\n",
		exactTop1, exactCount, float64(exactTop1)/float64(exactCount)*100,
		exactTop3, exactCount, float64(exactTop3)/float64(exactCount)*100,
		exactTop5, exactCount, float64(exactTop5)/float64(exactCount)*100)
}
