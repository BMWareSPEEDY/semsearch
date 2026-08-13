# semsearch

> Local semantic code search. Find code by describing what it does — not by remembering what it's called.

---

## The Problem

`grep` and "find in files" only match exact text. If you don't remember the exact function name, variable, or wording used in the code, you get nothing back — even when the code you're looking for exists and does exactly what you're describing.

This happens constantly on any codebase you didn't write every line of yourself, or one you wrote months ago and no longer remember the naming conventions of. You know *what* the code does. You don't know what it's literally called. Searching for `"retry failed api call"` finds nothing if the actual function is named `resilientFetch`.

Existing semantic search tools that solve this (Sourcegraph Cody, Cursor's codebase indexing) are cloud-tied, IDE-embedded, and often require sending your code to a third-party service. There wasn't a local-only, standalone, editor-agnostic option — something that just runs on your machine, indexes once, and answers plain-English queries without your code ever leaving your device.

`semsearch` is that tool.

---

## What It Does

Point it at a codebase. It indexes every function, class, and code block into vector embeddings — a numerical representation of what each chunk of code *means*, not just what it says. Then you search in plain English:

```
semsearch "where do we handle JSON body binding and validation errors"
```

...and it returns the actual relevant code, ranked by meaning, even if none of your words appear literally in the code.

It also handles exact lookups just as well — searching for a real function name finds that function immediately, so you're not trading one kind of search for another.

Everything runs locally: the embedding model, the index, the search itself. No account, no API key required for the core search, no code sent anywhere.

---

## How It Works (High Level)

1. **Chunking** — the codebase is parsed into meaningful units (functions, classes) using language-aware parsing, not arbitrary line splits.
2. **Embedding** — each chunk is converted into a vector using a small local embedding model. Plain-English queries are embedded the same way, so they can be compared directly.
3. **Semantic descriptions** — because raw code and plain-English queries don't naturally sit close together in vector space (a query like "retries failed requests" reads very differently from the actual code that does it), each chunk also gets a short, auto-generated natural-language description at index time. Both the code and its description are embedded, and a query is matched against whichever one it's closer to.
4. **Hybrid ranking** — semantic similarity is blended with keyword matching, so exact identifier searches stay sharp while conceptual searches actually work.
5. **Local API** — the same search is exposed over a local HTTP server, so it's not limited to the terminal — editor plugins, scripts, or other tools can query it too.

---

## Why This Approach

Semantic code search is a harder problem than semantic text search. Code and natural language are, in a real sense, two different modalities — a block of code and a sentence describing it don't naturally embed close together, even with a good model. Most of the engineering effort in this project went into closing that gap deliberately (auto-generated descriptions, careful chunk boundaries, hybrid scoring) rather than assuming a bigger model would solve it on its own — testing showed it largely doesn't.

This project was built with a strict local-first constraint: no cloud dependency for the core search path, no mandatory account, and no compromise on that even when it would have been easier.

---

## Benchmarks

The honest way to evaluate a search tool is against code you didn't write. So the tool is tested on five real, external open-source repositories — gin, cobra, gorilla/mux, clap, and hugo — with 173 hand-written queries in total across the five repos, each with a known-correct answer file. The queries are a mix of *conceptual* phrases ("rendering markdown content to html") and *exact-symbol* lookups, and a query counts as a hit only if the expected file lands in the top results.

Measured with the CodeMiniLM 384-dim model, semantic weight 0.65, and the tuned reranker (per-file dedup, shortlist 30):

| repo            | chunks | queries | top-1   | top-3   | top-5   |
|-----------------|--------|---------|---------|---------|---------|
| gin-gonic/gin   | 890    | 42      | 25/42   | 39/42   | 41/42 (98%) |
| spf13/cobra     | 521    | 27      | 22/27   | 24/27   | 27/27 (100%) |
| gorilla/mux     | 165    | 25      | 21/25   | 25/25   | 25/25 (100%) |
| clap-rs/clap    | 7,256  | 40      | 11/40   | 22/40   | 31/40 (78%) |
| gohugoio/hugo   | 11,514 | 39      | 16/39   | 25/39   | 32/39 (82%) |
| **total**       |        | **173** | 95/173  | 135/173 | **156/173 (90%)** |

A few honest observations:

- **Exact-symbol queries are the strongest category** — if you know the name, it finds it fast.
- **Conceptual queries are the weakest** — those that don't lexically overlap any code or docstring are where it still misses.
- **Small, focused repos hit 100%.** cobra and gorilla/mux are now perfect at top-5.
- **A note on the jump from 85% to 90%:** some of that gain is a *methodology* fix, not pure ranking improvement. A pass over the test set found eight "expected answer" labels that pointed at the wrong or aggregate file — for example, cobra's `Command` struct lives in `command.go`, not the top-level `cobra.go` wrapper. Those labels were corrected (the queries were right; the ground truth wasn't). The rest came from a new reranker parity mode that keeps only the best chunk per file in the final ordering. Both are worth stating plainly: eval numbers only mean something when the ground truth is actually correct.
- One of the 17 remaining misses is literally unfixable: a gin "CORS middleware" query whose answer doesn't exist in that checkout.

The full methodology and the complete "works / doesn't work" writeup are in `docs/evaluation.md` and `scratch/FINDINGS.md`.

---

## Design Principles

- **Local-first, always.** Your code never leaves your machine as part of the core search functionality.
- **Verifiable, not opaque.** Every result comes with a real file, line number, and similarity score — you can see why something matched, not just trust an opinion.
- **Minimal surface.** One tool, one job: turn a description into the right piece of code.

---

## Status

Actively developed. The core loop — index, search, serve, watch — is stable and covered by unit tests (`go test ./pkg/...`). Development so far has moved in four visible milestones: v1 shipped the full scan → chunk → embed → search pipeline; v2 overhauled semantic recall (doc-comment chunking, dual code/description vectors, targeted LLM descriptions); v3 added per-file dedup and a rebalanced reranker that lifted eval accuracy to 85% top-5; v4 corrected eight wrong eval ground-truth labels, added a reranker parity mode for per-file dedup in the final ordering, and brought eval accuracy to 156/173 (90%) top-5 across the five eval repos.

The `scratch/` directory contains the evaluation harness and experiment code — it's a working notebook, not a library, and it's largely gitignored. It's kept because it's genuinely useful for reproducing benchmark numbers.

---

## License

MIT. See `LICENSE`.

---

## Installation

### Prerequisites

- **Go 1.22 or newer** (the module targets `go 1.26`).
- **ONNX Runtime shared library.** On macOS: `brew install onnxruntime`. The code currently locates the library via a Homebrew Cellar path in `pkg/embedder/embedder.go`, so if you install a different version you may need to point it at your `libonnxruntime.dylib`.
- **An embedding model.** The repo ships with no model checked in (they're large and gitignored). The CLI expects one at `scratch/code_minilm/model_quantized.onnx` with `scratch/code_minilm/tokenizer.json`. The tested model is **CodeMiniLM** (`isuruwijesiri/all-MiniLM-L6-v2-code-search-512` on Hugging Face, 384-dim) — export it to ONNX and drop the two files in `scratch/code_minilm/`.
- **Optional — Ollama** (`ollama serve`) for LLM-generated semantic descriptions at index time. Not required; without it the tool falls back to heuristic synthetic descriptions.

### Build

```bash
git clone https://github.com/BMWareSPEEDY/semsearch.git
cd semsearch
go build -o semsearch .
```

That's it — a single binary. The embedding model, index, and search all run locally.

---

## Quickstart

### 1. Index a repo

```bash
./semsearch index /path/to/your/code
```

Without Ollama running, add `--no-llm` (or just accept the automatic fallback). The index is written to `.semsearch/` inside the target directory.

### 2. Search in plain English

```bash
./semsearch "scanner honors gitignore and skips node_modules"
```

Real output, searching semsearch's own codebase:

```
Search Results for query: 'scanner honors gitignore and skips node_modules'
==================================================

1. pkg/scanner/scanner.go:57-85 (score: 0.6864)
--------------------------------------------------
   // [Package: scanner | File: scanner.go | Symbol: NewFileScanner | Kind: function]
   func NewFileScanner(rootDir string) (*FileScanner, error) {
   	abs...

2. docs/getting-started.md:68-74 (score: 0.6162)
--------------------------------------------------
   ## What's ignored

   The scanner skips: `.git`, `node_modules`, `dist`, `build`, `target`,
   `vendor`, hidden dotfiles, binary files (NUL-byte sniff), ...

3. pkg/chunker/ast_chunker.go:157-218 (score: 0.4890)
--------------------------------------------------
   // [Package: chunker | File: ast_chunker.go | Symbol: walkAST | Kind: function]
   func (a *ASTChunker) walkAST(filePath string, node *sitter.Node, li...
==================================================
```

The file you actually wanted — `pkg/scanner/scanner.go` — is right at the top, even though your query shared almost no literal words with the code. Each result gives you the file, the line range, and a score so you can judge the match yourself.

Jump straight into your editor with `--editor code` (or `vim`/`subl`), or add `--context 5` to see surrounding lines.

### 3. Serve it over HTTP

```bash
./semsearch serve --port 1921
```

Binds to `127.0.0.1` only, by default. Then:

```bash
curl "http://127.0.0.1:1921/api/search?q=skip+binary+files&top=3"
```

### 4. Other useful commands

```bash
./semsearch status                  # index stats: model, chunk count, last indexed
./semsearch history                 # past queries and files you opened from them
./semsearch watch                   # keep the index fresh as you edit (incremental)
./semsearch index . --no-llm        # fast indexing without Ollama descriptions
```

---

## CLI Reference

| Command / flag | Description |
|----------------|-------------|
| `semsearch index [path]` | Build or rebuild the index for a directory (default: current) |
| `  --no-llm` | Skip LLM description generation; fall back to heuristic synthetic descriptions |
| `semsearch [path] <query>` | Search an indexed codebase with a natural-language query |
| `  -t, --top N` | Number of results to return (default 5) |
| `  -m, --min-score X` | Drop results below a similarity score threshold |
| `  -c, --context N` | Print N lines of surrounding context around each match |
| `  -e, --editor APP` | Open the top result in an editor (`code`, `vim`/`nvim`, `subl`) |
| `  --code-only` | Search implementation files only (excludes Markdown/docs) |
| `  --docs-only` | Search documentation files only |
| `  -j, --json` | Output results as JSON |
| `semsearch serve [path]` | Start the local HTTP API |
| `  -p, --port N` | Port to bind (default 1921, always `127.0.0.1`) |
| `semsearch watch [path]` | Incrementally re-index changed files (fsnotify, 500ms debounce) |
| `semsearch history [path]` | Show past queries and opened results |
| `semsearch status [path]` | Show index stats |

---

## HTTP API Reference

All endpoints bind to `127.0.0.1`. `GET` requests, JSON responses.

| Endpoint | Description |
|----------|-------------|
| `GET /api/search?q=<query>&top=<n>` | General search (also `/search`) |
| `GET /api/code?q=<query>` | Code-only search (also `/api/search?mode=code`) |
| `GET /api/docs?q=<query>` | Docs-only search (also `/api/search?mode=docs`) |
| `GET /api/status` | Index status: chunks, files, model (also `/health`) |

Query parameters: `q` or `query` (required), `top` or `limit`, and `mode` (`code`|`docs`) or `code_only=true` / `docs_only=true`.

Example:

```bash
curl "http://127.0.0.1:1921/api/search?q=retry+failed+api+requests&top=2"
```

```json
{
  "query": "retry failed api requests",
  "results": [
    {
      "chunk": {
        "id": 128,
        "file_path": "src/http/client.go",
        "symbol_name": "resilientFetch",
        "kind": "function_declaration",
        "line_start": 84,
        "line_end": 102,
        "content_hash": "1a2b3c4d",
        "content": "...",
        "description": "...",
        "preview": "func resilientFetch(url string, opts Options) { ..."
      },
      "score": 0.77
    }
  ]
}
```

---

## Configuration

Everything is tunable through environment variables; there is no config file.

| Variable | Default | Purpose |
|----------|---------|---------|
| `SEMSEARCH_LLM_MODEL` | `qwen3.5:0.8b` | Ollama model used to generate chunk descriptions |
| `SEMSEARCH_LLM_CONCURRENCY` | `4` | Parallel LLM description workers |
| `SEMSEARCH_MIN_DESC_LINES` | `30` | Only chunks with at least this many lines get an LLM description |
| `SEMSEARCH_MAX_LEN` | `128` | Tokenizer max sequence length (32–512) |
| `SEMSEARCH_SEM_WEIGHT` | `0.65` | Weight of semantic similarity vs. keyword signal in ranking (CLI) |
| `SEMSEARCH_FUSION` | *(unset)* | Alternative fusion strategies for experimentation: `rrf`, `mult`, `avg` |

Reranker tuning knobs (used during eval; defaults shown): `SYM=0.10`, `PATW=0.0`, `CONT=0.01`, `STABW=0.0`, `SEG=0`, and `MFD` — per-file dedup mode, default `1` (collapse the shortlist to the best chunk per file before boosting), with `2` for dedup after boosting and `0` to disable. You almost certainly don't need these — they exist because finding the right boost weights was a measured, iterative process (`scratch/reranker-tuning-log.md`).

---

## Architecture & Internals

A deeper write-up lives in [`docs/architecture.md`](docs/architecture.md). The short version:

```
index:  scan → chunk (tree-sitter / markdown / sliding window)
              → embed (code vector + description vector per chunk)
              → save (.semsearch/manifest.json + vectors.bin)
search: embed query → hybrid rank (cosine max-blend + BM25 + lexical)
              → rerank (per-file dedup + term-overlap boosts) → top-N
```

The three pieces that matter most, and why:

- **Dual vectors.** Every chunk stores an embedding of the raw code *and* an embedding of a generated natural-language description. A query scores against whichever it's closer to (`max`, not an average), so literal and conceptual matches can each win independently.
- **Structural metadata.** Chunks carry a `// [Package: ... | File: ... | Symbol: ...]` prefix before embedding. This measurably improved results — the model sees *where* the code lives, not just what it says.
- **Hybrid scoring.** Semantic cosine similarity is blended with a weighted BM25 keyword score. Pure vector search drifts on exact identifiers; pure keyword search misses concepts. The blend keeps both sharp.

---

## Known Limitations

Being honest about where it's rough:

- **Very large monorepos index slowly.** Chunk extraction is single-threaded. Use `--no-llm` and index target subdirectories rather than the whole tree.
- **Large mixed doc+code indexes have lower conceptual recall.** At 10K+ chunks (e.g. hugo), documentation and testdata chunks crowd out code results. `--code-only` helps; the eval numbers in the table above are the honest picture.
- **Conceptual queries that share no words with the code are the hardest case.** The description vectors close most of the gap but not all of it.
- **Small local model by design.** CodeMiniLM (384-dim) keeps everything fast and CPU-friendly; it's deliberately not a frontier model.
- **The ONNX Runtime path is macOS/Homebrew-specific** and pinned to a versioned Cellar path. Porting to other platforms means relaxing that path.
- **Search is brute-force cosine (no ANN index).** Fine for hundreds of thousands of chunks, slow at millions.
- **LLM descriptions are optional and network-bound.** They need a local Ollama server; without one, heuristic descriptions are used and results differ slightly.

---

## Roadmap

Ideas that are deferred but natural next steps:

- **Editor integration** (a VS Code extension that just calls the local API).
- **Multi-repo / cross-project search.**
- **ANN indexing** (HNSW) for very large codebases.
- **Incremental chunking** — the watcher re-embeds changed files, but initial chunk extraction itself is single-threaded.

---

## Contributing

Contributions are welcome. The codebase is small and readable: the pipeline lives in `pkg/` (`scanner`, `chunker`, `embedder`, `indexer`, `reranker`, `llm`, `watcher`, `history`), with the CLI glue in `main.go`.

Before opening a PR, please:

1. Run `go test ./pkg/...` (note: `go test ./...` fails by design — `scratch/` holds many one-off `main` scripts, and the eval files intentionally duplicate types).
2. If your change affects search quality, run the eval harness against at least one of the eval repos and report the numbers. `docs/evaluation.md` explains how — reproducible results are the whole point of that harness.

---

## Acknowledgments

- **Embedding model:** CodeMiniLM — `isuruwijesiri/all-MiniLM-L6-v2-code-search-512` on Hugging Face.
- **ONNX Runtime** and the `yalue/onnxruntime_go` Go bindings — fast local inference with no Python runtime.
- **tree-sitter** and its language grammars — the language-aware chunking this tool's quality depends on.
- **`spf13/cobra`** (CLI), **`monochromegane/go-gitignore`** (ignore rules), **`fsnotify`** (watch mode).
- **Ollama** — local LLM description generation, and the `qwen3.5` model behind it.
- **The open-source projects used for evaluation:** gin, cobra, gorilla/mux, clap, and hugo — this tool is better for being tested against real code that nobody here wrote.