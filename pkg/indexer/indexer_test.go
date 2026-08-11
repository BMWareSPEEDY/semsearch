package indexer_test

import (
	"math"
	"testing"
	"time"

	"github.com/ashishpaliwal/semsearch/pkg/chunker"
	"github.com/ashishpaliwal/semsearch/pkg/indexer"
)

func TestIndexerSaveLoadSearch(t *testing.T) {
	tempDir := t.TempDir()
	store := indexer.NewIndexStore(tempDir)

	manifest := indexer.Manifest{
		Version:          "1.0",
		CreatedAt:        time.Now(),
		Model:            "nomic-embed-text-v1.5",
		VectorDimensions: 4,
		TotalChunks:      2,
		Chunks: []chunker.Chunk{
			{ID: 0, FilePath: "a.go", Content: "func a()"},
			{ID: 1, FilePath: "b.go", Content: "func b()"},
		},
	}

	// Vectors: 2 chunks, 4 dimensions each (L2 normalized)
	vectors := []float32{
		1.0, 0.0, 0.0, 0.0, // Vector 0
		0.0, 1.0, 0.0, 0.0, // Vector 1
	}

	if err := store.Save(manifest, vectors); err != nil {
		t.Fatalf("Failed to save index: %v", err)
	}

	loadedStore := indexer.NewIndexStore(tempDir)
	if err := loadedStore.Load(); err != nil {
		t.Fatalf("Failed to load index: %v", err)
	}

	if loadedStore.Manifest.TotalChunks != 2 {
		t.Errorf("Expected 2 chunks, got %d", loadedStore.Manifest.TotalChunks)
	}

	// Query identical to Vector 0
	query := []float32{1.0, 0.0, 0.0, 0.0}
	results := loadedStore.HybridSearch(query, "", 10, 0.0, 1.0, false, false)

	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	if results[0].Chunk.FilePath != "a.go" || results[0].Score != 1.0 {
		t.Errorf("Expected top result a.go with score 1.0, got %s score %f", results[0].Chunk.FilePath, results[0].Score)
	}
}

// TestScoreChunkMaxBlend verifies the explicit design decision: a strong match on
// EITHER the code vector or the description vector wins via max, not average.
func TestScoreChunkMaxBlend(t *testing.T) {
	// query is orthogonal to the code vector (dot=0) but identical to the desc vector
	query := []float32{0.0, 0.0, 1.0, 0.0}
	codeVec := []float32{1.0, 0.0, 0.0, 0.0}
	descVec := []float32{0.0, 0.0, 1.0, 0.0}

	chunk := indexer.IndexedChunk{
		CodeVector: codeVec,
		DescVector: descVec,
	}

	score := indexer.ScoreChunk(query, chunk, 0.0)
	// max(codeSim=0, descSim=1) = 1.0 -> blended with weight 0.6 => 0.6
	want := 0.6 * 1.0
	if math.Abs(score-want) > 1e-5 {
		t.Errorf("expected blended score %.3f (max, not average), got %.3f", want, score)
	}

	// Confirm average would have scored 0.5*semanticWeight — the max choice must be higher
	averageSemantic := (0.0 + 1.0) / 2.0
	if score <= 0.6*averageSemantic {
		t.Errorf("max-blend score %f should exceed average-blend %f", score, 0.6*averageSemantic)
	}
}

// TestScoreChunkNoDescVector verifies single-vector fallback does not panic.
func TestScoreChunkNoDescVector(t *testing.T) {
	query := []float32{1.0, 0.0, 0.0, 0.0}
	chunk := indexer.IndexedChunk{CodeVector: []float32{1.0, 0.0, 0.0, 0.0}}
	score := indexer.ScoreChunk(query, chunk, 0.0)
	want := 0.6
	if math.Abs(score-want) > 1e-5 {
		t.Errorf("expected %.3f, got %.3f", want, score)
	}
}
