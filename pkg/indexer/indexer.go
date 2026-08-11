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

	dim := s.Manifest.VectorDimensions
	totalChunks := s.Manifest.TotalChunks
	queryTerms := strings.Fields(strings.ToLower(queryText))

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

	// 2. Calculate Keyword BM25/TF-IDF Scores
	keywordScores := make([]float32, totalChunks)
	if len(queryTerms) > 0 {
		docFreq := make(map[string]int)
		for _, term := range queryTerms {
			for i := 0; i < totalChunks; i++ {
				rawContent := s.Manifest.Chunks[i].Content
				// Strip structural metadata header comments for pure BM25 keyword scoring so exact matches are not diluted
				if strings.HasPrefix(rawContent, "// [Package:") {
					if idx := strings.Index(rawContent, "\n"); idx != -1 {
						rawContent = rawContent[idx+1:]
					}
				}

				if strings.Contains(strings.ToLower(rawContent), term) {
					docFreq[term]++
				}
			}
		}

		for i := 0; i < totalChunks; i++ {
			rawContent := s.Manifest.Chunks[i].Content
			if strings.HasPrefix(rawContent, "// [Package:") {
				if idx := strings.Index(rawContent, "\n"); idx != -1 {
					rawContent = rawContent[idx+1:]
				}
			}

			contentLower := strings.ToLower(rawContent)
			var tfidf float32 = 0.0
			for _, term := range queryTerms {
				tf := float32(strings.Count(contentLower, term))
				if tf > 0 {
					df := docFreq[term]
					idf := float32(math.Log(float64(totalChunks+1) / float64(df+1)))
					tfidf += tf * idf
				}
			}
			keywordScores[i] = tfidf
		}

		// Normalize keyword scores to [0, 1] range
		var maxKw float32 = 0.0001
		for _, kw := range keywordScores {
			if kw > maxKw {
				maxKw = kw
			}
		}
		for i := 0; i < totalChunks; i++ {
			keywordScores[i] /= maxKw
		}
	}

	// 3. Reciprocal Rank Fusion / Blended Score
	results := make([]SearchResult, 0, totalChunks)
	for i := 0; i < totalChunks; i++ {
		blendedScore := blendHybrid(semanticScores[i], keywordScores[i], semanticWeight)

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

	return float64(blendHybrid(float32(semanticScore), float32(bm25Score), 0.6))
}

// blendHybrid combines semantic and BM25 keyword scores into a single ranking score.
func blendHybrid(semanticScore, bm25Score float32, semanticWeight float32) float32 {
	return (semanticWeight * semanticScore) + ((1.0 - semanticWeight) * bm25Score)
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
