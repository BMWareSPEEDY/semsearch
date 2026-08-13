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
   lexical/semantic weighting (semantic weight 0.65 by default).
3. Rerank top-30 → top-5 (`pkg/reranker`): collapse to the best chunk per
   file, then apply tuned symbol/path/content term-overlap boosts
   (`SYM=0.10`, `PATW=0`, `CONT=0.01`).
4. Present file:line ranges; optionally open in an editor.

## LLM descriptions (optional)

`pkg/llm` generates natural-language descriptions for larger chunks (>= 30
lines by default) via a local Ollama model (`qwen3.5:0.8b`). Generation is
concurrent (`SEMSEARCH_LLM_CONCURRENCY`, default 4) and hash-gated through a
per-index cache (`.semsearch/desc_cache.json`), so re-indexing only describes
changed chunks. Description vectors help conceptual queries; `--no-llm` skips
the LLM and falls back to heuristic synthetic descriptions instead. Note that
`codeOnly` / `docsOnly` filters search by **file type** (Markdown/doc chunks
versus code chunks) and does not disable description-vector similarity — code
chunks are still scored with `max(codeSim, descSim)` from whichever
description vectors the index contains.

## Key design choices

- **Code-only filtering is by file type, not vector source**: dual vectors
  (code + desc) are stored, and `codeOnly` excludes Markdown/doc chunks from
  search while still applying description-vector similarity (`max(code, desc)`)
  to code chunks. This means the LLM description axis is always active for code
  regardless of the `codeOnly`/`docsOnly` filter, and `--no-llm` (synthetic
  descriptions) is a distinct index variant rather than an equivalent one.
- **Flat vector store**: no ANN index yet; search is O(n) per query, fine for
  hundreds of thousands of chunks, slow at millions.
- **Structural metadata prefix**: prepending package/file/symbol context to
  chunks measurably helps the embedding model (see docs/plan.md for the
  rationale and earlier ablation numbers).

## Scalability notes

- Chunk extraction is parallel (bounded worker pool, default `min(CPU, 8)`,
  capped at 64, results merged back in scan order so chunk IDs are
  deterministic). Embedding and LLM description generation are still the
  bottleneck for very large monorepos.
- Keyword statistics (tokens / document frequency / average length for BM25)
  are computed lazily once per index load and cached, so query cost is not
  re-paying tokenization of the whole index.
- Embedding batches are exact-size (never padded with stale rows); batch size
  is tunable via `SEMSEARCH_BATCH_SIZE` (default 32, 16 for models with dim >
  512).
- LLM description generation is network-bound on Ollama.
- For large repos: prefer `--no-llm` and index target subdirectories. See
  `scratch/codex_indexing_log.md` for a real-world failure case and the
  mitigations.
