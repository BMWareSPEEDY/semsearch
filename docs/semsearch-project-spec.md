# semsearch — Project Spec & Initial Plan

Personal notes on what I'm building, why, and how the piece fits together. Spent about a month putting this together to solve something that's been bugging me every time I browse an unfamiliar codebase.

---

## 1. The Problem

- `grep` and text search only work if you already know the exact identifier or keyword.
- If you don't remember whether the function was called `retryWithBackoff`, `attemptRequest`, or `resilientFetch`, text search returns nothing.
- Cloud tools (Sourcegraph Cody, Cursor) work well, but they require third-party services and send code off-machine.
- Want a tool that:
  - Runs 100% locally on my machine (no network or external API needed for search).
  - Understands concepts and plain English queries ("where do we handle token expiry").
  - Works straight from the terminal or as a tiny local HTTP API.

---

## 2. Core Idea

- **Scan & Chunk:** Walk the repo, respect `.gitignore`, and split files into logical code chunks (functions/types via tree-sitter, sections for markdown, sliding window as fallback).
- **Embed:** Generate vector embeddings for each code chunk using a local ONNX model (CodeMiniLM / Nomic).
- **Search:** Embed the query with the same model, compute cosine similarity, blend with BM25 keyword matching so exact symbol matches don't get lost, and return ranked results with line numbers.
- **Serve:** Expose the exact same search logic over a local HTTP port (`localhost:1921`) so I can hook it up to editor scripts or curl.

---

## 3. Key Design Decisions

- **Language:** Go. Compiles to a single fast binary, handles concurrency easily across cores, quick CLI startup time.
- **Inference:** ONNX Runtime via CGO (`yalue/onnxruntime_go`) so embeddings run locally on CPU without needing a Python environment.
- **Dual vectors:** Code and natural language don't always align closely in vector space. Chunks get both their raw code embedded and an optional plain-English summary (via a local Ollama model if running). A query matches whichever score is higher.
- **Hybrid search:** Combine vector cosine similarity with BM25 keywords so exact function names stay sharp while conceptual queries work.
- **Storage:** Simple flat binary file for vectors (`vectors.bin`) and JSON for chunk metadata (`manifest.json`). At single-repo scale (tens of thousands of chunks), brute-force cosine search takes just a few milliseconds — no need for a complex vector DB.

---

## 4. Initial Plan & Milestones

- [x] Basic scanner + sliding window chunker + ONNX embedder pipeline.
- [x] CLI commands (`index`, search query, `serve`).
- [x] Tree-sitter AST chunking for Go, Python, TS/JS, Rust.
- [x] Local LLM description generation via Ollama + caching to improve conceptual recall.
- [x] Reranker: per-file deduplication + term-overlap boosts.
- [x] Watcher mode (`semsearch watch`) for automatic incremental re-indexing on file edits.
- [x] Eval suite: test against real repos (gin, cobra, mux, clap, hugo) to measure real-world hit rates.
- [x] Performance tuning: batched tokenization, single-pass file scanning, and cached keyword statistics.

---

## 5. What I'm NOT Building

- Not an AI chat assistant or code generator — this only searches and locates existing code.
- Not a hosted cloud product — strictly local-first.
- Not adding heavy database dependencies — keep it lightweight and self-contained.
