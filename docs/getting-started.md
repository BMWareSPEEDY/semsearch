# Getting Started

Quick guide to building and using `semsearch`.

## Prerequisites

- **Go 1.22+**
- **ONNX Runtime** (on macOS: `brew install onnxruntime`)
- **Embedding model:** An ONNX model + tokenizer (tested with CodeMiniLM and Nomic Embed v2) placed where `semsearch` can find it.
- *(Optional)* **Ollama** running locally if you want LLM-generated chunk descriptions (`ollama serve`).

## Installation

```bash
# Clone and build
git clone https://github.com/BMWareSPEEDY/semsearch.git
cd semsearch
go build -o semsearch .
```

## Basic Usage

```bash
# 1. Index your project
./semsearch index .

# Index without Ollama (faster, skips description generation)
./semsearch index . --no-llm

# 2. Search using plain English
./semsearch "where do we parse HTTP request headers"

# Filter to code files only or docs only
./semsearch "routing table setup" --code-only
./semsearch "deployment instructions" --docs-only

# 3. Start local HTTP API server
./semsearch serve --port 1921

# 4. Watch for file changes and auto-update index
./semsearch watch
```

## Useful Config

You can set these via environment variables if you want to tweak things:

- `SEMSEARCH_LLM_MODEL` — Ollama model to use (default: `qwen3.5:0.8b`).
- `SEMSEARCH_LLM_CONCURRENCY` — Parallel workers for descriptions (default: `4`).
- `SEMSEARCH_SEM_WEIGHT` — Balance between semantic similarity and keyword matching (default: `0.65`).
- `SEMSEARCH_BATCH_SIZE` — Batch size during embedding (default: `32`).
