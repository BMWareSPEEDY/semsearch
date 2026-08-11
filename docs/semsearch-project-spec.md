# `semsearch` — Local Semantic Code Search (CLI + Local API)

A tool that indexes your codebase into vector embeddings and lets you search it in plain English — "where do we retry failed API calls" — and get back the actual relevant code, ranked by meaning instead of exact text match. Fully local: no cloud, no account, no code ever leaves your machine. Usable from the terminal or as a local HTTP API.

---

## 1. Problem Statement

`grep`/`ripgrep`/IDE "find in files" only match exact text. If you don't remember the exact identifier, string, or wording used in the code, you get zero results — even though the code you want exists and does exactly what you're describing.

Concrete example: searching for `"retry failed api call"` finds nothing if the actual function is named `resilientFetch`, `withBackoff`, or `attemptRequest`. You *know* what the code does; you don't know what it's literally called. This happens constantly on any codebase you didn't write every line of yourself, or one you wrote six months ago and forgot the naming conventions in.

### Why existing solutions don't close this for the target user
| Existing approach | What it does | Why it's not this |
|---|---|---|
| `grep` / `ripgrep` / IDE text search | Exact/regex text matching | No understanding of meaning — must know the literal string |
| GitHub code search | Text/regex search across repos | Same limitation, plus requires code to be on GitHub |
| Sourcegraph Cody | Semantic + AI-assisted code search | Cloud-tied, often paid, IDE-embedded — not a local, standalone CLI/API |
| Cursor codebase indexing | Semantic search over your project | Bundled inside a paid IDE, not usable standalone from a terminal or by other tools |
| Language Server "go to definition" | Structural navigation (symbols, references) | Requires knowing what to jump *to* already — doesn't help when you don't know the name |

**The gap:** there is no dominant **local-only, CLI-first, editor-agnostic** semantic code search tool that just runs on your machine, indexes once, and answers plain-English queries over a local HTTP API that anything (a script, an editor plugin, a shell alias) can hit.

---

## 2. Core Concept

1. **Index** — walk the codebase, chunk it into meaningful units (functions, classes, or fixed-size overlapping blocks as a fallback), compute a vector embedding for each chunk using a small local embedding model, store vectors + metadata (file, line range, chunk text) in a local index file.
2. **Query** — embed the user's natural-language query with the same model, compute cosine similarity against every indexed chunk, return the top-N ranked results with file, line number, and a snippet.
3. **Serve** — the same query logic exposed over a local HTTP API (`localhost:PORT`), so it's not CLI-only — anything on the machine can query it (editor plugins, shell scripts, other tools).

No cloud calls anywhere in the core path. The embedding model runs locally (CPU is fine at the scale of a single repo).

---

## 3. Target User

- Developers working in large or unfamiliar codebases (their own old projects, a big monorepo, an inherited codebase) who regularly hit "I know this exists somewhere, I just don't know what it's called."
- Developers who want semantic search but don't want their code sent to a cloud indexing service (privacy/IP-sensitive codebases) — this is a real, stated reason people avoid Cursor/Copilot-style cloud indexing.
- Anyone who wants to build editor integrations or scripts on top of semantic search without depending on a paid API.

---

## 4. Architecture

### 4.1 Components

```
┌─────────────┐     ┌──────────────────┐     ┌──────────────────┐
│   semsearch  │────▶│  Embedding Model  │────▶│  Local Vector     │
│   CLI        │     │  (local, ONNX)    │     │  Index (on disk)  │
└─────────────┘     └──────────────────┘     └──────────────────┘
       │                                              ▲
       ▼                                              │
┌─────────────┐                                       │
│ Local HTTP   │───────────────────────────────────────┘
│ Server (API) │
└─────────────┘
```

- **CLI** (`semsearch`) — indexing, querying, server management. This is the primary interface for a human at a terminal.
- **Embedding model** — a small, local sentence/code-embedding model. Recommend starting with a compact ONNX-exported model (e.g., a distilled sentence-transformer or a small code-specific embedding model) so inference runs fast on CPU with no GPU dependency and no Python-heavy runtime requirement if the CLI itself is built in Go/Rust (use an ONNX runtime binding).
- **Vector index** — for v1, a flat file is genuinely sufficient (a repo is thousands, not billions, of chunks). Store as a simple binary/columnar format (vectors as float32 arrays + a parallel metadata array). No need for a dedicated vector database at this scale — brute-force cosine similarity over a few thousand–tens of thousands of vectors is sub-100ms. Revisit only if real usage shows repos large enough to need approximate nearest neighbor (ANN) indexing (e.g., HNSW).
- **Local HTTP server** (`semsearch serve`) — binds to `localhost` only by default (never `0.0.0.0`, to avoid accidentally exposing code search on a network). Exposes the same query logic as the CLI, so external tools (editor plugins, scripts) can use it without shelling out.

### 4.2 Language choice

Go or Rust for the CLI/server, same reasoning as before — single static binary, fast startup, no runtime dependency headaches for end users. The embedding model inference is the one place you need a real ML runtime; using ONNX Runtime (which has both Go and Rust bindings) keeps this out of needing a Python environment installed on the user's machine, which matters a lot for "just install one binary and go."

---

## 5. Indexing Design

### 5.1 Chunking strategy
- **Preferred:** language-aware chunking — parse the file (tree-sitter is a strong fit here, has grammars for essentially every mainstream language and lightweight bindings) and chunk at function/method/class boundaries. This produces semantically coherent chunks (a whole function, not half of one) and lets you attach real metadata (function name, containing class, file, line range) to every chunk.
- **Fallback:** for languages without a tree-sitter grammar wired up yet, or for non-code files (README, config, docs), fall back to fixed-size overlapping text chunks (e.g., ~200 tokens with ~50 token overlap) so nothing is silently excluded from the index.

### 5.2 What gets indexed
- Respect `.gitignore` by default (don't index `node_modules`, build artifacts, etc.).
- Support a `.semsearchignore` for additional exclusions.
- Index actual source files by default; make indexing of comments/docstrings and README/markdown content configurable (docs often contain exactly the plain-English descriptions that make semantic search valuable — e.g., a comment saying "retries the request with exponential backoff" is a great match for a query like "where do we retry failed requests").

### 5.3 Incremental re-indexing
v1 can be full re-index on demand (`semsearch index .`) — simple, predictable, and fast enough at single-repo scale that this isn't a real problem initially. A `semsearch watch` mode (re-embed only changed files on save, using file hashes to detect changes) is a clear, valuable v1.1 addition once the core index/query loop is solid — don't build it before the basic loop is proven.

---

## 6. CLI Design

```
semsearch index .                      # build/rebuild the index for the current repo
semsearch index . --exclude vendor/    # additional exclusions beyond .gitignore
semsearch "where do we retry failed api calls"       # query, prints ranked results to terminal
semsearch "token refresh logic" --top 10 --json       # more results, machine-readable output
semsearch serve --port 1921            # start the local HTTP API
semsearch serve --port 1921 --daemon   # run in background
semsearch status                       # index stats: chunk count, last indexed time, model in use
```

### Example terminal output
```
$ semsearch "where do we retry failed api calls"

1. src/http/client.ts:84-102        (score: 0.91)
   async function resilientFetch(url, opts) {
     // retries with exponential backoff on 5xx or network errors
     ...

2. src/queue/worker.ts:40-58        (score: 0.77)
   function processJob(job) {
     // re-enqueues failed jobs up to MAX_RETRIES times
     ...

3. src/http/client.test.ts:12-30    (score: 0.65)
   describe("resilientFetch retry behavior", () => {
     ...
```

---

## 7. Local API Design

```
GET  /health                          → { status: "ok", indexed_chunks: 4213 }
GET  /search?q=<query>&top=<n>        → ranked results (JSON)
POST /index                            → trigger a re-index (async, returns job id)
GET  /index/status                     → indexing progress/status
```

### Example
```
curl "localhost:1921/search?q=retry+failed+api+calls&top=5"
```
```json
{
  "query": "retry failed api calls",
  "results": [
    {
      "file": "src/http/client.ts",
      "line_start": 84,
      "line_end": 102,
      "score": 0.91,
      "snippet": "async function resilientFetch(url, opts) { ... }"
    }
  ]
}
```

Binds to `127.0.0.1` only by default; a `--bind` flag can override for advanced/trusted-network use cases, but the default must stay local-only given this is indexing someone's private source code.

---

## 8. Embedding Model Choice — Open Decision

This is the single most consequential technical decision and needs deliberate evaluation before committing, not a guess:

- **Option A: General-purpose sentence embedding model** (e.g., a small distilled sentence-transformer) — well-understood, widely available in ONNX form, good at natural-language-to-natural-language matching (great for matching a query against comments/docstrings), weaker at matching a query against raw code syntax.
- **Option B: Code-specific embedding model** (models trained specifically on code, e.g., in the CodeBERT/UniXcoder lineage, or newer code-embedding models) — better at matching natural language to actual code structure/semantics, not just comments.
- **Recommendation:** evaluate both against a small hand-built test set (10-20 realistic "I'm looking for X" queries against a real repo you know well, with the "correct" answer known in advance) before committing — this is a cheap, half-day validation step that tells you which model actually performs before you build the rest of the tool around it. Don't skip this the way earlier ideas skipped validation.

---

## 9. Scope Boundaries (What NOT to Build)

- **Not a code generator or chatbot.** It returns existing code locations, ranked by relevance. It never writes, explains, or modifies code. This keeps the value proposition unambiguous and easy to verify (did it find the right code, yes/no) rather than opinion-based.
- **Not IDE-embedded for v1.** Ship as CLI + local API first. An editor plugin (VS Code extension that just calls the local API) is a natural, low-effort v2 once the core tool is solid — don't build the plugin before the search quality is proven.
- **Not a hosted/cloud service.** The entire value proposition is "your code never leaves your machine." Adding a cloud sync/hosted option later would need to be explicitly opt-in and clearly separate, never default.
- **Not a vector database product.** Don't over-engineer the storage layer (no need for HNSW/ANN indexing, no need for a dependency on a heavyweight vector DB) until real usage at real repo scale proves brute-force cosine similarity is actually too slow. It almost certainly won't be, at single-repo scale.

---

## 10. MVP Build Plan

**Weekend-scope v1:**
1. Fixed-size chunking (skip tree-sitter parsing initially — it's the right long-term approach but adds real complexity; a good fallback-only v1 chunker is a legitimate way to ship something testable fast).
2. Embedding model integration — get one model (start with Option A, general-purpose) running locally via ONNX Runtime, producing vectors for a list of text chunks.
3. Flat-file vector index — write/read chunks + vectors to disk.
4. `semsearch index .` and `semsearch "<query>"` — the core loop, terminal-only, no server yet.
5. Manual validation pass — the 10-20 query test set mentioned in Section 8, run against a real repo, to check the whole pipeline actually finds the right code before investing further.

**Post-MVP (v1.1+):**
6. `semsearch serve` — local HTTP API wrapping the same query logic.
7. Tree-sitter-based chunking for at least 1-2 languages you use most, replacing/supplementing the fixed-size fallback.
8. `--json` output mode, `semsearch status`.
9. Evaluate/swap to a code-specific embedding model if the validation pass shows the general-purpose model underperforming on real code (not just comments/docstrings).

**Explicitly deferred / v2 ideas, don't build yet:**
- `semsearch watch` incremental re-indexing.
- Multi-repo / cross-project search.
- Editor plugin (VS Code, JetBrains).
- ANN indexing for very large repos.
- Any hosted/cloud sync option.

---

## 11. Validation Step Before Full Build (Do This First)

Given the lesson from the earlier heuristic-based project idea (built confidence in an untested assumption before checking if it actually worked): before writing the full indexing/chunking/serving pipeline, do a small, cheap proof-of-concept —

1. Pick a real repo you know well.
2. Hand-write 10-20 natural-language queries for code you know exists in it, and note the "correct" file/function for each.
3. Manually embed those chunks and queries with 1-2 candidate models (a simple script, not the full tool) and check: does cosine similarity actually surface the right answer in the top 3-5 results?

If this quick test doesn't show clearly better-than-grep results, that's critical information *before* investing in chunking strategy, the server, and the CLI polish — and it's a half-day check, not a weekend.

---

## 12. Why This Is a Good Project

- **Unambiguous, verifiable value** — unlike the risk-scoring or behavioral-prediction ideas explored earlier, "did it find the code I meant" is a binary, immediately checkable outcome, not a trust-dependent judgment call. This directly avoids the trust-erosion failure mode raised earlier.
- **Real, non-manufactured gap** — genuine semantic search exists (Cody, Cursor) but is cloud-tied and IDE-embedded; nothing dominant is local-only, CLI/API-first, and editor-agnostic.
- **Real model, real engineering** — actual embedding inference and vector similarity search, not a heuristic dressed up as intelligence, and not a thin LLM API wrapper either.
- **Matches your stated architecture preferences** — local-first, no mandatory cloud dependency, minimal single-purpose CLI, real API surface for scripting/integration.
- **Buildable incrementally with a cheap validation gate up front** — the Section 11 check de-risks the whole project before the expensive parts are built.
