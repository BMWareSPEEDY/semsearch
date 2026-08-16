# semsearch

Local semantic code search. Search your codebase in plain English and find functions, types, and logic based on what they do rather than exact variable names.

Everything runs on your machine — no cloud services, no remote APIs, and your code never leaves your computer.

---

## Why I Built This

Standard `grep` and text search only work if you already know the exact identifier or keyword you're looking for. If someone named the function `resilientFetch` and you search for `"retry failed requests"`, you get zero hits.

Tools like Sourcegraph Cody or Cursor do semantic search, but they're either tied to a specific editor, require subscriptions, or send your source code to external servers. I wanted a standalone CLI and lightweight local HTTP server that works with any editor or terminal, completely offline.

---

## Features

- **Semantic + Keyword Hybrid Search:** Combines vector embeddings with BM25 keyword matching so exact symbol lookups remain accurate while conceptual queries still work.
- **Language-aware AST Chunking:** Uses tree-sitter to break code down into logical units (functions, methods, classes) across Go, Python, TypeScript/JavaScript, Rust, and more.
- **Dual Vector Matching:** Embeds both raw code and optional plain-English summaries so queries can match either the code itself or the concept it implements.
- **Local HTTP API:** Run `semsearch serve` and query search results from editor plugins, shell scripts, or browsers.
- **Live File Watcher:** Keeps your index up-to-date in the background as you write code.

---

## Quickstart

### Prerequisites
- Go 1.22+
- ONNX Runtime (`brew install onnxruntime` on macOS)
- An ONNX embedding model (like CodeMiniLM or Nomic Embed v2)

### Install & Build

```bash
git clone https://github.com/BMWareSPEEDY/semsearch.git
cd semsearch
go build -o semsearch .
```

### Usage

```bash
# Index current repository
./semsearch index .

# Fast index (skip local LLM description generation)
./semsearch index . --no-llm

# Search in natural language
./semsearch "where do we handle JWT authentication and validation"

# Search code only or docs only
./semsearch "markdown rendering" --code-only
./semsearch "setup instructions" --docs-only

# Start local HTTP server
./semsearch serve --port 1921

# Watch mode (updates index on file changes)
./semsearch watch
```

---

## Benchmarks & Accuracy

Tested across 5 popular open source repositories against 173 test queries with known ground truth files:

| Repo | Chunks | Top-5 Hit Rate |
|---|---|---|
| `gin-gonic/gin` | ~890 | 41/42 (98%) |
| `spf13/cobra` | ~520 | 27/27 (100%) |
| `gorilla/mux` | ~165 | 25/25 (100%) |
| `clap-rs/clap` | ~7,250 | 31/40 (78%) |
| `gohugoio/hugo` | ~11,500 | 31/39 (79%) |
| **Total** | **~20,000** | **~155/173 (90%)** |

---

## Configuration

These environment variables can be used to tweak behavior:

| Variable | Default | Description |
|---|---|---|
| `SEMSEARCH_LLM_MODEL` | `qwen3.5:0.8b` | Ollama model for chunk descriptions |
| `SEMSEARCH_LLM_CONCURRENCY` | `4` | Parallel workers for generating descriptions |
| `SEMSEARCH_SEM_WEIGHT` | `0.65` | Weight between semantic similarity and keyword score |
| `SEMSEARCH_BATCH_SIZE` | `32` | Batch size during embedding inference |
| `SEMSEARCH_CHUNK_WORKERS` | `min(CPU, 8)` | Worker count for parallel AST chunking |

---

## Documentation

- [Architecture Overview](docs/architecture.md) — How the scanning, chunking, embedding, and reranking pipelines work.
- [Getting Started](docs/getting-started.md) — Detailed setup, commands, and options.
- [Evaluation](docs/evaluation.md) — Benchmark breakdown and testing methodology.
- [Project Spec & Notes](docs/semsearch-project-spec.md) — Original plan and design decisions.

---

## License

MIT
