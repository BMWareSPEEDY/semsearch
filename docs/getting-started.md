# semsearch — Getting Started

`semsearch` is a local-first semantic code search engine written in Go. It
indexes a codebase into vector embeddings using a local ONNX model and lets
you query it in natural language from the CLI or a local HTTP API. Everything
runs on your machine — no cloud, no network required for search.

## Prerequisites

- Go 1.22+
- An ONNX embedding model. The repo is tested against:
  - `scratch/code_minilm/model_quantized.onnx` + `tokenizer.json` (CodeMiniLM, 384-dim) — used for eval
  - `scratch/model/model_quantized.onnx` + `tokenizer.json` (Nomic Embed v2, 768-dim) — used by the CLI
- ONNX Runtime shared library (macOS: `brew install onnxruntime`; the code
  looks for `libonnxruntime.dylib` under Homebrew).

## Build

```bash
go build -o semsearch .
```

## Usage

```bash
# Index the current directory (or pass a path)
./semsearch index [path]

# Search the indexed codebase with a natural language query
./semsearch "query"

# Start a local HTTP API server
./semsearch serve --port 1921

# Check index status
./semsearch status

# Re-index on file changes (incremental watcher)
./semsearch watch

# Filter search to code files or docs only
./semsearch "query" --code-only
./semsearch "query" --docs-only
```

The index is written to `.semsearch/` inside the target directory
(`manifest.json` + `vectors.bin`).

## Indexing a large repo

```bash
# Skip LLM description generation (fast, code-only search still works)
./semsearch index /path/to/repo --no-llm

# Use local LLM descriptions (Ollama, qwen3.5:0.8b by default)
./semsearch index /path/to/repo
```

Environment variables:

| variable                    | default | purpose                                     |
|-----------------------------|---------|---------------------------------------------|
| `SEMSEARCH_LLM_MODEL`       | `qwen3.5:0.8b` | Ollama model used for descriptions   |
| `SEMSEARCH_LLM_CONCURRENCY` | `4`     | parallel LLM description workers             |
| `SEMSEARCH_MIN_DESC_LINES`  | `30`    | only chunks with >= this many lines get LLM descs |

## What's ignored

The scanner skips: `.git`, `node_modules`, `dist`, `build`, `target`,
`vendor`, hidden dotfiles, binary files (NUL-byte sniff), minified bundles
(lines > 20KB), and common lockfiles. It also honours `.gitignore` and
`.semsearchignore`.

## Known limits

- Very large monorepos (thousands of files) index slowly — chunk extraction is
  single-threaded. Use `--no-llm` and index target subdirectories.
- Large mixed doc+code indexes (10K+ chunks) have lower conceptual recall.
  See `scratch/FINDINGS.md` for a detailed "works / doesn't work" writeup.
