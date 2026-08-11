package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ashishpaliwal/semsearch/pkg/embedder"
	"github.com/ashishpaliwal/semsearch/pkg/indexer"
	"github.com/ashishpaliwal/semsearch/pkg/reranker"
)

type TestCase struct {
	Query               string `json:"query"`
	Category            string `json:"category"` // "conceptual" vs "exact_symbol"
	ExpectedFileSubstring string `json:"expected_file_substring"`
}

var externalTestCases = []TestCase{
	// --- CATEGORY 1: CONCEPTUAL / NATURAL LANGUAGE QUERIES (The "Hard Part") ---
	{
		Query:               "where is the HTTP request router tree matching logic",
		Category:            "conceptual",
		ExpectedFileSubstring: "tree.go",
	},
	{
		Query:               "how to handle JSON body binding and validation errors",
		Category:            "conceptual",
		ExpectedFileSubstring: "binding/json.go",
	},
	{
		Query:               "recovering from panics and printing stack trace middleware",
		Category:            "conceptual",
		ExpectedFileSubstring: "recovery.go",
	},
	{
		Query:               "writing colored HTTP access logs to stdout or file",
		Category:            "conceptual",
		ExpectedFileSubstring: "logger.go",
	},
	{
		Query:               "parsing multiparts file upload and saving to disk",
		Category:            "conceptual",
		ExpectedFileSubstring: "context.go",
	},
	{
		Query:               "cross origin resource sharing CORS header settings",
		Category:            "conceptual",
		ExpectedFileSubstring: "mode.go",
	},
	{
		Query:               "where do we render HTML templates and static files",
		Category:            "conceptual",
		ExpectedFileSubstring: "render/html.go",
	},
	{
		Query:               "serving static assets and directory index files",
		Category:            "conceptual",
		ExpectedFileSubstring: "fs.go",
	},
	{
		Query:               "setting response cookie with security attributes",
		Category:            "conceptual",
		ExpectedFileSubstring: "context.go",
	},
	{
		Query:               "how to abort request pipeline processing early",
		Category:            "conceptual",
		ExpectedFileSubstring: "context.go",
	},
	{
		Query:               "where are custom validator tag rules registered",
		Category:            "conceptual",
		ExpectedFileSubstring: "binding/default_validator.go",
	},
	{
		Query:               "protobuf protocol buffer response rendering",
		Category:            "conceptual",
		ExpectedFileSubstring: "render/protobuf.go",
	},
	{
		Query:               "YAML payload request binding validator",
		Category:            "conceptual",
		ExpectedFileSubstring: "binding/yaml.go",
	},
	{
		Query:               "how to redirect incoming HTTP request to another URL",
		Category:            "conceptual",
		ExpectedFileSubstring: "context.go",
	},
	{
		Query:               "where is gin release mode debug mode configured",
		Category:            "conceptual",
		ExpectedFileSubstring: "mode.go",
	},

	// --- CATEGORY 2: EXACT SYMBOL / IDENTIFIER QUERIES (BM25 / Keyword Baseline) ---
	{
		Query:               "Engine struct definition",
		Category:            "exact_symbol",
		ExpectedFileSubstring: "gin.go",
	},
	{
		Query:               "Context struct definition",
		Category:            "exact_symbol",
		ExpectedFileSubstring: "context.go",
	},
	{
		Query:               "func (engine *Engine) Run",
		Category:            "exact_symbol",
		ExpectedFileSubstring: "gin.go",
	},
	{
		Query:               "func (c *Context) JSON",
		Category:            "exact_symbol",
		ExpectedFileSubstring: "context.go",
	},
	{
		Query:               "func (c *Context) BindJSON",
		Category:            "exact_symbol",
		ExpectedFileSubstring: "context.go",
	},
	{
		Query:               "type HandlerFunc func(*Context)",
		Category:            "exact_symbol",
		ExpectedFileSubstring: "gin.go",
	},
	{
		Query:               "func RecoveryWithWriter",
		Category:            "exact_symbol",
		ExpectedFileSubstring: "recovery.go",
	},
	{
		Query:               "func LoggerWithConfig",
		Category:            "exact_symbol",
		ExpectedFileSubstring: "logger.go",
	},
	{
		Query:               "type RouterGroup struct",
		Category:            "exact_symbol",
		ExpectedFileSubstring: "routergroup.go",
	},
	{
		Query:               "func (group *RouterGroup) GET",
		Category:            "exact_symbol",
		ExpectedFileSubstring: "routergroup.go",
	},
}

func main() {
	rootDir := "scratch/external_eval/gin"
	store := indexer.NewIndexStore(rootDir)
	if err := store.Load(); err != nil {
		log.Fatalf("Failed to load gin index: %v", err)
	}

	tokenizerJSON, err := os.ReadFile("scratch/code_minilm/tokenizer.json")
	if err != nil {
		log.Fatalf("Failed to read tokenizer: %v", err)
	}

	emb, err := embedder.NewEmbedder("scratch/code_minilm/model_quantized.onnx", tokenizerJSON)
	if err != nil {
		log.Fatalf("Failed to initialize embedder: %v", err)
	}

	r := reranker.NewReranker()

	total := len(externalTestCases)
	var conceptualTop1, conceptualTop3, conceptualTop5, conceptualCount int
	var exactTop1, exactTop3, exactTop5, exactCount int

	fmt.Printf("==============================================================\n")
	fmt.Printf("   EXTERNAL UNFAMILIAR REPO EVALUATION: `gin-gonic/gin`       \n")
	fmt.Printf("   Total Evaluated Queries: %d\n", total)
	fmt.Printf("==============================================================\n\n")

	for i, tc := range externalTestCases {
		queryVec, err := emb.EmbedBatch([]string{tc.Query}, "search_query: ")
		if err != nil {
			log.Fatalf("Embedding error on query '%s': %v", tc.Query, err)
		}

		rawCandidates := store.HybridSearch(queryVec, tc.Query, 20, 0.0, 0.6, true, false)
		results := r.Rerank(tc.Query, rawCandidates, 5)

		hitRank := -1
		for rank, res := range results {
			if strings.HasSuffix(res.Chunk.FilePath, tc.ExpectedFileSubstring) || strings.Contains(res.Chunk.FilePath, tc.ExpectedFileSubstring) {
				hitRank = rank + 1
				break
			}
		}

		status := "FAIL"
		if hitRank == 1 {
			status = "TOP-1 PASS"
		} else if hitRank > 1 && hitRank <= 3 {
			status = fmt.Sprintf("TOP-%d PASS", hitRank)
		} else if hitRank > 3 && hitRank <= 5 {
			status = fmt.Sprintf("TOP-%d PASS", hitRank)
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

		topMatch := "None"
		if len(results) > 0 {
			topMatch = filepath.Base(results[0].Chunk.FilePath)
		}

		fmt.Printf("[%2d/%2d] [%-12s] Query: \"%s\"\n", i+1, total, tc.Category, tc.Query)
		fmt.Printf("       Expected: %-25s | Top Match: %-20s | Result: [%s]\n", tc.ExpectedFileSubstring, topMatch, status)
	}

	fmt.Printf("\n==============================================================\n")
	fmt.Printf("           FINAL EXPLICIT EVALUATION METRICS BY TYPE          \n")
	fmt.Printf("==============================================================\n")
	fmt.Printf("1. CONCEPTUAL / NATURAL LANGUAGE QUERIES (n=%d):\n", conceptualCount)
	fmt.Printf("   Top-1 Accuracy : %2d / %2d  (%.1f%%)\n", conceptualTop1, conceptualCount, float64(conceptualTop1)/float64(conceptualCount)*100)
	fmt.Printf("   Top-3 Accuracy : %2d / %2d  (%.1f%%)\n", conceptualTop3, conceptualCount, float64(conceptualTop3)/float64(conceptualCount)*100)
	fmt.Printf("   Top-5 Accuracy : %2d / %2d  (%.1f%%)\n", conceptualTop5, conceptualCount, float64(conceptualTop5)/float64(conceptualCount)*100)
	fmt.Printf("\n2. EXACT SYMBOL / IDENTIFIER QUERIES (n=%d):\n", exactCount)
	fmt.Printf("   Top-1 Accuracy : %2d / %2d  (%.1f%%)\n", exactTop1, exactCount, float64(exactTop1)/float64(exactCount)*100)
	fmt.Printf("   Top-3 Accuracy : %2d / %2d  (%.1f%%)\n", exactTop3, exactCount, float64(exactTop3)/float64(exactCount)*100)
	fmt.Printf("   Top-5 Accuracy : %2d / %2d  (%.1f%%)\n", exactTop5, exactCount, float64(exactTop5)/float64(exactCount)*100)
	fmt.Printf("==============================================================\n")

	reportData := map[string]interface{}{
		"conceptual_count": conceptualCount,
		"conceptual_top1":  conceptualTop1,
		"conceptual_top3":  conceptualTop3,
		"conceptual_top5":  conceptualTop5,
		"exact_count":       exactCount,
		"exact_top1":        exactTop1,
		"exact_top3":        exactTop3,
		"exact_top5":        exactTop5,
	}

	outData, _ := json.MarshalIndent(reportData, "", "  ")
	os.WriteFile("scratch/external_eval_results.json", outData, 0644)
}
