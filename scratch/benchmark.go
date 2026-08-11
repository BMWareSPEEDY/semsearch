package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	ort "github.com/yalue/onnxruntime_go"
)

type WordPieceTokenizer struct {
	vocab map[string]int64
	unkID int64
	clsID int64
	sepID int64
}

func NewTokenizer(path string) (*WordPieceTokenizer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var root struct {
		Model struct {
			Vocab map[string]int64 `json:"vocab"`
		} `json:"model"`
	}

	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	vocab := root.Model.Vocab
	return &WordPieceTokenizer{
		vocab: vocab,
		unkID: vocab["[UNK]"],
		clsID: vocab["[CLS]"],
		sepID: vocab["[SEP]"],
	}, nil
}

func (t *WordPieceTokenizer) Tokenize(text string, maxLen int) ([]int64, []int64, []int64) {
	text = strings.ToLower(text)
	words := strings.Fields(text)

	inputIDs := []int64{t.clsID}

	for _, word := range words {
		if len(inputIDs) >= maxLen-1 {
			break
		}

		// Basic subword tokenization fallback/match
		if id, ok := t.vocab[word]; ok {
			inputIDs = append(inputIDs, id)
			continue
		}

		// WordPiece splitting algorithm
		end := len(word)
		subwords := []int64{}
		start := 0
		for start < end {
			curEnd := end
			var curTokID int64 = -1

			for start < curEnd {
				substr := word[start:curEnd]
				if start > 0 {
					substr = "##" + substr
				}
				if id, ok := t.vocab[substr]; ok {
					curTokID = id
					break
				}
				curEnd--
			}

			if curTokID == -1 {
				subwords = []int64{t.unkID}
				break
			}
			subwords = append(subwords, curTokID)
			start = curEnd
		}

		for _, subID := range subwords {
			if len(inputIDs) >= maxLen-1 {
				break
			}
			inputIDs = append(inputIDs, subID)
		}
	}

	inputIDs = append(inputIDs, t.sepID)
	length := len(inputIDs)

	attentionMask := make([]int64, maxLen)
	tokenTypeIDs := make([]int64, maxLen)
	paddedIDs := make([]int64, maxLen)

	for i := 0; i < length; i++ {
		paddedIDs[i] = inputIDs[i]
		attentionMask[i] = 1
	}

	return paddedIDs, attentionMask, tokenTypeIDs
}

func main() {
	log.Println("Initializing ONNX Runtime...")
	ort.SetSharedLibraryPath("/opt/homebrew/Cellar/onnxruntime/1.28.0_1/lib/libonnxruntime.dylib")
	err := ort.InitializeEnvironment()
	if err != nil {
		log.Fatalf("Failed to initialize ONNX environment: %v", err)
	}
	defer ort.DestroyEnvironment()

	tokenizer, err := NewTokenizer("scratch/model/tokenizer.json")
	if err != nil {
		log.Fatalf("Failed to load tokenizer: %v", err)
	}
	log.Println("Tokenizer loaded successfully!")

	modelPath := "scratch/model/model_quantized.onnx"
	
	// Prepare test inputs
	sampleTexts := []string{
		"search_query: where do we retry failed API requests with exponential backoff",
		"search_document: func resilientFetch(url string, opts FetchOptions) (*Response, error) { return retry(opts.MaxRetries) }",
		"search_document: func parseJSONConfig(path string) (*Config, error) { return nil, nil }",
		"search_query: database connection pool initialization settings",
	}

	maxSeqLen := 128
	batchSize := len(sampleTexts)

	allIDs := make([]int64, batchSize*maxSeqLen)
	allMask := make([]int64, batchSize*maxSeqLen)
	allTypeIDs := make([]int64, batchSize*maxSeqLen)

	for b, text := range sampleTexts {
		ids, mask, types := tokenizer.Tokenize(text, maxSeqLen)
		copy(allIDs[b*maxSeqLen:(b+1)*maxSeqLen], ids)
		copy(allMask[b*maxSeqLen:(b+1)*maxSeqLen], mask)
		copy(allTypeIDs[b*maxSeqLen:(b+1)*maxSeqLen], types)
	}

	shape := ort.NewShape(int64(batchSize), int64(maxSeqLen))
	
	inputIDsTensor, err := ort.NewTensor(shape, allIDs)
	if err != nil {
		log.Fatalf("Failed to create input_ids tensor: %v", err)
	}
	defer inputIDsTensor.Destroy()

	attentionMaskTensor, err := ort.NewTensor(shape, allMask)
	if err != nil {
		log.Fatalf("Failed to create attention_mask tensor: %v", err)
	}
	defer attentionMaskTensor.Destroy()

	tokenTypeIDsTensor, err := ort.NewTensor(shape, allTypeIDs)
	if err != nil {
		log.Fatalf("Failed to create token_type_ids tensor: %v", err)
	}
	defer tokenTypeIDsTensor.Destroy()

	outputShape := ort.NewShape(int64(batchSize), int64(maxSeqLen), 768)
	outputData := make([]float32, batchSize*maxSeqLen*768)
	outputTensor, err := ort.NewTensor(outputShape, outputData)
	if err != nil {
		log.Fatalf("Failed to create output tensor: %v", err)
	}
	defer outputTensor.Destroy()

	session, err := ort.NewAdvancedSession(
		modelPath,
		[]string{"input_ids", "attention_mask", "token_type_ids"},
		[]string{"last_hidden_state"},
		[]ort.ArbitraryTensor{inputIDsTensor, attentionMaskTensor, tokenTypeIDsTensor},
		[]ort.ArbitraryTensor{outputTensor},
		nil,
	)
	if err != nil {
		log.Fatalf("Failed to create ONNX session: %v", err)
	}
	defer session.Destroy()

	log.Println("Running benchmark inference (10 iterations)...")
	start := time.Now()
	iterations := 10
	for i := 0; i < iterations; i++ {
		err = session.Run()
		if err != nil {
			log.Fatalf("Inference run %d failed: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	
	totalInferences := batchSize * iterations
	perInferenceMs := float64(elapsed.Milliseconds()) / float64(totalInferences)
	
	fmt.Printf("\n=== BENCHMARK RESULTS ===\n")
	fmt.Printf("Total Batch Inferences: %d chunks across %d session runs\n", totalInferences, iterations)
	fmt.Printf("Total Elapsed Time: %v\n", elapsed)
	fmt.Printf("Average Latency per Chunk Embedding: %.2f ms\n", perInferenceMs)
	fmt.Printf("Embedding Throughput: %.1f chunks/sec\n", float64(totalInferences)/elapsed.Seconds())
	fmt.Printf("=========================\n\n")

	// Calculate cosine similarity between query 0 and docs 1 & 2
	embeddings := extractPooledEmbeddings(outputData, allMask, batchSize, maxSeqLen, 768)
	sim01 := cosineSimilarity(embeddings[0], embeddings[1])
	sim02 := cosineSimilarity(embeddings[0], embeddings[2])

	fmt.Printf("Semantic Similarity Test:\n")
	fmt.Printf("Query: '%s'\n", sampleTexts[0])
	fmt.Printf("Doc 1 (Relevant Retry): Similarity Score = %.4f\n", sim01)
	fmt.Printf("Doc 2 (Irrelevant Config): Similarity Score = %.4f\n", sim02)
}

func extractPooledEmbeddings(raw []float32, attentionMask []int64, batchSize, maxSeqLen, hiddenDim int) [][]float32 {
	embeddings := make([][]float32, batchSize)
	for b := 0; b < batchSize; b++ {
		vec := make([]float32, hiddenDim)
		var validTokens float32 = 0.0

		for s := 0; s < maxSeqLen; s++ {
			if attentionMask[b*maxSeqLen+s] == 1 {
				validTokens += 1.0
				offset := (b*maxSeqLen + s) * hiddenDim
				for d := 0; d < hiddenDim; d++ {
					vec[d] += raw[offset+d]
				}
			}
		}

		if validTokens > 0 {
			for d := 0; d < hiddenDim; d++ {
				vec[d] /= validTokens
			}
		}

		// L2 Normalize
		var norm float64 = 0.0
		for d := 0; d < hiddenDim; d++ {
			norm += float64(vec[d] * vec[d])
		}
		norm = math.Sqrt(norm)
		if norm > 0 {
			for d := 0; d < hiddenDim; d++ {
				vec[d] = float32(float64(vec[d]) / norm)
			}
		}

		embeddings[b] = vec
	}
	return embeddings
}

func cosineSimilarity(a, b []float32) float32 {
	var dot float32 = 0.0
	for i := 0; i < len(a); i++ {
		dot += a[i] * b[i]
	}
	return dot
}
