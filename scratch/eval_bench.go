package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/ashishpaliwal/semsearch/pkg/embedder"
	"github.com/ashishpaliwal/semsearch/pkg/indexer"
	"github.com/ashishpaliwal/semsearch/pkg/reranker"
)

type BenchCase struct {
	Query    string
	Category string // conceptual | exact_symbol | error_handling | request_pipeline | rendering | config
	Expected string
}

type BenchRepo struct {
	Key   string
	Label string
	Dir   string
	Cases []BenchCase
}

var benchRepos = []BenchRepo{
	{Key: "gin", Label: "gin-gonic/gin (Go web framework)", Dir: "scratch/external_eval/gin", Cases: ginCases},
	{Key: "cobra", Label: "spf13/cobra (Go CLI framework)", Dir: "scratch/external_eval/cobra", Cases: cobraCases},
	{Key: "mux", Label: "gorilla/mux (Go HTTP router)", Dir: "scratch/external_eval/mux", Cases: muxCases},
	{Key: "clap", Label: "clap-rs/clap (Rust CLI argument parser)", Dir: "scratch/external_eval/clap", Cases: clapCases},
	{Key: "hugo", Label: "gohugoio/hugo (Go static site generator)", Dir: "scratch/external_eval/hugo", Cases: hugoCases},
}

func main() {
	only := map[string]bool{}
	for _, a := range os.Args[1:] {
		only[a] = true
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

	report := map[string]interface{}{}
	fmt.Printf("==============================================================\n")
	fmt.Printf("  SEMSEARCH MULTI-REPO EVALUATION (CodeMiniLM, code-only)\n")
	fmt.Printf("==============================================================\n")

	for _, repo := range benchRepos {
		if len(only) > 0 && !only[repo.Key] {
			continue
		}
		store := indexer.NewIndexStore(repo.Dir)
		if err := store.Load(); err != nil {
			fmt.Printf("SKIP %-6s no index: %v\n", repo.Key, err)
			continue
		}

		catCount := map[string]int{}
		catTop1 := map[string]int{}
		catTop3 := map[string]int{}
		catTop5 := map[string]int{}

		var totalCount, totalTop1, totalTop3, totalTop5 int

		fmt.Printf("\n>>> %s  (%d chunks, %d queries)\n", repo.Label, store.Manifest.TotalChunks, len(repo.Cases))
		for _, tc := range repo.Cases {
			qv, err := emb.EmbedBatch([]string{tc.Query}, "search_query: ")
			if err != nil {
				continue
			}
			w, sl := 0.6, 20
			if v := os.Getenv("W"); v != "" {
				fmt.Sscanf(v, "%f", &w)
			}
			if v := os.Getenv("SL"); v != "" {
				fmt.Sscanf(v, "%d", &sl)
			}
			raw := store.HybridSearch(qv, tc.Query, sl, 0.0, float32(w), true, false)
			res := r.Rerank(tc.Query, raw, 5)
			hit := -1
			for k, rr := range res {
				if strings.HasSuffix(rr.Chunk.FilePath, tc.Expected) || strings.Contains(rr.Chunk.FilePath, tc.Expected) {
					hit = k + 1
					break
				}
			}

			catCount[tc.Category]++
			totalCount++
			if hit >= 1 && hit <= 5 {
				catTop5[tc.Category]++
				totalTop5++
			}
			if hit >= 1 && hit <= 3 {
				catTop3[tc.Category]++
				totalTop3++
			}
			if hit == 1 {
				catTop1[tc.Category]++
				totalTop1++
			}

			status := "FAIL"
			if hit == 1 {
				status = "TOP-1"
			} else if hit > 1 && hit <= 5 {
				status = fmt.Sprintf("TOP-%d", hit)
			}
			fmt.Printf("  [%s] %-14s %q -> %s\n", status, tc.Category, tc.Query, tc.Expected)
		}

		fmt.Printf("\n  %s SUMMARY:\n", repo.Key)
		rows := make([]string, 0, len(catCount))
		for cat := range catCount {
			rows = append(rows, cat)
		}
		sort.Strings(rows)
		for _, cat := range rows {
			fmt.Printf("    %-16s top1=%d/%d top3=%d/%d top5=%d/%d\n", cat, catTop1[cat], catCount[cat], catTop3[cat], catCount[cat], catTop5[cat], catCount[cat])
		}
		fmt.Printf("    %-16s top1=%d/%d top3=%d/%d top5=%d/%d\n", "TOTAL", totalTop1, totalCount, totalTop3, totalCount, totalTop5, totalCount)

		report[repo.Key] = map[string]interface{}{
			"label":        repo.Label,
			"chunks":       store.Manifest.TotalChunks,
			"query_count":  totalCount,
			"top1":         totalTop1,
			"top3":         totalTop3,
			"top5":         totalTop5,
			"by_category":  catCount,
			"cat_top1":     catTop1,
			"cat_top3":     catTop3,
			"cat_top5":     catTop5,
		}
	}

	out, _ := json.MarshalIndent(report, "", "  ")
	os.WriteFile("scratch/multi_repo_results.json", out, 0644)
	fmt.Printf("\nwrote scratch/multi_repo_results.json\n")
}
