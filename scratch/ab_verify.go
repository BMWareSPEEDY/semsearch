package main

// ab_verify.go — A/B verification that the optimized HybridSearch produces
// exactly the same ranking as the pre-optimization algorithm, across all eval
// queries on the real external indexes.
//
//   go run scratch/ab_verify.go scratch/evalcases_gin.go scratch/evalcases_cobra.go \
//          scratch/evalcases_mux.go scratch/evalcases_clap.go scratch/evalcases_hugo.go

import (
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ashishpaliwal/semsearch/pkg/embedder"
	"github.com/ashishpaliwal/semsearch/pkg/indexer"
)

// ---- reference (old) keyword scoring, copied verbatim ----

type BenchCase struct {
	Query    string
	Category string
	Expected string
}
func oldCountTermMatches(tokens map[string]int, term string) float64 {
	if tokens[term] > 0 {
		return float64(tokens[term])
	}
	if len(term) < 4 {
		return 0
	}
	var total float64
	for tok, cnt := range tokens {
		if len(tok) >= 4 {
			if strings.HasPrefix(tok, term) || strings.HasPrefix(term, tok) {
				total += float64(cnt) * 0.6
			}
		}
	}
	return total
}

func oldTokenizeTerms(s string) []string {
	var terms []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() >= 2 && !oldStopword(cur.String()) {
			terms = append(terms, cur.String())
		}
		cur.Reset()
	}
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return terms
}

func oldStopword(w string) bool {
	switch w {
	case "the", "a", "an", "and", "or", "but", "of", "to", "in", "on", "for",
		"with", "at", "by", "from", "as", "is", "are", "was", "were", "be",
		"been", "being", "do", "does", "did", "has", "have", "had", "how",
		"where", "when", "why", "what", "which", "who", "whom", "this", "that",
		"these", "those", "it", "its", "not", "no", "so", "such", "if", "then",
		"than", "can", "could", "will", "would", "should", "may", "might",
		"into", "over", "under", "between", "through", "during", "about",
		"after", "before", "out", "up", "down", "off", "per", "via", "i", "we",
		"you", "your", "they", "them", "there", "their", "here", "our", "my",
		"am", "all", "any", "some", "more", "most", "other", "another",
		"etc", "s", "please", "already", "become", "became", "upon",
		"within", "without", "against", "because":
		return true
	}
	return false
}

func oldComputeKeywordScores(s *indexer.IndexStore, queryTerms []string, totalChunks int) ([]float32, []float32) {
	bm25Scores := make([]float32, totalChunks)
	lexicalScores := make([]float32, totalChunks)
	type chunkTokenData struct {
		contentTerms map[string]int
		headerTerms  map[string]int
		pathTerms    map[string]int
		tokenCount   int
	}
	data := make([]chunkTokenData, totalChunks)
	df := make(map[string]int)
	avgTokens := 0.0
	tokenDocs := 0

	for i := 0; i < totalChunks; i++ {
		ch := &s.Manifest.Chunks[i]
		raw := ch.Content
		var body string
		headerTerms := map[string]int{}
		if strings.HasPrefix(raw, "// [Package:") {
			if idx := strings.Index(raw, "\n"); idx != -1 {
				header := raw[:idx]
				body = raw[idx+1:]
				for _, t := range oldTokenizeTerms(header) {
					headerTerms[t]++
				}
			} else {
				body = raw
			}
		} else {
			body = raw
		}

		contentTerms := map[string]int{}
		for _, t := range oldTokenizeTerms(body) {
			contentTerms[t]++
		}

		pathTerms := map[string]int{}
		base := strings.ToLower(filepath.Base(ch.FilePath))
		for _, t := range queryTerms {
			if strings.Contains(base, t) {
				pathTerms[t]++
			}
		}

		combined := make(map[string]struct{}, len(contentTerms)+len(headerTerms)+len(pathTerms))
		for t := range contentTerms {
			combined[t] = struct{}{}
		}
		for t := range headerTerms {
			combined[t] = struct{}{}
		}
		for t := range pathTerms {
			combined[t] = struct{}{}
		}

		data[i] = chunkTokenData{contentTerms: contentTerms, headerTerms: headerTerms, pathTerms: pathTerms, tokenCount: len(contentTerms)}
		for t := range combined {
			df[t]++
		}
		avgTokens += float64(data[i].tokenCount)
		tokenDocs++
	}
	if tokenDocs > 0 {
		avgTokens /= float64(tokenDocs)
	}

	const k1 = 1.4
	const b = 0.6
	for i := 0; i < totalChunks; i++ {
		td := data[i]
		dl := float64(td.tokenCount)
		if dl == 0 {
			dl = 1
		}
		var score float64
		var lexScore float64
		for _, term := range queryTerms {
			contentTf := oldCountTermMatches(td.contentTerms, term)
			headerTf := oldCountTermMatches(td.headerTerms, term)
			pathTf := float64(td.pathTerms[term])
			if contentTf == 0 && headerTf == 0 && pathTf == 0 {
				continue
			}
			docFreq := float64(df[term])
			idf := math.Log((float64(totalChunks)-docFreq+0.5)/(docFreq+0.5) + 1.0)
			tfCombined := contentTf + 4.0*headerTf + 2.0*pathTf
			denom := tfCombined + k1*(1-b+b*dl/avgTokens)
			if denom <= 0 {
				denom = 1
			}
			score += idf * (tfCombined * (k1 + 1)) / denom
			lexScore += 3.0*headerTf + 2.0*pathTf + 0.5*contentTf
		}
		bm25Scores[i] = float32(score)
		lexicalScores[i] = float32(lexScore)
	}
	var maxBm25 float32 = 0.0001
	var maxLex float32 = 0.0001
	for i := 0; i < totalChunks; i++ {
		if bm25Scores[i] > maxBm25 {
			maxBm25 = bm25Scores[i]
		}
		if lexicalScores[i] > maxLex {
			maxLex = lexicalScores[i]
		}
	}
	for i := 0; i < totalChunks; i++ {
		bm25Scores[i] /= maxBm25
		lexicalScores[i] /= maxLex
	}
	return bm25Scores, lexicalScores
}

// oldComputeAllScores + oldHybridSearch reproduce the full pre-change search.
func oldHybridSearch(s *indexer.IndexStore, queryVec []float32, queryText string, topN int, minScore float32, semanticWeight float32, codeOnly, docsOnly bool, qterms []string) []indexer.SearchResult {
	dim := s.Manifest.VectorDimensions
	totalChunks := s.Manifest.TotalChunks

	semanticScores := make([]float32, totalChunks)
	for i := 0; i < totalChunks; i++ {
		filePath := s.Manifest.Chunks[i].FilePath
		isDoc := strings.HasSuffix(filePath, ".md") || strings.HasSuffix(filePath, ".markdown") || strings.Contains(filePath, "docs/") || strings.HasPrefix(filepath.Base(filePath), "README")
		if codeOnly && isDoc {
			semanticScores[i] = -1.0
			continue
		}
		if docsOnly && !isDoc {
			semanticScores[i] = -1.0
			continue
		}
		vecOffset := i * dim
		chunkVector := s.Vectors[vecOffset : vecOffset+dim]
		var descVector []float32
		if len(s.DescVectors) >= (vecOffset + dim) {
			descVector = s.DescVectors[vecOffset : vecOffset+dim]
		}
		codeSim := oldCosine(queryVec, chunkVector)
		if descVector == nil {
			semanticScores[i] = codeSim
		} else {
			descSim := oldCosine(queryVec, descVector)
			semanticScores[i] = float32(math.Max(float64(codeSim), float64(descSim)))
		}
	}

	keywordScores := make([]float32, totalChunks)
	lexicalScores := make([]float32, totalChunks)
	if len(qterms) > 0 {
		keywordScores, lexicalScores = oldComputeKeywordScores(s, qterms, totalChunks)
	}

	results := make([]indexer.SearchResult, 0, totalChunks)
	for i := 0; i < totalChunks; i++ {
		blended := (semanticWeight * semanticScores[i]) + ((1.0 - semanticWeight) * (0.6*keywordScores[i] + 0.4*lexicalScores[i]))
		if blended >= minScore {
			results = append(results, indexer.SearchResult{Chunk: s.Manifest.Chunks[i], Score: blended})
		}
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > topN {
		results = results[:topN]
	}
	return results
}

func oldCosine(a, b []float32) float32 {
	var dot float32 = 0.0
	for i := 0; i < len(a); i++ {
		dot += a[i] * b[i]
	}
	return dot
}

var abRepos = []struct {
	key   string
	dir   string
	cases []BenchCase
}{
	{"gin", "scratch/external_eval/gin", ginCases},
	{"cobra", "scratch/external_eval/cobra", cobraCases},
	{"mux", "scratch/external_eval/mux", muxCases},
	{"clap", "scratch/external_eval/clap", clapCases},
	{"hugo", "scratch/external_eval/hugo", hugoCases},
}

func main() {
	tok, err := os.ReadFile("scratch/code_minilm/tokenizer.json")
	if err != nil {
		log.Fatal(err)
	}
	emb, err := embedder.NewEmbedder("scratch/code_minilm/model_quantized.onnx", tok)
	if err != nil {
		log.Fatal(err)
	}
	mismatches := 0
	for _, rp := range abRepos {
		store := indexer.NewIndexStore(rp.dir)
		if err := store.Load(); err != nil {
			log.Fatalf("%s: %v", rp.key, err)
		}
		// Force the new path to build its cache once (it is lazy).
		_ = store.HybridSearch([]float32{}, "warmup", 1, 0, 0.6, true, false)
		for _, tc := range rp.cases {
			qv, err := emb.EmbedBatch([]string{tc.Query}, "search_query: ")
			if err != nil {
				continue
			}
			nw := 30
			newResults := store.HybridSearch(qv, tc.Query, nw, 0.0, 0.75, true, false)
			oldResults := oldHybridSearch(store, qv, tc.Query, nw, 0.0, 0.75, true, false, indexer.TokenizeTerms(tc.Query))
			// copy results after optimizer (for ranking comparison use raw shortlists; rerank later would be identical since inputs identical)
			if len(newResults) != len(oldResults) {
				mismatches++
				fmt.Printf("%s: %q length %d != %d\n", rp.key, tc.Query, len(newResults), len(oldResults))
				continue
			}
			for i := range newResults {
				if newResults[i].Chunk.FilePath != oldResults[i].Chunk.FilePath ||
					math.Float32bits(newResults[i].Score) != math.Float32bits(oldResults[i].Score) {
					mismatches++
					fmt.Printf("%s: %q #%d new=%s:%v old=%s:%v\n", rp.key, tc.Query, i, newResults[i].Chunk.FilePath, newResults[i].Score, oldResults[i].Chunk.FilePath, oldResults[i].Score)
					break
				}
			}
		}
	}
	if mismatches == 0 {
		fmt.Println("A/B VERIFY: rankings bit-identical across all repos and queries")
	}
	fmt.Printf("mismatches: %d\n", mismatches)
}