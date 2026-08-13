package embedder

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

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
	batchSize int

	mu          sync.Mutex
	initErr     error
	threeInputs bool
	inputsKnown bool
}

func NewEmbedder(modelPath string, tokenizerJSON []byte) (*Embedder, error) {
	tok, err := NewTokenizer(tokenizerJSON)
	if err != nil {
		return nil, err
	}

	if err := ensureRuntime(); err != nil {
		return nil, err
	}

	dim := 768
	if strings.Contains(modelPath, "code_minilm") {
		dim = 384
	} else if strings.Contains(modelPath, "qwen3_embed") {
		dim = 1024
	}

	maxLen := 128
	if v := os.Getenv("SEMSEARCH_MAX_LEN"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 32 && parsed <= 512 {
			maxLen = parsed
		}
	}

	batchSize := 32
	if dim > 512 {
		batchSize = 16
	}
	if v := os.Getenv("SEMSEARCH_BATCH_SIZE"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 1 && parsed <= 512 {
			batchSize = parsed
		}
	}

	return &Embedder{
		tokenizer: tok,
		modelPath: modelPath,
		maxLen:    maxLen,
		dim:       dim,
		batchSize: batchSize,
	}, nil
}

func (e *Embedder) Dimension() int {
	return e.dim
}

// inputSpec returns the ONNX input names and bound tensors for this model,
// using the cached 2-vs-3-input decision when known so later batches never
// re-probe the failing configuration.
func (e *Embedder) inputSpec(idsTensor, maskTensor, typesTensor *ort.Tensor[int64]) ([]string, []ort.ArbitraryTensor) {
	if e.inputsKnown && !e.threeInputs {
		return []string{"input_ids", "attention_mask"}, []ort.ArbitraryTensor{idsTensor, maskTensor}
	}
	return []string{"input_ids", "attention_mask", "token_type_ids"}, []ort.ArbitraryTensor{idsTensor, maskTensor, typesTensor}
}

// runBatch embeds a single batch of n texts, writing their pooled and
// normalized vectors into dest at the chunk offset. Sessions and tensors are
// created per batch sized exactly to n rows, so small calls (e.g. one query)
// pay for n rows of inference instead of a padded full batch, and batches are
// always in-bounds regardless of how many rows precede them.
func (e *Embedder) runBatch(texts []string, prefix string, n, offset int, dest []float32) error {
	rowLen := e.maxLen

	idsData := make([]int64, n*rowLen)
	maskData := make([]int64, n*rowLen)
	typesData := make([]int64, n*rowLen)

	for b, text := range texts {
		ids, mask, types := e.tokenizer.Tokenize(prefix+text, e.maxLen)
		base := b * rowLen
		copy(idsData[base:base+rowLen], ids)
		copy(maskData[base:base+rowLen], mask)
		copy(typesData[base:base+rowLen], types)
	}

	shape := ort.NewShape(int64(n), int64(rowLen))
	idsTensor, err := ort.NewTensor(shape, idsData)
	if err != nil {
		return fmt.Errorf("input_ids tensor error: %w", err)
	}
	defer idsTensor.Destroy()
	maskTensor, err := ort.NewTensor(shape, maskData)
	if err != nil {
		return fmt.Errorf("attention_mask tensor error: %w", err)
	}
	defer maskTensor.Destroy()
	typesTensor, err := ort.NewTensor(shape, typesData)
	if err != nil {
		return fmt.Errorf("token_type_ids tensor error: %w", err)
	}
	defer typesTensor.Destroy()

	outputData := make([]float32, n*rowLen*e.dim)
	outputTensor, err := ort.NewTensor(ort.NewShape(int64(n), int64(rowLen), int64(e.dim)), outputData)
	if err != nil {
		return fmt.Errorf("output tensor error: %w", err)
	}
	defer outputTensor.Destroy()
	outputs := []ort.ArbitraryTensor{outputTensor}

	inputNames, inputs := e.inputSpec(idsTensor, maskTensor, typesTensor)
	session, err := ort.NewAdvancedSession(
		e.modelPath,
		inputNames,
		[]string{"last_hidden_state"},
		inputs,
		outputs,
		nil,
	)
	if err == nil {
		err = session.Run()
	}
	if err != nil && !e.inputsKnown {
		// Some exported models take token_type_ids, others don't. The failure
		// can surface at session creation or at Run; retry once without
		// token_type_ids and remember which input set works.
		if session != nil {
			session.Destroy()
			session = nil
		}
		session, err = ort.NewAdvancedSession(
			e.modelPath,
			[]string{"input_ids", "attention_mask"},
			[]string{"last_hidden_state"},
			[]ort.ArbitraryTensor{idsTensor, maskTensor},
			outputs,
			nil,
		)
		if err == nil {
			err = session.Run()
		}
		if err == nil {
			e.threeInputs = false
			e.inputsKnown = true
		}
	}
	if session != nil {
		defer session.Destroy()
	}
	if err != nil {
		return fmt.Errorf("ONNX inference execution failed: %w", err)
	}

	poolInto(outputData, maskData, n, offset, e.maxLen, e.dim, dest)
	return nil
}

// Destroy releases ONNX resources. The embedder holds no persistent sessions or
// tensors (they are created per batch), so this is a no-op kept for API
// compatibility.
func (e *Embedder) Destroy() {
	e.mu.Lock()
	defer e.mu.Unlock()
	// no persistent resources to release
}

func (e *Embedder) EmbedBatch(texts []string, prefix string) ([]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.initErr != nil {
		return nil, e.initErr
	}

	total := len(texts)
	result := make([]float32, total*e.dim)
	bs := e.batchSize

	for start := 0; start < total; start += bs {
		end := start + bs
		if end > total {
			end = total
		}
		if err := e.runBatch(texts[start:end], prefix, end-start, start, result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// poolInto mean-pools + L2-normalizes each batch row and writes it into dest at
// the corresponding chunk offset. raw and mask are batch-local (rows 0..n-1);
// dest is indexed by the absolute chunk row.
func poolInto(raw []float32, mask []int64, realRows, offset, maxSeqLen, hiddenDim int, dest []float32) {
	for b := 0; b < realRows; b++ {
		row := offset + b
		vec := dest[row*hiddenDim : (row+1)*hiddenDim]
		for i := range vec {
			vec[i] = 0
		}
		var validTokens float32 = 0.0
		for s := 0; s < maxSeqLen; s++ {
			if mask[b*maxSeqLen+s] == 1 {
				validTokens += 1.0
				off := (b*maxSeqLen + s) * hiddenDim
				for d := 0; d < hiddenDim; d++ {
					vec[d] += raw[off+d]
				}
			}
		}
		if validTokens > 0 {
			for d := 0; d < hiddenDim; d++ {
				vec[d] /= validTokens
			}
		}

		var norm float64 = 0.0
		for d := 0; d < hiddenDim; d++ {
			v := float64(vec[d])
			norm += v * v
		}
		norm = math.Sqrt(norm)
		if norm > 0 {
			for d := 0; d < hiddenDim; d++ {
				vec[d] = float32(float64(vec[d]) / norm)
			}
		}
	}
}

// ensureRuntime resolves the ONNX Runtime shared library for the current
// platform and initializes the ORT environment (idempotent).
func ensureRuntime() error {
	lib, err := FindLibraryPath()
	if err != nil {
		return err
	}
	ort.SetSharedLibraryPath(lib)
	if err := ort.InitializeEnvironment(); err != nil {
		// Environment may already be initialized.
	}
	return nil
}

// FindLibraryPath locates the ONNX Runtime shared library. It checks (in
// order): the ONNXRUNTIME_LIB env var, the directory of the running binary
// (Windows), then standard Homebrew/system locations for macOS and Linux.
func FindLibraryPath() (string, error) {
	if v := os.Getenv("ONNXRUNTIME_LIB"); v != "" {
		if _, err := os.Stat(v); err == nil {
			return v, nil
		}
		return "", fmt.Errorf("ONNXRUNTIME_LIB=%q does not exist", v)
	}

	// Windows: look for onnxruntime.dll next to the executable, then on PATH.
	if runtime.GOOS == "windows" {
		if exe, err := os.Executable(); err == nil {
			cand := filepath.Join(filepath.Dir(exe), "onnxruntime.dll")
			if _, err := os.Stat(cand); err == nil {
				return cand, nil
			}
		}
		if p, err := exec.LookPath("onnxruntime.dll"); err == nil {
			return p, nil
		}
		return "", fmt.Errorf("onnxruntime.dll not found; set ONNXRUNTIME_LIB to its path")
	}

	candidates := []string{}
	if runtime.GOOS == "darwin" {
		// Homebrew Cellar path is versioned; pick whatever version is present.
		for _, prefix := range []string{"/opt/homebrew", "/usr/local"} {
			matches, _ := filepath.Glob(prefix + "/Cellar/onnxruntime/*/lib/libonnxruntime.dylib")
			candidates = append(candidates, matches...)
		}
		candidates = append(candidates,
			"/opt/homebrew/lib/libonnxruntime.dylib",
			"/usr/local/lib/libonnxruntime.dylib",
			"/opt/homebrew/opt/onnxruntime/lib/libonnxruntime.dylib",
		)
	} else if runtime.GOOS == "linux" {
		candidates = append(candidates,
			"/usr/local/lib/libonnxruntime.so",
			"/usr/local/lib/libonnxruntime.so.1",
			"/usr/lib/libonnxruntime.so",
			"/usr/lib/x86_64-linux-gnu/libonnxruntime.so",
			"/usr/lib/aarch64-linux-gnu/libonnxruntime.so",
		)
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("ONNX Runtime shared library not found; install onnxruntime or set ONNXRUNTIME_LIB")
}
