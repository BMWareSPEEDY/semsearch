package indexer

// Keyword-scoring regression tests: the optimized computeKeywordScores uses
// pre-tokenized, sorted per-chunk token data. These tests assert it produces
// bit-identical BM25 + lexical scores to the original per-query tokenization
// algorithm, on synthetic data and (when present) the real external eval repos.

import (
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ashishpaliwal/semsearch/pkg/chunker"
)

// oldComputeKeywordScores is the original algorithm, kept verbatim here as a
// reference implementation.
func oldComputeKeywordScores(s *IndexStore, queryTerms []string, totalChunks int) ([]float32, []float32) {
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
				for _, t := range TokenizeTerms(header) {
					headerTerms[t]++
				}
			} else {
				body = raw
			}
		} else {
			body = raw
		}

		contentTerms := map[string]int{}
		for _, t := range TokenizeTerms(body) {
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

		data[i] = chunkTokenData{
			contentTerms: contentTerms,
			headerTerms:  headerTerms,
			pathTerms:    pathTerms,
			tokenCount:   len(contentTerms),
		}
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

func assertSlicesEqual(t *testing.T, name string, a, b []float32) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("%s: length mismatch %d != %d", name, len(a), len(b))
	}
	for i := range a {
		if math.Float32bits(a[i]) != math.Float32bits(b[i]) {
			t.Errorf("%s: index %d differs: %v != %v", name, i, a[i], b[i])
		}
	}
}

func TestKeywordScoresMatchOldAlgorithmSynthetic(t *testing.T) {
	manifest := Manifest{
		TotalChunks: 6,
		Chunks: []chunker.Chunk{
			{FilePath: "pkg/http/client.go", Content: "// [Package: http | File: client.go | Symbol: resilientFetch | Kind: function]\nresilientFetch retries failed requests with exponential backoff"},
			{FilePath: "pkg/http/client.go", Content: "// [Package: http | File: client.go | Symbol: parseJSON | Kind: function]\nfunc parseJSONConfig path returns config"},
			{FilePath: "auth.go", Content: "func login user token validation checks"},
			{FilePath: "README.md", Content: "# Documentation\nThis file has build and deploy instructions."},
			{FilePath: "config/loader.go", Content: ""},
			{FilePath: "POSITIONAL.go", Content: "handling positional arguments flags parsing"},
		},
	}
	store := &IndexStore{Manifest: manifest}

	queries := []string{
		"retry failed http requests",
		"parsing positional flags",
		"config parse",
		"building documentation",
		"login validation",
	}
	for _, q := range queries {
		terms := TokenizeTerms(q)
		newB, newL := store.computeKeywordScores(terms, manifest.TotalChunks)
		oldB, oldL := oldComputeKeywordScores(store, terms, manifest.TotalChunks)
		assertSlicesEqual(t, "bm25("+q+")", newB, oldB)
		assertSlicesEqual(t, "lex("+q+")", newL, oldL)
	}
}

// TestKeywordScoresMatchOldAlgorithmGrouped uses chunks that share content so
// the df / prefix-leniency paths (and the per-file dedup weighting) get
// exercised with real query terms.
func TestKeywordScoresMatchOldAlgorithmGrouped(t *testing.T) {
	cases := []string{
		"retry", "backoff", "authentication middleware",
		"cancellable context timeout", "filesystem read write",
		"rendering markdown to html", "positional~position",
		"Engine struct definition",
	}
	manifest := buildTinyManifest()
	store := &IndexStore{Manifest: manifest}
	for _, q := range cases {
		terms := TokenizeTerms(q)
		newB, newL := store.computeKeywordScores(terms, manifest.TotalChunks)
		oldB, oldL := oldComputeKeywordScores(store, terms, manifest.TotalChunks)
		assertSlicesEqual(t, "bm25("+q+")", newB, oldB)
		assertSlicesEqual(t, "lex("+q+")", newL, oldL)
	}
}

func buildTinyManifest() Manifest {
	var chunks []chunker.Chunk
	add := func(path, content string) {
		chunks = append(chunks, chunker.Chunk{FilePath: path, Content: content})
	}
	add("pkg/retry/backoff.go", "// [Package: retry | File: backoff.go | Symbol: ExponentialBackoff | Kind: struct]\nfunc next with max retries and backoff interval")
	add("pkg/retry/retry.go", "// [Package: retry | File: retry.go | Symbol: retry | Kind: function]\nfunc retry operation attempts attempt with delay")
	add("pkg/http/middleware/auth.go", "// [Package: http | File: auth.go | Symbol: Authentication | Kind: function]\nauthentication middleware token authorization check")
	add("pkg/http/context.go", "// [Package: http | File: context.go | Symbol: Context | Kind: struct]\ncontext cancellation timeout deadline request")
	add("pkg/fs/io.go", "func read file write bytes filesystem")
	add("docs/markdown.md", "# Markdown\ngoldmark converts markdown content to html rendering")
	add("pkg/engine.go", "Engine struct definition router group engine")
	add("pkg/positional.go", "positional argument flags parser")
	add("pkg/empty.go", "")
	for i := range chunks {
		chunks[i].ID = i
	}
	return Manifest{TotalChunks: len(chunks), Chunks: chunks}
}

// TestKeywordScoresMatchOldAlgorithmRealRepos compares against the real external
// eval indexes if they are present on disk (they are gitignored clones).
func TestKeywordScoresMatchOldAlgorithmRealRepos(t *testing.T) {
	queries := []string{
		"retry failed api requests with backoff",
		"Engine struct definition",
		"handling integer parsing errors",
		"converting markdown content to html",
		"discovering content files on disk",
	}
	for _, repo := range []string{"gin", "cobra", "mux", "clap", "hugo"} {
		dir := filepath.Join("..", "..", "scratch", "external_eval", repo)
		if _, err := os.Stat(filepath.Join(dir, ".semsearch", "manifest.json")); err != nil {
			t.Logf("skipping %s: no local index at %s", repo, dir)
			continue
		}
		store := NewIndexStore(dir)
		if err := store.Load(); err != nil {
			t.Fatalf("load %s: %v", repo, err)
		}
		total := store.Manifest.TotalChunks
		if total == 0 {
			continue
		}
		// Warm up the cache once, then compare per query against a fresh,
		// cache-less store built from the same manifest.
		store.computeKeywordScores([]string{"warmup"}, total)
		for _, q := range queries {
			terms := TokenizeTerms(q)
			if len(terms) == 0 {
				continue
			}
			// fresh store (no cached keyword index) for the old algorithm
			fresh := &IndexStore{Manifest: store.Manifest, Vectors: store.Vectors, DescVectors: store.DescVectors}
			newB, newL := store.computeKeywordScores(terms, total)
			oldB, oldL := oldComputeKeywordScores(fresh, terms, total)
			assertSlicesEqual(t, repo+":bm25("+q+")", newB, oldB)
			assertSlicesEqual(t, repo+":lex("+q+")", newL, oldL)
		}
	}
}

// TestHybridSearchCacheConsistency ensures that searches before and after the
// keyword cache is built return identical rankings.
func TestHybridSearchCacheConsistency(t *testing.T) {
	manifest := buildTinyManifest()
	dim := 4
	manifest.VectorDimensions = dim
	store := &IndexStore{Manifest: manifest, Vectors: make([]float32, manifest.TotalChunks*dim)}
	// arbitrary unit-ish vectors
	for i := range store.Vectors {
		store.Vectors[i] = float32(i+1) / float32(manifest.TotalChunks*dim)
	}
	q := []float32{0.5, 0.5, 0.5, 0.5}
	a := store.HybridSearch(q, "authentication middleware", 5, 0.0, 0.65, false, false)
	b := store.HybridSearch(q, "authentication middleware", 5, 0.0, 0.65, false, false)
	if len(a) != len(b) {
		t.Fatalf("result count changed across cached runs: %d != %d", len(a), len(b))
	}
	for i := range a {
		if math.Float32bits(a[i].Score) != math.Float32bits(b[i].Score) || a[i].Chunk.FilePath != b[i].Chunk.FilePath {
			t.Errorf("rank %d changed: %s:%v != %s:%v", i, a[i].Chunk.FilePath, a[i].Score, b[i].Chunk.FilePath, b[i].Score)
		}
	}
}

func TestChunkTokensHelper(t *testing.T) {
	ch := chunker.Chunk{
		FilePath: "dir/Client.go",
		Content:  "// [Package: http | File: Client.go | Symbol: Client | Kind: struct]\nfunc New Client connects to remote server",
	}
	ct := buildChunkTokens(&ch)
	if ct.base != "client.go" {
		t.Errorf("base = %q", ct.base)
	}
	// content tokens must be sorted unique
	for i := 1; i < len(ct.content); i++ {
		if ct.content[i-1] >= ct.content[i] {
			t.Fatalf("content tokens not sorted: %v", ct.content)
		}
	}
	if ct.uniqueCnt != int32(len(ct.content)) {
		t.Errorf("uniqueCnt %d != len %d", ct.uniqueCnt, len(ct.content))
	}
	if i := sort.SearchStrings(ct.header, "client"); !(i < len(ct.header) && ct.header[i] == "client") {
		t.Errorf("header tokens should contain client, got %v", ct.header)
	}
}