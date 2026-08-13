package watcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/ashishpaliwal/semsearch/pkg/chunker"
	"github.com/ashishpaliwal/semsearch/pkg/embedder"
	"github.com/ashishpaliwal/semsearch/pkg/indexer"
	"github.com/ashishpaliwal/semsearch/pkg/llm"
	"github.com/ashishpaliwal/semsearch/pkg/scanner"
)

type IndexWatcher struct {
	rootDir   string
	store     *indexer.IndexStore
	embedder  *embedder.Embedder
	scanner   *scanner.FileScanner
	chunker   chunker.Chunker
	llmClient llm.LLMClient
	descCache *llm.DescriptionCache
	mu        sync.Mutex
}

func NewIndexWatcher(rootDir string, store *indexer.IndexStore, emb *embedder.Embedder, sc *scanner.FileScanner) *IndexWatcher {
	return &IndexWatcher{
		rootDir:   rootDir,
		store:     store,
		embedder:  emb,
		scanner:   sc,
		chunker:   chunker.NewDefaultChunker(),
		llmClient: nil,
		descCache: llm.NewDescriptionCache(filepath.Join(rootDir, ".semsearch")),
	}
}

// SetLLMClient enables index-time LLM description generation on the watch path.
// When unset, the watcher falls back to heuristic synthetic descriptions.
func (w *IndexWatcher) SetLLMClient(llmClient llm.LLMClient) {
	w.llmClient = llmClient
}

func (w *IndexWatcher) Start() error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fsw.Close()

	// Recursively add directories to watch
	err = filepath.Walk(w.rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == ".semsearch" || base == "node_modules" || base == "dist" || base == "vendor" {
				return filepath.SkipDir
			}
			return fsw.Add(path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Printf("Started semsearch watch mode on %s. Monitoring filesystem changes...\n", w.rootDir)

	var debounceTimer *time.Timer
	changedFiles := make(map[string]bool)
	var mapMu sync.Mutex

	for {
		select {
		case event, ok := <-fsw.Events:
			if !ok {
				return nil
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove) != 0 {
				ext := filepath.Ext(event.Name)
				if ext == ".bin" || ext == ".json" || filepath.Base(event.Name) == "semsearch" {
					continue
				}

				mapMu.Lock()
				changedFiles[event.Name] = true
				mapMu.Unlock()

				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.AfterFunc(500*time.Millisecond, func() {
					mapMu.Lock()
					filesToProcess := make([]string, 0, len(changedFiles))
					for f := range changedFiles {
						filesToProcess = append(filesToProcess, f)
					}
					changedFiles = make(map[string]bool)
					mapMu.Unlock()

					w.reindexFiles(filesToProcess)
				})
			}
		case err, ok := <-fsw.Errors:
			if !ok {
				return nil
			}
			log.Printf("Watcher error: %v", err)
		}
	}
}

func (w *IndexWatcher) reindexFiles(filePaths []string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, path := range filePaths {
		content, err := os.ReadFile(path)
		if err != nil {
			// File removed or unreadable
			continue
		}

		hash := sha256.Sum256(content)
		hashStr := hex.EncodeToString(hash[:8])

		if w.store.Manifest.FileHashes != nil {
			if existingHash, ok := w.store.Manifest.FileHashes[path]; ok && existingHash == hashStr {
				continue
			}
		}

		fmt.Printf("[%s] Re-embedding modified file: %s...\n", time.Now().Format("15:04:05"), path)

		chunks, err := w.chunker.ChunkBytes(path, content)
		if err != nil || len(chunks) == 0 {
			continue
		}

		llmAvailable := w.llmClient != nil && w.llmClient.IsAvailable()

		minDescLines := 30
		var codeTexts []string
		var descTexts []string
		var skipDesc []bool
		for _, ch := range chunks {
			codeTexts = append(codeTexts, ch.Content)
			ext := strings.ToLower(filepath.Ext(ch.FilePath))
			chunkLines := ch.LineEnd - ch.LineStart + 1
			if ext == ".md" || ext == ".markdown" || ext == ".rst" || chunkLines < minDescLines {
				skipDesc = append(skipDesc, true)
				descTexts = append(descTexts, "")
				continue
			}
			skipDesc = append(skipDesc, false)
			descTexts = append(descTexts, "")
		}

		if llmAvailable {
			descTexts, _ = llm.GenerateDescriptions(context.Background(), w.llmClient, w.descCache, chunks, func(i int) bool { return skipDesc[i] }, 4)
		}

		for i := range chunks {
			if descTexts[i] == "" {
				descTexts[i] = chunker.GenerateSyntheticDescription(chunks[i].Content, chunks[i].SymbolName, chunks[i].Kind, chunks[i].FilePath)
			}
			chunks[i].Description = descTexts[i]
		}

		codeVectors, err := w.embedder.EmbedBatch(codeTexts, "search_document: ")
		if err != nil {
			log.Printf("Failed to embed modified file %s: %v", path, err)
			continue
		}

		descVectors, err := w.embedder.EmbedBatch(descTexts, "search_document: ")
		if err != nil {
			log.Printf("Failed to embed descriptions for %s: %v", path, err)
			continue
		}

		dim := w.embedder.Dimension()
		for i := range chunks {
			if skipDesc[i] {
				off := i * dim
				copy(descVectors[off:off+dim], codeVectors[off:off+dim])
			}
		}

		if err := w.store.UpdateFileChunksDual(path, hashStr, chunks, codeVectors, descVectors); err != nil {
			log.Printf("Failed to update vector index for %s: %v", path, err)
		} else {
			fmt.Printf("[%s] Successfully updated index for %s (%d chunks)\n", time.Now().Format("15:04:05"), path, len(chunks))
		}
	}
}
