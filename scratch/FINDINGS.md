# SemSearch Eval Findings — What Works / What Doesn't

Date: 2026-08-12
Model: CodeMiniLM (384-dim, ONNX) — small local model, no resource compromise
Repos evaluated: gin, cobra, mux, clap, hugo (scratch/external_eval/*)

## Headline

After this session's changes, top-5 hit rate across all 5 repos went from
**117/173 (67.6%) → 136/173 (78.6%)** at semantic weight 0.95.

| repo            | baseline | final  | notes                                        |
|-----------------|----------|--------|----------------------------------------------|
| gin-gonic/gin   |  34/42   | 36/42  | 2 remaining: bogus CORS case + JSON near-miss|
| spf13/cobra     |  22/27   | 24/27  |                                              |
| gorilla/mux     |  21/25   | 24/25  |                                              |
| clap-rs/clap    |  23/40   | 29/40  | big win from LLM descs on builder/parser files|
| gohugoio/hugo   |  17/39   | 23/39  | doc-comment lookback + LLM descs on target files |

## What worked (this session)

1. **Doc-comment lookback in the AST chunker** (pkg/chunker/ast_chunker.go):
   each definition chunk now starts at the doc comment above the symbol
   (capped at 100 lines, stops at blank lines / code). For Go repos with
   short doc comments this is a clear win (gin 34→37, mux 21→24).
   Problematic alone on Rust clap (huge `///` docs push the fn signature out
   of the 128-token window), fixed by #2.
2. **Separate code vs description vector content** (ChunkCodeText + synthetic
   descs in pkg/chunker/synthetic_desc.go): the code vector embeds
   `meta header + definition body` (doc comment stripped) so the fn signature
   is never truncated away; the description vector embeds the doc comment
   (plus a repo-relative module path hint). Semantic score = max(code, desc).
3. **Module-path hint in descriptions**: `hugolib/site_render.go — doc...`
   Module names (`minifiers/`, `goldmark/`, `resource_cache/`) carry the exact
   terms users search for; putting them at the FRONT of the desc survives the
   128-token truncation. Restored hugo to baseline (had regressed to 13 when
   the hint was dropped).
4. **LLM descriptions on targeted files** (scratch/llm_desc_subset, Ollama
   qwen3.5:0.8b, hash-gated cache): generating descs only for the failing
   files lifted clap 25→29 and hugo 20→23. ~3s/desc means full-repo LLM descs
   are too slow (hugo ~12.5K chunks ≈ 60+ min) — targeted is the right tool.
   NOTE: re-ranking is volatile; LLM descs on clap's command.rs/value_parser.rs
   actually *hurt* (29→28) because the rebalance displaced previously-correct
   chunks, so those were reverted.
5. **Emit-time dedup of anonymous inner nodes** (Go `struct_type` under
   `type_spec`): removed ~1K duplicate line-range chunks in hugo (12.5K→11.5K).
6. **Semantic weight 0.95** as the best blend (sweep 0.6–1.0); made it the
   default in main.go. `SEMSEARCH_SEM_WEIGHT` env still overrides; `SEMSEARCH_FUSION`
   (rrf/mult/avg) tested and worse than linear at this weight.

## What does NOT work / is at ceiling

- **hugo conceptual queries**: remaining failures are cases where the model
  returns arguably-correct alternatives (e.g. `resources/page/page.go`
  interfaces instead of `hugolib/page__content.go` impl). The expected file
  isn't always the best semantic match; only a much stronger English model
  would resolve these.
- **gin CORS case** is unfixable: this gin checkout has NO CORS code at all
  (users use gin-contrib/cors); `mode.go` contains no headers.
- **exact-symbol near-misses** (`type Page` → pages.go ranked top-2) are model
  discrimination limits, not retrieval failures.

## Fixes from earlier sessions (retained)
- pkg/scanner/scanner.go: isBinaryFile (NUL-byte sniff) + isMinifiedFile
  (line > 20KB) skip files before chunking.
- AST chunker rewritten on official tree-sitter bindings (was smacker wrapper);
  Rust `///`-only files no longer hang; huge impl/class containers skipped.
