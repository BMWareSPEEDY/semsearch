# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`semsearch` is a local-first semantic code search engine written in Go. It indexes codebases into vector embeddings using a local ONNX model (Nomic Embed v2) and enables natural language semantic querying via CLI or local HTTP API.

## Commands

| Command | Description |
|---------|-------------|
| `go build -o semsearch .` | Build the binary (outputs to root) |
| `go test ./...` | Run all tests |
| `go test ./pkg/...` | Run package tests only |
| `./semsearch index [path]` | Index a directory (default: current) |
| `./semsearch "query"` | Search indexed codebase |
| `./semsearch serve --port 1921` | Start local HTTP API server |
| `./semsearch status` | Check index status |

## Architecture

```
semsearch/
├── main.go                 # CLI entry point (cobra commands)
├── go.mod                  # Module: github.com/ashishpaliwal/semsearch
├── pkg/
│   ├── scanner/            # File system traversal with .gitignore/.semsearchignore
│   ├── chunker/            # Text/Markdown chunking (sliding window + header-based)
│   ├── embedder/           # ONNX Runtime + tokenizer (Nomic Embed v2, 768-dim)
│   └── indexer/            # Vector storage (manifest.json + vectors.bin) + cosine search
├── scratch/model/          # ONNX model + tokenizer.json
└── .semsearch/             # Generated index (gitignored)
```

### Key Data Flow

1. **Index**: Scan → Chunk → Embed (batch) → Save manifest + vectors
2. **Search**: Load index → Embed query → Cosine similarity (SIMD) → Rank → Present

### Dependencies

- `github.com/spf13/cobra` - CLI framework
- `github.com/monochromegane/go-gitignore` - .gitignore parsing
- `github.com/yalue/onnxruntime_go` - ONNX Runtime CGO bindings

### Model Requirements

- ONNX model at `scratch/model/model_quantized.onnx`
- Tokenizer at `scratch/model/tokenizer.json`
- Requires ONNX Runtime shared library (macOS: `/opt/homebrew/Cellar/onnxruntime/1.28.0_1/lib/libonnxruntime.dylib`)

## Code Style Notes

- Main CLI logic in `main.go` (root), not `cmd/semsearch/main.go`
- Chunking uses sliding window (30 lines, 20 step) for code, header-based for Markdown
- Embeddings use mean pooling + L2 normalization
- Vector storage: flat `[]float32` (768 dims × 4 bytes = 3KB/vector)
- Search: brute-force cosine similarity (no ANN index yet)