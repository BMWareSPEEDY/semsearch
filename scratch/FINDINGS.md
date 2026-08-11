# SemSearch Eval Findings — What Works / What Doesn't

Date: 2026-08-11
Model: CodeMiniLM (384-dim, ONNX), code-only vector search + hybrid scoring
Repos evaluated: gin, cobra, mux, clap, hugo (see scratch/logs/multi_repo_eval_run2.txt)

## Summary numbers (top-5 hit rate)

| repo            | chunks | queries | top-1 | top-3 | top-5 | notes                                  |
|-----------------|--------|---------|-------|-------|-------|----------------------------------------|
| gin-gonic/gin   |    856 |      42 | 23/42 | 30/42 | 34/42 | 81% — Go web framework                |
| spf13/cobra     |    447 |      27 | 19/27 | 22/27 | 22/27 | 81% — Go CLI framework                |
| gorilla/mux     |    160 |      25 |  7/25 | 16/25 | 21/25 | 84% — Go router                       |
| clap-rs/clap    |   6391 |      40 | 12/40 | 18/40 | 23/40 | 58% — Rust, large index               |
| gohugoio/hugo   |  10847 |      39 |  8/39 | 15/39 | 17/39 | 44% — very large index, mixed doc/code |

## What works
- Exact-symbol queries: very strong on small/medium Go repos (top-3 ~9/10+).
  e.g. "func (c *Command) Execute", "func (r *Router) ServeHTTP" hit TOP-1.
- Small repos (gin/cobra/mux): 81–84% top-5 overall.
- Code-only search (--code-only flag) reliably surfaces the right file for
  exact symbols even on large repos (hugo: "func (h *HugoSites) Build" -> TOP-1).
- Indexing pipeline itself: fast for repos < ~1k source files (clap 75s,
  hugo ~5min with --no-llm).
- LLM description generation (Ollama qwen3.5:0.8b): concurrent, hash-gated
  cache, deterministic; ~31–644 descs per repo in seconds-to-minutes.

## What does NOT work / degrades
1. **Large mixed doc+code indexes (hugo 44%)**: once the index gets large
   (10K+ chunks) and includes markdown docs + testdata, conceptual queries
   get drowned by near-identical high-similarity doc/test chunks. Code-only
   search helps but conceptual recall is still weak on big codebases.
2. **Minified bundles**: a 306KB minified JS bundle produced 1639 near-identical
   chunks that dominated EVERY search result. FIXED: scanner now skips binary
   content and minified files (see pkg/scanner/scanner.go, isBinaryFile +
   isMinifiedFile + tests).
3. **Binary files mislabeled with a source extension** (e.g. a PNG named
   "fake.js"): tree-sitter would pathologically backtrack (ts_parser__recover
   spinning for many minutes). FIXED: isBinaryFile skips files with NUL bytes.
4. **Very large monorepos (openai/codex, 6107 files)**: full indexing projected
   ~4.4h (single-threaded extraction + tree-sitter .ts schemas) and LLM desc
   generation on ~34K chunks was network-bound. See scratch/codex_indexing_log.md.
   Mitigation: --no-llm for code-only search; index only target crates.
5. **Conceptual semantic queries on Rust/large code** (clap 58%, hugo 44%):
   CodeMiniLM is trained for code matching; conceptual phrases that don't
   lexically overlap the code rank poorly. LLM description vectors (when used)
   improve this but the code-only eval below does not include them.

## Fixes made in this session
- pkg/scanner/scanner.go: added isBinaryFile (NUL-byte sniff, 4KB) and
  isMinifiedFile (line > 20KB). Both skip files before chunking.
- pkg/scanner/scanner_test.go: TestScanSkipsBinaryFileWithSourceExtension,
  TestScanSkipsMinifiedBundle.
- After the minified fix, hugo top-5 improved 7/39 -> 17/39.
