# Architecture

Overview of how `semsearch` works under the hood.

```
semsearch/
├── main.go         # CLI commands (cobra)
├── pkg/
│   ├── scanner/    # Walks folders, handles .gitignore / .semsearchignore
│   ├── chunker/    # AST parsing (tree-sitter), markdown sections, sliding window
│   ├── embedder/   # ONNX Runtime + tokenization
│   ├── indexer/    # Vector storage (manifest.json + vectors.bin) and similarity search
│   ├── llm/        # Optional Ollama summaries for chunks (cached)
│   ├── reranker/   # Result scoring, per-file dedup, keyword boost
│   ├── watcher/    # File watcher for incremental re-indexing
│   └── history/    # Stores recent queries
```

---

## How It Works

### 1. Indexing
- **Scan:** `pkg/scanner` walks through directories, skips `.git`, `node_modules`, binaries, minified files, and anything ignored by `.gitignore` or `.semsearchignore`.
- **Chunk:** `pkg/chunker` breaks source code down into clean logical blocks:
  - Tree-sitter for Go, Python, JS/TS, Rust, etc. (functions, methods, classes).
  - Header-based splitting for Markdown.
  - Sliding-window (30 lines with overlap) as fallback for other files.
- **Embed:** Chunks get embedded with an ONNX model (CodeMiniLM or Nomic). If Ollama is running, larger chunks also get a short natural language description generated and embedded as a second vector.
- **Save:** Writes `.semsearch/manifest.json` (chunk metadata, file paths, line ranges) and `.semsearch/vectors.bin` (raw float32 vectors).

### 2. Searching
- Embeds the search query using the same ONNX model.
- Calculates cosine similarity against all indexed vectors.
- Blends vector similarity with BM25 keyword matching (default 65% semantic, 35% keyword).
- Reranks results: deduplicates multiple chunks from the exact same file to avoid clutter, applies a slight boost for matching identifiers or paths, and returns the top matches with snippets and line numbers.

---

## Design Choices

- **Local-first:** Everything runs on your machine. No telemetry, no remote servers, no cloud embeddings.
- **Dual vectors:** Storing both code embeddings and plain-English summary embeddings bridges the gap when searching for ideas ("handle payment webhook") against raw code implementations.
- **Flat files over vector DBs:** At codebase scale (1k–50k chunks), flat float arrays in memory take just tens of milliseconds to scan through. Skipping an external database keeps setup trivial.
- **Parallel processing:** Chunking runs across available CPU cores, and keyword stats are cached in memory after the first load so repeated searches stay fast (<50ms).
