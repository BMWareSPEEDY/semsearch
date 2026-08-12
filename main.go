package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ashishpaliwal/semsearch/pkg/chunker"
	"github.com/ashishpaliwal/semsearch/pkg/embedder"
	"github.com/ashishpaliwal/semsearch/pkg/history"
	"github.com/ashishpaliwal/semsearch/pkg/indexer"
	"github.com/ashishpaliwal/semsearch/pkg/llm"
	"github.com/ashishpaliwal/semsearch/pkg/reranker"
	"github.com/ashishpaliwal/semsearch/pkg/scanner"
	"github.com/ashishpaliwal/semsearch/pkg/watcher"
)

var (
	modelPath     = "scratch/code_minilm/model_quantized.onnx"
	tokenizerPath = "scratch/code_minilm/tokenizer.json"
)

const defaultLLMModel = "qwen3.5:0.8b"

func main() {
	var topN int
	var minScore float64
	var contextLines int
	var jsonOutput bool
	var codeOnly bool
	var docsOnly bool
	var editorApp string

	var rootCmd = &cobra.Command{
		Use:   "semsearch [path] <query>",
		Short: "Local semantic code search engine",
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				cmd.Help()
				return
			}
			targetDir := "."
			query := args[0]

			if info, err := os.Stat(args[0]); err == nil && info.IsDir() {
				if len(args) < 2 {
					cmd.Help()
					return
				}
				targetDir = args[0]
				query = args[1]
			}

			runSearch(targetDir, query, topN, float32(minScore), contextLines, editorApp, codeOnly, docsOnly, jsonOutput)
		},
	}

	rootCmd.Flags().IntVarP(&topN, "top", "t", 5, "Number of top results to return")
	rootCmd.Flags().Float64VarP(&minScore, "min-score", "m", 0.0, "Minimum similarity score threshold")
	rootCmd.Flags().IntVarP(&contextLines, "context", "c", 0, "Number of surrounding context lines to expand")
	rootCmd.Flags().StringVarP(&editorApp, "editor", "e", "", "Jump to result in editor (e.g. code, vim, subl)")
	rootCmd.Flags().BoolVar(&codeOnly, "code-only", false, "Filter search to code implementation files only")
	rootCmd.Flags().BoolVar(&docsOnly, "docs-only", false, "Filter search to documentation files only")
	rootCmd.Flags().BoolVarP(&jsonOutput, "json", "j", false, "Output results in JSON format")

	var indexCmd = &cobra.Command{
		Use:   "index [path]",
		Short: "Index a directory for semantic search",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			targetDir := "."
			if len(args) > 0 {
				targetDir = args[0]
			}
			noLLM, _ := cmd.Flags().GetBool("no-llm")
			runIndexing(targetDir, noLLM)
		},
	}
	indexCmd.Flags().Bool("no-llm", false, "Skip LLM description generation (fall back to heuristic synthetic descriptions)")

	var watchCmd = &cobra.Command{
		Use:   "watch [path]",
		Short: "Start incremental re-indexing watcher on file changes",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			targetDir := "."
			if len(args) > 0 {
				targetDir = args[0]
			}
			runWatcher(targetDir)
		},
	}

	var historyCmd = &cobra.Command{
		Use:   "history [path]",
		Short: "View query search history and opened actions",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			targetDir := "."
			if len(args) > 0 {
				targetDir = args[0]
			}
			runHistory(targetDir)
		},
	}

	var serveCmd = &cobra.Command{
		Use:   "serve [path]",
		Short: "Start local HTTP API server",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			targetDir := "."
			if len(args) > 0 {
				targetDir = args[0]
			}
			port, _ := cmd.Flags().GetInt("port")
			runServer(targetDir, port)
		},
	}
	serveCmd.Flags().IntP("port", "p", 1921, "HTTP server port")

	var statusCmd = &cobra.Command{
		Use:   "status [path]",
		Short: "Check index status",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			targetDir := "."
			if len(args) > 0 {
				targetDir = args[0]
			}
			runStatus(targetDir)
		},
	}

	rootCmd.AddCommand(indexCmd, watchCmd, historyCmd, serveCmd, statusCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runIndexing(dir string, noLLM bool) {
	fmt.Printf("Scanning directory: %s...\n", dir)
	s, err := scanner.NewFileScanner(dir)
	if err != nil {
		log.Fatalf("Failed to initialize scanner: %v", err)
	}

	files, err := s.Scan()
	if err != nil {
		log.Fatalf("File scanning failed: %v", err)
	}

	fmt.Printf("Found %d files. Extracting chunks...\n", len(files))
	c := chunker.NewDefaultChunker()
	var allChunks []chunker.Chunk

	fileHashes := make(map[string]string)
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		hash := sha256.Sum256(content)
		fileHashes[file] = hex.EncodeToString(hash[:8])

		chunks, err := c.ChunkFile(file)
		if err != nil {
			continue
		}
		for i := range chunks {
			chunks[i].ID = len(allChunks)
			allChunks = append(allChunks, chunks[i])
		}
	}

	fmt.Printf("Extracted %d chunks. Loading ONNX embedder...\n", len(allChunks))
	tokenizerJSON, err := os.ReadFile(tokenizerPath)
	if err != nil {
		log.Fatalf("Failed to read tokenizer JSON: %v", err)
	}

	emb, err := embedder.NewEmbedder(modelPath, tokenizerJSON)
	if err != nil {
		log.Fatalf("Failed to initialize embedder: %v", err)
	}

	descCache := llm.NewDescriptionCache(filepath.Join(dir, ".semsearch"))

	ctx := context.Background()
	minDescLines := 30
	if v := os.Getenv("SEMSEARCH_MIN_DESC_LINES"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			minDescLines = parsed
		}
	}
	concurrency := 4
	if v := os.Getenv("SEMSEARCH_LLM_CONCURRENCY"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			concurrency = parsed
		}
	}

	// Index-time LLM description generation (never used at query time).
	// One call per chunk, temperature 0, hash-gated disk cache so re-runs
	// and the watcher reuse descriptions for unchanged chunks.
	llmModel := os.Getenv("SEMSEARCH_LLM_MODEL")
	if llmModel == "" {
		llmModel = defaultLLMModel
	}
	llmClient := llm.NewOllamaClient(llmModel)
	if !noLLM && !llmClient.IsAvailable() {
		fmt.Printf("WARNING: local LLM (%s) unavailable — falling back to heuristic descriptions\n", llmModel)
	}

	var codeTexts []string
	var descTexts []string
	var skipDesc []bool // true => reuse code vector as description vector (small/trivial chunks)

	codeTexts = make([]string, len(allChunks))
	for i := range allChunks {
		codeTexts[i] = chunker.ChunkCodeText(allChunks[i].Content)
	}

	newDescs := 0
	if !noLLM {
		skipDesc = make([]bool, len(allChunks))
		for i := range allChunks {
			chunkLines := allChunks[i].LineEnd - allChunks[i].LineStart + 1
			if isMarkdownFile(allChunks[i].FilePath) || chunkLines < minDescLines {
				skipDesc[i] = true
			}
		}
		descTexts, newDescs = llm.GenerateDescriptions(ctx, llmClient, descCache, allChunks, func(i int) bool { return skipDesc[i] }, concurrency)
	} else {
		descTexts = make([]string, len(allChunks))
	}

	// Heuristic fallback for chunks without an LLM description.
	for i := range allChunks {
		if descTexts[i] == "" {
			descTexts[i] = chunker.GenerateSyntheticDescription(allChunks[i].Content, allChunks[i].SymbolName, allChunks[i].Kind, allChunks[i].FilePath)
		}
		allChunks[i].Description = descTexts[i]
	}

	fmt.Printf("Generating dual vector embeddings (code + descriptions) for %d chunks...\n", len(allChunks))
	codeVectors, err := emb.EmbedBatch(codeTexts, "search_document: ")
	if err != nil {
		log.Fatalf("Code embedding generation failed: %v", err)
	}

	descVectors, err := emb.EmbedBatch(descTexts, "search_document: ")
	if err != nil {
		log.Fatalf("Description embedding generation failed: %v", err)
	}

	// Small/trivial chunks carry no description — reuse their code vector so the
	// max(code, desc) blend degenerates to plain code similarity for them.
	dim := emb.Dimension()
	for i := range allChunks {
		if len(skipDesc) > i && skipDesc[i] {
			off := i * dim
			copy(descVectors[off:off+dim], codeVectors[off:off+dim])
		}
	}

	store := indexer.NewIndexStore(dir)
	manifest := indexer.Manifest{
		Version:           "1.0",
		CreatedAt:         time.Now(),
		Model:             "isuruwijesiri/all-MiniLM-L6-v2-code-search-512",
		VectorDimensions:  emb.Dimension(),
		TotalChunks:       len(allChunks),
		IndexedFilesCount: len(files),
		FileHashes:        fileHashes,
		Chunks:            allChunks,
	}

	if err := store.SaveDual(manifest, codeVectors, descVectors); err != nil {
		log.Fatalf("Failed to save dual vector index: %v", err)
	}

	fmt.Printf("Indexing complete! Dual vector index saved to %s/.semsearch/\n", dir)
	if !noLLM {
		fmt.Printf("LLM descriptions generated for %d new chunks (cached in .semsearch/desc_cache.json)\n", newDescs)
	}
}

func runSearch(dir, query string, topN int, minScore float32, contextLines int, editorApp string, codeOnly, docsOnly, jsonOutput bool) {
	store := indexer.NewIndexStore(dir)
	if err := store.Load(); err != nil {
		log.Fatalf("No index found in %s. Run 'semsearch index %s' first: %v", dir, dir, err)
	}

	hist := history.NewHistoryManager(dir)
	queryID, _ := hist.Add(query)

	tokenizerJSON, err := os.ReadFile(tokenizerPath)
	if err != nil {
		log.Fatalf("Failed to read tokenizer JSON: %v", err)
	}

	emb, err := embedder.NewEmbedder(modelPath, tokenizerJSON)
	if err != nil {
		log.Fatalf("Failed to initialize embedder: %v", err)
	}

	queryVec, err := emb.EmbedBatch([]string{query}, "search_query: ")
	if err != nil {
		log.Fatalf("Failed to embed query: %v", err)
	}

	// Candidate shortlist size (Top 30 candidate retrieval for reranking pass)
	candidateLimit := 30
	if topN > candidateLimit {
		candidateLimit = topN
	}

	rawCandidates := store.HybridSearch(queryVec, query, candidateLimit, minScore, 0.65, codeOnly, docsOnly)

	// Second-pass Reranker over Top-20 candidates
	r := reranker.NewReranker()
	results := r.Rerank(query, rawCandidates, topN)

	if jsonOutput {
		data, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("\nSearch Results for query: '%s'\n", query)
	fmt.Printf("==================================================\n")
	absDir, _ := filepath.Abs(dir)
	for i, res := range results {
		relPath, err := filepath.Rel(absDir, res.Chunk.FilePath)
		if err != nil || strings.HasPrefix(relPath, "..") {
			relPath = res.Chunk.FilePath
		}

		displayStart := res.Chunk.LineStart
		displayEnd := res.Chunk.LineEnd

		fmt.Printf("\n%d. %s:%d-%d (score: %.4f)\n", i+1, relPath, displayStart, displayEnd, res.Score)
		fmt.Printf("--------------------------------------------------\n")

		if contextLines > 0 {
			printExpandedContext(res.Chunk.FilePath, res.Chunk.LineStart, res.Chunk.LineEnd, contextLines)
		} else {
			lines := strings.Split(res.Chunk.Preview, "\n")
			for _, l := range lines {
				fmt.Printf("   %s\n", l)
			}
		}
	}
	fmt.Printf("==================================================\n\n")

	if editorApp != "" && len(results) > 0 {
		topMatch := results[0].Chunk
		hist.RecordAction(queryID, topMatch.FilePath, topMatch.LineStart)
		openInEditor(editorApp, topMatch.FilePath, topMatch.LineStart)
	}
}

func isMarkdownFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown" || ext == ".rst" || ext == ".txt" || ext == ".adoc"
}

func printExpandedContext(filePath string, lineStart, lineEnd, contextLines int) {
	file, err := os.Open(filePath)
	if err != nil {
		return
	}
	defer file.Close()

	start := lineStart - contextLines
	if start < 1 {
		start = 1
	}
	end := lineEnd + contextLines

	scanner := bufio.NewScanner(file)
	currentLine := 0
	for scanner.Scan() {
		currentLine++
		if currentLine >= start && currentLine <= end {
			prefix := "   "
			if currentLine >= lineStart && currentLine <= lineEnd {
				prefix = " > "
			}
			fmt.Printf("%s%4d | %s\n", prefix, currentLine, scanner.Text())
		}
	}
}

func openInEditor(editorApp, filePath string, line int) {
	fmt.Printf("Opening %s:%d in editor '%s'...\n", filePath, line, editorApp)
	var cmd *exec.Cmd
	switch strings.ToLower(editorApp) {
	case "code", "vscode":
		cmd = exec.Command("code", "--goto", fmt.Sprintf("%s:%d", filePath, line))
	case "vim", "nvim":
		cmd = exec.Command(editorApp, fmt.Sprintf("+%d", line), filePath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	case "subl", "sublime":
		cmd = exec.Command("subl", fmt.Sprintf("%s:%d", filePath, line))
	default:
		cmd = exec.Command(editorApp, filePath)
	}

	if err := cmd.Run(); err != nil {
		log.Printf("Failed to launch editor '%s': %v", editorApp, err)
	}
}

func runWatcher(dir string) {
	store := indexer.NewIndexStore(dir)
	if err := store.Load(); err != nil {
		log.Fatalf("No index found in %s. Run 'semsearch index %s' first: %v", dir, dir, err)
	}

	tokenizerJSON, err := os.ReadFile(tokenizerPath)
	if err != nil {
		log.Fatalf("Failed to read tokenizer JSON: %v", err)
	}

	emb, err := embedder.NewEmbedder(modelPath, tokenizerJSON)
	if err != nil {
		log.Fatalf("Failed to initialize embedder: %v", err)
	}

	sc, err := scanner.NewFileScanner(dir)
	if err != nil {
		log.Fatalf("Failed to initialize scanner: %v", err)
	}

	w := watcher.NewIndexWatcher(dir, store, emb, sc)
	llmModel := os.Getenv("SEMSEARCH_LLM_MODEL")
	if llmModel == "" {
		llmModel = defaultLLMModel
	}
	llmClient := llm.NewOllamaClient(llmModel)
	if llmClient.IsAvailable() {
		w.SetLLMClient(llmClient)
	}
	if err := w.Start(); err != nil {
		log.Fatalf("Watcher error: %v", err)
	}
}

func runHistory(dir string) {
	hist := history.NewHistoryManager(dir)
	logs, err := hist.Load()
	if err != nil || len(logs) == 0 {
		fmt.Println("No search history recorded yet.")
		return
	}

	fmt.Printf("Search Query History for %s:\n", dir)
	fmt.Printf("==================================================\n")
	for _, l := range logs {
		fmt.Printf("%d. [%s] \"%s\"\n", l.ID, l.Timestamp.Format("2006-01-02 15:04:05"), l.Query)
		if l.OpenedFile != "" {
			fmt.Printf("   -> Opened: %s:%d\n", l.OpenedFile, l.OpenedLine)
		}
	}
}

func runServer(dir string, port int) {
	store := indexer.NewIndexStore(dir)
	if err := store.Load(); err != nil {
		log.Fatalf("No index found in %s. Run 'semsearch index %s' first: %v", dir, dir, err)
	}

	tokenizerJSON, err := os.ReadFile(tokenizerPath)
	if err != nil {
		log.Fatalf("Failed to read tokenizer JSON: %v", err)
	}

	emb, err := embedder.NewEmbedder(modelPath, tokenizerJSON)
	if err != nil {
		log.Fatalf("Failed to initialize embedder: %v", err)
	}

	searchHandler := func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			q = r.URL.Query().Get("query")
		}
		if q == "" {
			http.Error(w, "missing search query parameter (q or query)", http.StatusBadRequest)
			return
		}

		topN := 5
		limitStr := r.URL.Query().Get("top")
		if limitStr == "" {
			limitStr = r.URL.Query().Get("limit")
		}
		if limitStr != "" {
			if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
				topN = parsed
			}
		}

		// Support explicit mode=code or mode=docs, as well as code_only / docs_only / code / docs flags
		mode := strings.ToLower(r.URL.Query().Get("mode"))
		codeOnly := mode == "code" || r.URL.Query().Get("code_only") == "true" || r.URL.Query().Get("code") == "true"
		docsOnly := mode == "docs" || r.URL.Query().Get("docs_only") == "true" || r.URL.Query().Get("docs") == "true"

		// Route-specific defaults
		if strings.HasSuffix(r.URL.Path, "/code") {
			codeOnly = true
		} else if strings.HasSuffix(r.URL.Path, "/docs") {
			docsOnly = true
		}

		queryVec, err := emb.EmbedBatch([]string{q}, "search_query: ")
		if err != nil {
			http.Error(w, "failed to embed query", http.StatusInternalServerError)
			return
		}

		results := store.HybridSearch(queryVec, q, topN, 0.0, 0.95, codeOnly, docsOnly)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"query":   q,
			"results": results,
		})
	}

	statusHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":         "ok",
			"indexed_chunks": store.Manifest.TotalChunks,
			"indexed_files":  store.Manifest.IndexedFilesCount,
			"model":          store.Manifest.Model,
		})
	}

	// Developer-Friendly Endpoints
	http.HandleFunc("/api/search", searchHandler)
	http.HandleFunc("/api/code", searchHandler)
	http.HandleFunc("/api/docs", searchHandler)
	http.HandleFunc("/api/status", statusHandler)

	// Legacy & Quick Aliases
	http.HandleFunc("/search", searchHandler)
	http.HandleFunc("/health", statusHandler)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	fmt.Printf("Starting semsearch REST API on http://%s\n", addr)
	fmt.Printf("  • Search Code & Docs : GET http://%s/api/search?q=...\n", addr)
	fmt.Printf("  • Search Code Only   : GET http://%s/api/code?q=...\n", addr)
	fmt.Printf("  • Search Docs Only   : GET http://%s/api/docs?q=...\n", addr)
	fmt.Printf("  • System Status      : GET http://%s/api/status\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func runStatus(dir string) {
	store := indexer.NewIndexStore(dir)
	if err := store.Load(); err != nil {
		fmt.Printf("No active index found in %s.\n", dir)
		return
	}

	fmt.Printf("Index Status for %s:\n", dir)
	fmt.Printf("  Model: %s\n", store.Manifest.Model)
	fmt.Printf("  Indexed Chunks: %d\n", store.Manifest.TotalChunks)
	fmt.Printf("  Indexed Files: %d\n", store.Manifest.IndexedFilesCount)
	fmt.Printf("  Created At: %s\n", store.Manifest.CreatedAt.Format(time.RFC1123))
}
