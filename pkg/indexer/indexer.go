package indexer

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ashishpaliwal/semsearch/pkg/chunker"
)

type Manifest struct {
	Version           string            `json:"version"`
	CreatedAt         time.Time         `json:"created_at"`
	Model             string            `json:"model"`
	VectorDimensions  int               `json:"vector_dimensions"`
	TotalChunks       int               `json:"total_chunks"`
	IndexedFilesCount int               `json:"indexed_files_count"`
	FileHashes        map[string]string `json:"file_hashes"`
	Chunks            []chunker.Chunk   `json:"chunks"`
}

type SearchResult struct {
	Chunk chunker.Chunk `json:"chunk"`
	Score float32       `json:"score"`
}

type IndexStore struct {
	rootDir     string
	indexDir    string
	Manifest    Manifest
	Vectors     []float32 // Code vectors: TotalChunks * VectorDimensions
	DescVectors []float32 // Description vectors: TotalChunks * VectorDimensions (optional)
}

func NewIndexStore(rootDir string) *IndexStore {
	return &IndexStore{
		rootDir:  rootDir,
		indexDir: filepath.Join(rootDir, ".semsearch"),
	}
}

func (s *IndexStore) SaveDual(manifest Manifest, codeVectors, descVectors []float32) error {
	if err := os.MkdirAll(s.indexDir, 0755); err != nil {
		return err
	}

	manifestPath := filepath.Join(s.indexDir, "manifest.json")
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(manifestPath, manifestData, 0644); err != nil {
		return err
	}

	// Write code vectors
	if err := writeVectorFile(filepath.Join(s.indexDir, "vectors.bin"), codeVectors); err != nil {
		return err
	}

	// Write description vectors if present
	if len(descVectors) > 0 {
		if err := writeVectorFile(filepath.Join(s.indexDir, "desc_vectors.bin"), descVectors); err != nil {
			return err
		}
	}

	s.Manifest = manifest
	s.Vectors = codeVectors
	s.DescVectors = descVectors
	return nil
}

func (s *IndexStore) Save(manifest Manifest, vectors []float32) error {
	return s.SaveDual(manifest, vectors, nil)
}

func writeVectorFile(path string, vectors []float32) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	buf := make([]byte, 4)
	for _, val := range vectors {
		bits := math.Float32bits(val)
		binary.LittleEndian.PutUint32(buf, bits)
		if _, err := file.Write(buf); err != nil {
			return err
		}
	}
	return nil
}

func readVectorFile(path string, totalFloats int) ([]float32, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	vectors := make([]float32, totalFloats)
	buf := make([]byte, 4)

	for i := 0; i < totalFloats; i++ {
		if _, err := io.ReadFull(file, buf); err != nil {
			return nil, fmt.Errorf("corrupted vector binary store at index %d: %w", i, err)
		}
		bits := binary.LittleEndian.Uint32(buf)
		vectors[i] = math.Float32frombits(bits)
	}
	return vectors, nil
}

func (s *IndexStore) Load() error {
	manifestPath := filepath.Join(s.indexDir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read index manifest: %w", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return fmt.Errorf("corrupted index manifest: %w", err)
	}

	totalFloats := manifest.TotalChunks * manifest.VectorDimensions
	codeVecs, err := readVectorFile(filepath.Join(s.indexDir, "vectors.bin"), totalFloats)
	if err != nil {
		return fmt.Errorf("failed to open code vector binary store: %w", err)
	}

	var descVecs []float32
	descPath := filepath.Join(s.indexDir, "desc_vectors.bin")
	if _, errStat := os.Stat(descPath); errStat == nil {
		descVecs, _ = readVectorFile(descPath, totalFloats)
	}

	s.Manifest = manifest
	s.Vectors = codeVecs
	s.DescVectors = descVecs
	return nil
}

func (s *IndexStore) UpdateFileChunks(filePath, fileHash string, newChunks []chunker.Chunk, newVectors []float32) error {
	return s.UpdateFileChunksDual(filePath, fileHash, newChunks, newVectors, nil)
}

// UpdateFileChunksDual replaces the chunks belonging to filePath with newChunks
// and their two embedding vectors (code + description). Description vectors for
// kept chunks are preserved; if newDescVectors is nil, description vectors are
// dropped and the index degrades to single-vector mode.
func (s *IndexStore) UpdateFileChunksDual(filePath, fileHash string, newChunks []chunker.Chunk, newCodeVectors, newDescVectors []float32) error {
	dim := s.Manifest.VectorDimensions

	var keptChunks []chunker.Chunk
	var keptCodeVecs []float32
	var keptDescVecs []float32
	hasDescVectors := len(s.DescVectors) == len(s.Vectors)

	for i, ch := range s.Manifest.Chunks {
		if ch.FilePath != filePath {
			keptChunks = append(keptChunks, ch)
			vecOffset := i * dim
			keptCodeVecs = append(keptCodeVecs, s.Vectors[vecOffset:vecOffset+dim]...)
			if hasDescVectors && len(newDescVectors) > 0 {
				keptDescVecs = append(keptDescVecs, s.DescVectors[vecOffset:vecOffset+dim]...)
			}
		}
	}

	for i := range newChunks {
		newChunks[i].ID = len(keptChunks)
		keptChunks = append(keptChunks, newChunks[i])
	}
	keptCodeVecs = append(keptCodeVecs, newCodeVectors...)
	keptDescVecs = append(keptDescVecs, newDescVectors...)

	if s.Manifest.FileHashes == nil {
		s.Manifest.FileHashes = make(map[string]string)
	}
	s.Manifest.FileHashes[filePath] = fileHash
	s.Manifest.TotalChunks = len(keptChunks)
	s.Manifest.CreatedAt = time.Now()
	s.Manifest.Chunks = keptChunks

	if len(keptDescVecs) > 0 {
		return s.SaveDual(s.Manifest, keptCodeVecs, keptDescVecs)
	}
	return s.Save(s.Manifest, keptCodeVecs)
}

func (s *IndexStore) Search(queryVector []float32, topN int, minScore float32) []SearchResult {
	return s.HybridSearch(queryVector, "", topN, minScore, 0.5, false, false)
}

func (s *IndexStore) HybridSearch(queryVector []float32, queryText string, topN int, minScore float32, semanticWeight float32, codeOnly, docsOnly bool) []SearchResult {
	if s.Manifest.TotalChunks == 0 || len(s.Vectors) == 0 {
		return nil
	}

	// Allow tuning the fusion weight without recompiling callers.
	if v := os.Getenv("SEMSEARCH_SEM_WEIGHT"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 32); err == nil && parsed >= 0 && parsed <= 1 {
			semanticWeight = float32(parsed)
		}
	}
	if v := os.Getenv("SEMSEARCH_FUSION"); v != "" {
		return s.hybridSearchVariant(queryVector, queryText, topN, minScore, semanticWeight, codeOnly, docsOnly, v)
	}

	semanticScores, keywordScores, lexicalScores := s.computeAllScores(queryVector, queryText, codeOnly, docsOnly)

	// 3. Blended Score
	totalChunks := s.Manifest.TotalChunks
	results := make([]SearchResult, 0, totalChunks)
	for i := 0; i < totalChunks; i++ {
		blendedScore := blendHybrid(semanticScores[i], keywordScores[i], lexicalScores[i], semanticWeight)

		if blendedScore >= minScore {
			results = append(results, SearchResult{
				Chunk: s.Manifest.Chunks[i],
				Score: blendedScore,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topN {
		results = results[:topN]
	}

	return results
}

// computeAllScores returns semantic cosine scores plus the normalized BM25 and
// lexical scores for every chunk. Reused by the fusion variants.
func (s *IndexStore) computeAllScores(queryVector []float32, queryText string, codeOnly, docsOnly bool) ([]float32, []float32, []float32) {
	dim := s.Manifest.VectorDimensions
	totalChunks := s.Manifest.TotalChunks
	queryTerms := TokenizeTerms(queryText)

	// 1. Calculate Semantic Cosine Scores with Doc Gravitational Pull Dampening
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
		semanticScores[i] = scoreSemantic(queryVector, chunkVector, descVector)
	}

	// 2. Calculate Keyword BM25 scores (tokenized, stopword-filtered) plus
	// lexical signal from symbol names and file paths.
	keywordScores := make([]float32, totalChunks)
	lexicalScores := make([]float32, totalChunks)
	if len(queryTerms) > 0 {
		keywordScores, lexicalScores = s.computeKeywordScores(queryTerms, totalChunks)
	}
	return semanticScores, keywordScores, lexicalScores
}

// hybridSearchVariant applies an alternative fusion strategy (used for tuning).
// Strategies: "rrf" (reciprocal rank fusion over semantic + keyword rank),
// "sem_first" (semantic primary, keyword as capped boost), "mult" (multiplicative).
func (s *IndexStore) hybridSearchVariant(queryVector []float32, queryText string, topN int, minScore float32, semanticWeight float32, codeOnly, docsOnly bool, strategy string) []SearchResult {
	totalChunks := s.Manifest.TotalChunks
	sem, kw, lex := s.computeAllScores(queryVector, queryText, codeOnly, docsOnly)
	final := make([]float32, totalChunks)
	excluded := make([]bool, totalChunks)

	switch strategy {
	case "rrf":
		// Reciprocal rank fusion over semantic rank and combined keyword rank.
		semRank := rankIndices(sem)
		kwRank := rankIndices(addSlices(kw, lex))
		for i := 0; i < totalChunks; i++ {
			if sem[i] < 0 {
				final[i] = -1
				excluded[i] = true
				continue
			}
			final[i] = float32(1.0/float64(60+semRank[i]) + 1.0/float64(60+kwRank[i]))
		}
	case "mult":
		for i := 0; i < totalChunks; i++ {
			if sem[i] < 0 {
				final[i] = -1
				excluded[i] = true
				continue
			}
			// Semantic is primary; keyword acts as a multiplicative boost so it
			// can only amplify, never override, a decent semantic match.
			boost := 1.0 + 0.8*float64(kw[i]) + 0.6*float64(lex[i])
			final[i] = float32(math.Max(0, float64(sem[i])) * boost)
		}
	case "avg":
		for i := 0; i < totalChunks; i++ {
			if sem[i] < 0 {
				final[i] = -1
				excluded[i] = true
				continue
			}
			final[i] = 0.7*sem[i] + 0.3*kw[i]
		}
	default:
		return s.HybridSearch(queryVector, queryText, topN, minScore, semanticWeight, codeOnly, docsOnly)
	}

	results := make([]SearchResult, 0, totalChunks)
	for i := 0; i < totalChunks; i++ {
		if excluded[i] || final[i] < minScore {
			continue
		}
		results = append(results, SearchResult{
			Chunk: s.Manifest.Chunks[i],
			Score: final[i],
		})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if len(results) > topN {
		results = results[:topN]
	}
	return results
}

func rankIndices(scores []float32) []int {
	rank := make([]int, len(scores))
	for i := range scores {
		rank[i] = i
	}
	sort.Slice(rank, func(a, b int) bool {
		if scores[rank[a]] == scores[rank[b]] {
			return rank[a] < rank[b]
		}
		return scores[rank[a]] > scores[rank[b]]
	})
	out := make([]int, len(scores))
	for pos, idx := range rank {
		out[idx] = pos
	}
	return out
}

func addSlices(a, b []float32) []float32 {
	out := make([]float32, len(a))
	for i := range a {
		out[i] = a[i] + b[i]
	}
	return out
}

// computeKeywordScores returns, per chunk, a normalized BM25 score over the
// code content and a normalized lexical score combining symbol-name and
// file-path term matches. Both are in [0,1].
func (s *IndexStore) computeKeywordScores(queryTerms []string, totalChunks int) ([]float32, []float32) {
	bm25Scores := make([]float32, totalChunks)
	lexicalScores := make([]float32, totalChunks)

	// Pre-tokenize each chunk once. contentTerms counts full matches in the
	// raw body; headerTerms counts matches in the structural metadata header
	// (which carries package/file/symbol context).
	type chunkTokenData struct {
		contentTerms map[string]int
		headerTerms  map[string]int
		pathTerms    map[string]int
		tokenCount   int
	}
	data := make([]chunkTokenData, totalChunks)

	// df counts chunks containing each term (in content OR header OR path).
	df := make(map[string]int)
	avgTokens := 0.0
	tokenDocs := 0

	for i := 0; i < totalChunks; i++ {
		ch := &s.Manifest.Chunks[i]
		raw := ch.Content

		// Isolate the structural metadata header, if present.
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

	// BM25 with k1/b constants; header and path terms are weighted higher so an
	// exact symbol or filename match outranks a scattered keyword coincidence.
	// Term matching uses prefix leniency (positional~position, flags~flag) so
	// natural-language query words still hit code/doc-comment words.
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
			contentTf := countTermMatches(td.contentTerms, term)
			headerTf := countTermMatches(td.headerTerms, term)
			pathTf := float64(td.pathTerms[term])

			if contentTf == 0 && headerTf == 0 && pathTf == 0 {
				continue
			}
			docFreq := float64(df[term])
			idf := math.Log((float64(totalChunks)-docFreq+0.5)/(docFreq+0.5) + 1.0)

			// Weighted term frequency: header (symbol/pkg) terms dominate.
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

	// Normalize both to [0,1].
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

// countTermMatches returns the summed frequency of tokens that either equal the
// query term or, for terms of length >= 4, share a long-enough prefix (so
// "positional" matches "position", "flags" matches "flag"). Prefix leniency is
// limited to avoid the noisy substring matches of the old implementation.
func countTermMatches(tokens map[string]int, term string) float64 {
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

// TokenizeTerms lowercases text and splits it into alphanumeric/underscore
// tokens, dropping English stopwords and tokens shorter than 2 characters.
func TokenizeTerms(s string) []string {
	var terms []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() >= 2 && !isStopword(cur.String()) {
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

func isStopword(w string) bool {
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

func cosineSimilaritySIMD(a, b []float32) float32 {
	var dot float32 = 0.0
	for i := 0; i < len(a); i++ {
		dot += a[i] * b[i]
	}
	return dot
}

// scoreSemantic compares the query vector against both the raw code vector and
// the description vector, taking the max rather than the average. A query may
// legitimately match the code's literal structure (exact symbol) OR its described
// purpose (conceptual); max lets each axis contribute independently instead of
// diluting a strong match on one axis with a weak match on the other.
func scoreSemantic(queryVec, codeVec, descVec []float32) float32 {
	codeSim := cosineSimilaritySIMD(queryVec, codeVec)
	if descVec == nil {
		return codeSim
	}
	descSim := cosineSimilaritySIMD(queryVec, descVec)
	return float32(math.Max(float64(codeSim), float64(descSim)))
}

// ScoreChunk is the full per-chunk scoring used by the search path: semantic
// max-blend against the query, blended with the BM25 keyword score.
func ScoreChunk(queryVec []float32, chunk IndexedChunk, bm25Score float64) float64 {
	codeSim := cosineSimilaritySIMD(queryVec, chunk.CodeVector)
	descSim := codeSim
	if chunk.DescVector != nil {
		descSim = cosineSimilaritySIMD(queryVec, chunk.DescVector)
	}

	// take the max, not the average — a strong match on either axis is a real match
	semanticScore := math.Max(float64(codeSim), float64(descSim))

	return float64(blendHybrid(float32(semanticScore), float32(bm25Score), 0, 0.6))
}

// blendHybrid combines semantic, BM25 keyword and lexical signal scores into a
// single ranking score. semanticWeight controls how much semantic similarity
// matters relative to the keyword + lexical signals.
func blendHybrid(semanticScore, bm25Score, lexicalScore float32, semanticWeight float32) float32 {
	return (semanticWeight * semanticScore) + ((1.0 - semanticWeight) * (0.6*bm25Score + 0.4*lexicalScore))
}

// IndexedChunk is a chunk plus its two per-chunk embedding vectors (code + description).
type IndexedChunk struct {
	ChunkID     string
	File        string
	LineStart   int
	LineEnd     int
	Text        string
	CodeVector  []float32 // existing: embedding of the raw code (+ metadata prefix)
	DescVector  []float32 // new: embedding of the generated description
	Description string    // store the text too, useful for debugging/display
}
