package embedder

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	ort "github.com/yalue/onnxruntime_go"
)

type Tokenizer struct {
	vocab map[string]int64
	unkID int64
	clsID int64
	sepID int64
}

func NewTokenizer(tokenizerJSON []byte) (*Tokenizer, error) {
	var root struct {
		Model struct {
			Vocab map[string]int64 `json:"vocab"`
		} `json:"model"`
		AddedTokens []struct {
			ID      int64  `json:"id"`
			Content string `json:"content"`
		} `json:"added_tokens"`
	}

	if err := json.Unmarshal(tokenizerJSON, &root); err != nil {
		return nil, err
	}

	vocab := root.Model.Vocab
	if vocab == nil {
		vocab = make(map[string]int64)
	}

	for _, at := range root.AddedTokens {
		vocab[at.Content] = at.ID
	}

	unkID, ok := vocab["[UNK]"]
	if !ok {
		unkID = vocab["<unk>"]
	}

	clsID, ok := vocab["[CLS]"]
	if !ok {
		clsID = vocab["<s>"]
	}

	sepID, ok := vocab["[SEP]"]
	if !ok {
		sepID = vocab["</s>"]
	}

	return &Tokenizer{
		vocab: vocab,
		unkID: unkID,
		clsID: clsID,
		sepID: sepID,
	}, nil
}

func (t *Tokenizer) Tokenize(text string, maxLen int) ([]int64, []int64, []int64) {
	text = strings.ToLower(text)
	words := strings.Fields(text)

	inputIDs := []int64{t.clsID}

	for _, word := range words {
		if len(inputIDs) >= maxLen-1 {
			break
		}

		if id, ok := t.vocab[word]; ok {
			inputIDs = append(inputIDs, id)
			continue
		}

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

type Embedder struct {
	tokenizer *Tokenizer
	modelPath string
	maxLen    int
	dim       int
}

func NewEmbedder(modelPath string, tokenizerJSON []byte) (*Embedder, error) {
	tok, err := NewTokenizer(tokenizerJSON)
	if err != nil {
		return nil, err
	}

	ort.SetSharedLibraryPath("/opt/homebrew/Cellar/onnxruntime/1.28.0_1/lib/libonnxruntime.dylib")
	if err := ort.InitializeEnvironment(); err != nil {
		// Environment may already be initialized
	}

	dim := 768
	if strings.Contains(modelPath, "code_minilm") {
		dim = 384
	} else if strings.Contains(modelPath, "qwen3_embed") {
		dim = 1024
	}

	return &Embedder{
		tokenizer: tok,
		modelPath: modelPath,
		maxLen:    128,
		dim:       dim,
	}, nil
}

func (e *Embedder) Dimension() int {
	return e.dim
}

func (e *Embedder) EmbedBatch(texts []string, prefix string) ([]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	batchSize := 32
	total := len(texts)
	result := make([]float32, total*e.dim)

	for i := 0; i < total; i += batchSize {
		end := i + batchSize
		if end > total {
			end = total
		}

		subTexts := texts[i:end]
		curBatchSize := len(subTexts)

		allIDs := make([]int64, curBatchSize*e.maxLen)
		allMask := make([]int64, curBatchSize*e.maxLen)
		allTypeIDs := make([]int64, curBatchSize*e.maxLen)

		for b, text := range subTexts {
			fullText := prefix + text
			ids, mask, types := e.tokenizer.Tokenize(fullText, e.maxLen)
			copy(allIDs[b*e.maxLen:(b+1)*e.maxLen], ids)
			copy(allMask[b*e.maxLen:(b+1)*e.maxLen], mask)
			copy(allTypeIDs[b*e.maxLen:(b+1)*e.maxLen], types)
		}

		shape := ort.NewShape(int64(curBatchSize), int64(e.maxLen))

		inputIDsTensor, err := ort.NewTensor(shape, allIDs)
		if err != nil {
			return nil, fmt.Errorf("input_ids tensor error: %w", err)
		}

		attentionMaskTensor, err := ort.NewTensor(shape, allMask)
		if err != nil {
			inputIDsTensor.Destroy()
			return nil, fmt.Errorf("attention_mask tensor error: %w", err)
		}

		tokenTypeIDsTensor, err := ort.NewTensor(shape, allTypeIDs)
		if err != nil {
			inputIDsTensor.Destroy()
			attentionMaskTensor.Destroy()
			return nil, fmt.Errorf("token_type_ids tensor error: %w", err)
		}

		outputShape := ort.NewShape(int64(curBatchSize), int64(e.maxLen), int64(e.dim))
		outputData := make([]float32, curBatchSize*e.maxLen*e.dim)
		outputTensor, err := ort.NewTensor(outputShape, outputData)
		if err != nil {
			inputIDsTensor.Destroy()
			attentionMaskTensor.Destroy()
			tokenTypeIDsTensor.Destroy()
			return nil, fmt.Errorf("output tensor error: %w", err)
		}

		session, err := ort.NewAdvancedSession(
			e.modelPath,
			[]string{"input_ids", "attention_mask", "token_type_ids"},
			[]string{"last_hidden_state"},
			[]ort.ArbitraryTensor{inputIDsTensor, attentionMaskTensor, tokenTypeIDsTensor},
			[]ort.ArbitraryTensor{outputTensor},
			nil,
		)
		if err != nil {
			session, err = ort.NewAdvancedSession(
				e.modelPath,
				[]string{"input_ids", "attention_mask"},
				[]string{"last_hidden_state"},
				[]ort.ArbitraryTensor{inputIDsTensor, attentionMaskTensor},
				[]ort.ArbitraryTensor{outputTensor},
				nil,
			)
			if err != nil {
				inputIDsTensor.Destroy()
				attentionMaskTensor.Destroy()
				tokenTypeIDsTensor.Destroy()
				outputTensor.Destroy()
				return nil, fmt.Errorf("ONNX session error: %w", err)
			}
		}

		err = session.Run()
		inputIDsTensor.Destroy()
		attentionMaskTensor.Destroy()
		tokenTypeIDsTensor.Destroy()
		outputTensor.Destroy()
		session.Destroy()

		if err != nil {
			return nil, fmt.Errorf("ONNX inference execution failed: %w", err)
		}

		subVectors := poolAndNormalize(outputData, allMask, curBatchSize, e.maxLen, e.dim)
		copy(result[i*e.dim:end*e.dim], subVectors)
	}

	return result, nil
}

func poolAndNormalize(raw []float32, attentionMask []int64, batchSize, maxSeqLen, hiddenDim int) []float32 {
	result := make([]float32, batchSize*hiddenDim)

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

		copy(result[b*hiddenDim:(b+1)*hiddenDim], vec)
	}

	return result
}
