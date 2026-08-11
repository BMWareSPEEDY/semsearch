package reranker

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
	"github.com/ashishpaliwal/semsearch/pkg/indexer"
)

type CrossEncoderReranker struct {
	modelPath string
	vocab     map[string]int64
	clsID     int64
	sepID     int64
	unkID     int64
}

func NewCrossEncoderReranker(modelPath, tokenizerPath string) (*CrossEncoderReranker, error) {
	tokData, err := os.ReadFile(tokenizerPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read reranker tokenizer: %w", err)
	}

	var root struct {
		Model struct {
			Vocab map[string]int64 `json:"vocab"`
		} `json:"model"`
	}

	if err := json.Unmarshal(tokData, &root); err != nil {
		return nil, fmt.Errorf("failed to parse reranker tokenizer JSON: %w", err)
	}

	vocab := root.Model.Vocab
	return &CrossEncoderReranker{
		modelPath: modelPath,
		vocab:     vocab,
		clsID:     vocab["[CLS]"],
		sepID:     vocab["[SEP]"],
		unkID:     vocab["[UNK]"],
	}, nil
}

func (cr *CrossEncoderReranker) Rerank(query string, candidates []indexer.SearchResult, topN int) []indexer.SearchResult {
	if len(candidates) <= 1 || query == "" {
		return candidates
	}

	maxLen := 128
	for i := range candidates {
		pairText := strings.ToLower(query + " " + candidates[i].Chunk.Content)
		words := strings.Fields(pairText)

		inputIDs := []int64{cr.clsID}
		for _, w := range words {
			if len(inputIDs) >= maxLen-1 {
				break
			}
			if id, ok := cr.vocab[w]; ok {
				inputIDs = append(inputIDs, id)
			} else {
				inputIDs = append(inputIDs, cr.unkID)
			}
		}
		inputIDs = append(inputIDs, cr.sepID)

		paddedIDs := make([]int64, maxLen)
		mask := make([]int64, maxLen)
		for idx, id := range inputIDs {
			paddedIDs[idx] = id
			mask[idx] = 1
		}

		tokenTypeIDs := make([]int64, maxLen)
		shape := ort.NewShape(1, int64(maxLen))
		inputIDsTensor, err1 := ort.NewTensor(shape, paddedIDs)
		maskTensor, err2 := ort.NewTensor(shape, mask)
		typeIDsTensor, err3 := ort.NewTensor(shape, tokenTypeIDs)

		if err1 == nil && err2 == nil && err3 == nil {
			outputShape := ort.NewShape(1, 1)
			outputData := make([]float32, 1)
			outputTensor, err4 := ort.NewTensor(outputShape, outputData)

			if err4 == nil {
				session, errSession := ort.NewAdvancedSession(
					cr.modelPath,
					[]string{"input_ids", "attention_mask", "token_type_ids"},
					[]string{"logits"},
					[]ort.ArbitraryTensor{inputIDsTensor, maskTensor, typeIDsTensor},
					[]ort.ArbitraryTensor{outputTensor},
					nil,
				)

				if errSession == nil {
					if errRun := session.Run(); errRun == nil {
						candidates[i].Score += outputData[0]
					}
					session.Destroy()
				}
				outputTensor.Destroy()
			}
		}

		if err1 == nil {
			inputIDsTensor.Destroy()
		}
		if err2 == nil {
			maskTensor.Destroy()
		}
		if err3 == nil {
			typeIDsTensor.Destroy()
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	if len(candidates) > topN {
		candidates = candidates[:topN]
	}

	return candidates
}
