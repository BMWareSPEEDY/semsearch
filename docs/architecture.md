# semsearch — Architecture

`semsearch` is a local-first semantic code search engine written in Go. This
document describes the runtime pipeline: scan → chunk → embed → index → search.

```
semsearch/
├── main.go                 # CLI entry point (cobra commands)
├── go.mod                  # Module: github.com/ashishpaliwal/semsearch
├── pkg/
│   ├── scanner/            # File system traversal with .gitignore/.semsearchignore
│   ├── chunker/            # Sliding-window + AST + Markdown chunking
│   ├── embedder/           # ONNX Runtime + tokenizer (embeddings)
│   ├── indexer/            # Vector storage (manifest.json + vectors.bin) + cosine search
│   ├── llm/                # Ollama description generation (concurrent, cached)
│   ├── reranker/           # Reranking
│   ├── watcher/            # Incremental re-indexing on file changes
│   └── history/            # Query history persistence
└── scratch/                # Eval harness, models, logs (partly gitignored)
```

## Data flow

### Indexing
1. **Scan** — `pkg/scanner` walks the tree, honours `.gitignore` /
   `.semsearchignore`, and skips hidden dirs, binary files (NUL sniff), and
   minified bundles (line > 20KB).
2. **Chunk** — `pkg/chunker` splits files:
   - `.go/.py/.js/.ts`: AST chunking via tree-sitter (functions, methods,
     structs/classes, types), capped at 80 lines each, with a structural
     metadata prefix (`// [Package | File | Symbol | Kind]`).
   - Markdown: header-based section chunks.
   - Everything else: sliding window (30 lines, step 20).
3. **Embed** — `pkg/embedder` runs batches through ONNX Runtime. Vectors are
   mean-pooled and L2-normalized. Each chunk gets a code vector and a
   description vector (from the LLM description, or the code vector when the
   chunk is small / LLM skipped).
4. **Save** — `pkg/indexer` writes `.semsearch/manifest.json` (metadata) and
   `.semsearch/vectors.bin` (flat `[]float32`, 768 × 4 bytes = 3KB/vector).

### Searching
1. Embed the query with the search_query prefix.
2. Brute-force cosine similarity over all vectors (SIMD), fused with a small
   lexical/semantic weighting.
3. Rerank top-N (e.g. top-20 → top-5).
4. Present file:line ranges; optionally open in an editor.

## LLM descriptions (optional)

`pkg/llm` generates natural-language descriptions for larger chunks (>= 30
lines by default) via a local Ollama model (`qwen3.5:0.8b`). Generation is
concurrent (`SEMSEARCH_LLM_CONCURRENCY`, default 4) and hash-gated through a
per-index cache (`.semsearch/desc_cache.json`), so re-indexing only describes
changed chunks. Description vectors help conceptual queries but are optional —
`--no-llm` skips them and code-only search is unaffected.

## Key design choices

- **Code-only baseline first**: dual vectors (code + desc) stored, but search
  and eval can filter to code-only. This makes `--no-llm` indexes fully
  comparable in eval.
- **Flat vector store**: no ANN index yet; search is O(n) per query, fine for
  hundreds of thousands of chunks, slow at millions.
- **Structural metadata prefix**: prepending package/file/symbol context to
  chunks measurably helps the embedding model (see docs/plan.md for the
  rationale and earlier ablation numbers).

## Scalability notes

- Chunk extraction is single-threaded; very large monorepos index slowly.
- LLM description generation is network-bound on Ollama.
- For large repos: prefer `--no-llm` and index target subdirectories. See
  `scratch/codex_indexing_log.md` for a real-world failure case and the
  mitigations.
