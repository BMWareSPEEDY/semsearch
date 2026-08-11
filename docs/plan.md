# Comprehensive Architectural & Implementation Plan: `semsearch`

`semsearch` is a high-performance, local-first, editor-agnostic semantic code search engine and local HTTP service. It indexes your codebase into vector embeddings using a local ONNX model and enables natural language semantic querying over local terminal sessions, scripts, and editor integrations.

This document outlines the end-to-end technical decisions, system architecture, data flow, embedding models, language support, file type coverage, index structures, component specifications, edge-case handling, and phase-by-phase implementation roadmap.

---

## 1. Core Technical Decisions & Technology Stack

### 1.1 Implementation Language: **Go (Golang)**
- **Rationale:**
  1. **Single Static Binary Distribution:** Compiles down to a single binary with zero external dependencies (no Python runtime, no Node.js runtime, no dynamic library paths).
  2. **High-Performance Concurrency:** Goroutines and channels allow concurrent file system traversal, tree-sitter AST parsing, and chunking across CPU cores with minimal overhead.
  3. **Low Latency & Fast Cold-Start:** Sub-50ms startup time for CLI queries compared to Python/Node-based wrappers.
  4. **Robust Native Standard Library:** Native `net/http` server, robust JSON marshaling, and file system primitives.

### 1.2 Embedding Model Runtime: **Evaluated Micro-Benchmark (`yalue/onnxruntime_go` vs `saucecontrol/ort`)**
- **Evaluation Strategy:** During Phase 1 PoC implementation, run a micro-benchmark comparing:
  1. **`yalue/onnxruntime_go` (CGO wrapper around native ONNX C library):** Expected higher inference throughput leveraging official C/C++ SIMD optimizations (AVX2/ARM Neon), requiring dynamic/static linking of shared runtime binaries.
  2. **`saucecontrol/ort` (Pure Go runtime):** Pure Go port enabling CGO-free cross-compilation and simpler binary builds, evaluated against `nomic-embed-text-v1.5` execution speed.
- **Selection Criteria:** Choose `yalue/onnxruntime_go` as default if batch throughput is >2x faster for indexing, or `saucecontrol/ort` if throughput is comparable (<25% difference) to benefit from pure Go distribution.
- **Execution Strategy:** Download pre-quantized (`.onnx`) model weights during initialization/first run into `~/.cache/semsearch/models/`.

### 1.3 Primary Embedding Model: **Nomic Embed v2 (`nomic-embed-text-v1.5` / `nomic-embed-text`)**
- **Model Specification:**
  - **Model Identifier:** `nomic-embed-text-v1.5` (Quantized ONNX format).
  - **Vector Dimension:** 768 dimensions (Float32).
  - **Context Window:** 8,192 tokens (Extended long-context support).
  - **Task Prefixes (Required by Nomic Embed):**
    - Search Queries: `search_query: <query_string>`
    - Code / Document Chunks: `search_document: <chunk_text>`
  - **Reasoning & Advantages:** 
    1. **Extended Context Window (8k tokens):** Can capture full functions, multi-function blocks, and long module docstrings without clipping.
    2. **High Quality Code & Multilingual General Representation:** Superior semantic mapping between plain English intent and raw code implementation.
    3. **Matryoshka Representation Learning (MRL):** Supports flexible dimension truncation (e.g. down to 256 or 512) if memory footprint optimization is desired.

---

## 2. Supported Programming Languages & File Extensions

`semsearch` categorizes project files into two indexing tiers: **Language-Aware AST AST Parsing** (Primary) and **Fallback Text/Document Chunking** (Secondary).

### 2.1 Tier 1: Primary AST-Parsed Programming Languages (via Tree-Sitter)

| Language | Extensions | Tree-Sitter Grammar | Extracted AST Symbols |
| :--- | :--- | :--- | :--- |
| **Go** | `.go` | `tree-sitter-go` | Functions, Methods, Structs, Interfaces, Package Docs |
| **TypeScript / JavaScript** | `.ts`, `.tsx`, `.js`, `.jsx`, `.mjs`, `.cjs` | `tree-sitter-typescript` / `tree-sitter-javascript` | Functions, Arrow Functions, Classes, Interfaces, Methods, Exports |
| **Python** | `.py`, `.pyi` | `tree-sitter-python` | Def functions, Async def, Classes, Module Docstrings, Decorators |
| **Rust** | `.rs` | `tree-sitter-rust` | Fn functions, Impl blocks, Structs, Enums, Traits, Macros |
| **C / C++** | `.c`, `.h`, `.cpp`, `.hpp`, `.cc`, `.cxx` | `tree-sitter-cpp` | Functions, Methods, Classes, Structs, Namespaces |
| **Java** | `.java` | `tree-sitter-java` | Classes, Interfaces, Methods, Enums, Annotations |
| **C#** | `.cs` | `tree-sitter-c-sharp` | Classes, Interfaces, Methods, Properties, Namespaces |
| **PHP** | `.php` | `tree-sitter-php` | Functions, Classes, Methods, Trait declarations |
| **Ruby** | `.rb` | `tree-sitter-ruby` | Methods (`def`), Classes (`class`), Modules (`module`) |
| **Swift** | `.swift` | `tree-sitter-swift` | Functions, Structs, Classes, Protocols, Extensions |
| **Kotlin** | `.kt`, `.kts` | `tree-sitter-kotlin` | Functions (`fun`), Classes, Objects, Interfaces |

---

### 2.2 Tier 2: Text, Documentation, & Configuration File Types (Sliding Window Fallback)

| Category | File Extensions | Chunking Logic |
| :--- | :--- | :--- |
| **Documentation & Guides** | `.md`, `.markdown`, `.rst`, `.txt`, `.adoc` | Split by Markdown Headers (`#`, `##`) and 400-token sliding window |
| **Configuration Files** | `.json`, `.yaml`, `.yml`, `.toml`, `.xml`, `.ini`, `.env.example` | Block/Section-based chunking (~300 tokens) |
| **Build & Infrastructure** | `Dockerfile`, `Makefile`, `.sh`, `.bash`, `.zsh`, `CMakeLists.txt`, `k8s/*.yaml` | Paragraph and script target block chunking |
| **Web & Styling Templates** | `.html`, `.htm`, `.css`, `.scss`, `.sass`, `.vue`, `.svelte` | Element/Block semantic fallback chunking |

---

### 2.3 Excluded / Ignored File Types (Hardcoded Safety Rules)

- **Binary / Compressed Media:** `.png`, `.jpg`, `.jpeg`, `.gif`, `.ico`, `.pdf`, `.zip`, `.tar`, `.gz`, `.7z`, `.exe`, `.dll`, `.so`, `.dylib`, `.wasm`.
- **Lock & Generated Files:** `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `Cargo.lock`, `go.sum`, `poetry.lock`.
- **Build Artifacts & Cache:** `.git/`, `.semsearch/`, `node_modules/`, `dist/`, `build/`, `target/`, `__pycache__/`, `.venv/`.

---

## 3. Detailed Architecture & Data Pipeline

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           semsearch CLI / API                           │
└─────────────────────────────────────────────────────────────────────────┘
        │                                                    │
        ▼ (Index Phase)                                      ▼ (Search Phase)
┌───────────────────────────┐                        ┌───────────────────────────┐
│ File System Traversal     │                        │ User Natural Query        │
│ (.gitignore / ignore filter)                       │ (Prefixed: search_query:) │
└───────────────────────────┘                        └───────────────────────────┘
        │                                                          │
        ▼                                                          ▼
┌───────────────────────────┐                        ┌───────────────────────────┐
│ Chunking Engine           │                        │ ONNX Embedding Engine     │
│ (Tree-Sitter + Fallback)  │                        │ (Nomic Embed v2 - 768d)   │
└───────────────────────────┘                        └───────────────────────────┘
        │                                                          │
        ▼                                                          ▼
┌───────────────────────────┐                        ┌───────────────────────────┐
│ Context & Prefix Ingestion│                        │ SIMD Cosine Vector Search │
│ (Prefixed: search_document:)                       │ Ranker (Top-N Selection)  │
└───────────────────────────┘                        └───────────────────────────┘
        │                                                          │
        ▼                                                          ▼
┌───────────────────────────┐                        ┌───────────────────────────┐
│ ONNX Embedding Engine     │                        │ Result Presenter          │
│ (Batch Chunks Inference)  │                        │ (CLI Formatter / JSON API)│
└───────────────────────────┘                        └───────────────────────────┘
        │
        ▼
┌───────────────────────────┐
│ Persistence Layer         │
│ (.semsearch/index.bin)    │
└───────────────────────────┘
```

---

## 4. Component Deep Dive & Data Models

### 4.1 Data Structures & Index Serialization

#### Metadata & Index Manifest (`.semsearch/manifest.json`)
```json
{
  "version": "1.0",
  "created_at": "2026-08-10T17:05:00Z",
  "model": "nomic-embed-text-v1.5",
  "vector_dimensions": 768,
  "total_chunks": 3412,
  "indexed_files_count": 218,
  "chunks": [
    {
      "id": 0,
      "file_path": "src/http/client.go",
      "symbol_name": "ResilientFetch",
      "kind": "function",
      "line_start": 45,
      "line_end": 78,
      "content_hash": "a8f5c...7e2",
      "preview": "func ResilientFetch(url string, opts FetchOptions) (*Response, error) {..."
    }
  ]
}
```

#### Vector Binary File (`.semsearch/vectors.bin`)
- Fixed-stride binary blob: Array of contiguous `float32` arrays.
- Offsets directly map to chunk index IDs (`id * vector_dimensions * 4 bytes = id * 768 * 4 bytes`).
- Allows fast memory-mapped (`mmap`) reads or direct loading into memory in a single contiguous RAM allocation (`[]float32`).

---

### 4.2 Vector Storage & Search Performance Strategy

- **Memory Calculations for 768-dim Vectors (Nomic Embed v2):**
  - $768 \times 4\text{ bytes} = 3,072\text{ bytes} \approx 3.0\text{ KB per vector}$.
  - 10,000 chunks require **~30.7 MB** of RAM.
  - 50,000 chunks require **~153.6 MB** of RAM.
- **Search Strategy:**
  - Flat brute-force Cosine Similarity with SIMD (AVX2 / ARM Neon vectorization).
  - Sub-25ms response time on 30,000 vectors on modern CPU cores.

---

## 5. CLI & Local HTTP Server Specification

### 5.1 CLI Commands & Flags

```bash
# Index current directory
semsearch index .
semsearch index /path/to/project --exclude "docs/*" --rebuild

# Natural language semantic query
semsearch "where do we retry failed API requests"
semsearch "jwt validation logic" --top 10 --json
semsearch "database connection pooling setup" --min-score 0.65

# Local Server Management
semsearch serve --port 1921
semsearch serve --port 1921 --daemon

# Status check
semsearch status
```

### 5.2 Local HTTP API Specification

- **Host Binding:** Strictly `127.0.0.1` (localhost only).
- **Endpoints:**
  1. `GET /health`: Returns service status and loaded index metrics.
  2. `GET /search?q=<query>&top=<limit>&min_score=<score>`: Returns ranked array of search results in JSON format.
  3. `POST /index`: Triggers asynchronous background re-indexing of the workspace.
  4. `GET /index/status`: Returns current progress of background indexing operation.

---

## 6. Phase-by-Phase Implementation Roadmap

### Phase 1: PoC Validation & Model Benchmark Harness
1. Download ONNX quantization of `nomic-embed-text-v1.5` and verify Go ONNX Runtime bindings.
2. Verify tokenization with HuggingFace `tokenizer.json` parser in Go.
3. Test Nomic `search_query:` vs `search_document:` prefix behavior on a sample codebase.

### Phase 2: File Traversal & Multi-Language Chunking Pipeline
1. Build CLI scaffold using Go (`spf13/cobra`).
2. Build file scanner with recursive `.gitignore` and `.semsearchignore` processing.
3. Integrate Tree-Sitter grammars for Tier 1 languages (Go, TS/JS, Python, Rust, C/C++, Java, C#, PHP, Ruby, Swift, Kotlin).
4. Implement Tier 2 sliding-window fallback chunker for Markdown and configuration files.

### Phase 3: Embedding Generator & Vector Persistence Layer
1. Implement concurrent Goroutine batching for ONNX vector inference.
2. Build `.semsearch/vectors.bin` float32 storage writer/reader and `manifest.json` metadata generator.
3. Build SIMD-accelerated flat brute-force Cosine Similarity search engine.

### Phase 4: CLI Formatting, HTTP API & Background Daemon
1. Implement terminal result renderer (colorized code previews, file paths, line numbers, scores).
2. Implement `--json` machine-readable output mode.
3. Build `net/http` local server exposing `/search`, `/health`, `/index`, `/index/status`.
