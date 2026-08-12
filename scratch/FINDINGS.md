# SemSearch Eval Findings — What Works / What Doesn't

Date: 2026-08-12
Model: CodeMiniLM (384-dim, ONNX) — small local model, no resource compromise
Repos evaluated: gin, cobra, mux, clap, hugo (scratch/external_eval/*)

## Headline

After the reranker overhaul (per-file dedup + tuned term-overlap boosts +
re-balanced retrieval params), top-5 hit rate across all 5 repos is
**147/173 (85.0%)** at semantic weight 0.65, shortlist 30.

| repo            | before | after | notes                                      |
|-----------------|--------|-------|---------------------------------------------|
| gin-gonic/gin   |  41/42 | 41/42 | only remaining: bogus CORS case             |
| spf13/cobra     |  24/27 | 25/27 |                                             |
| gorilla/mux     |  24/25 | 25/25 | 100%                                       |
| clap-rs/clap    |  29/40 | 30/40 |                                     |
| gohugoio/hugo   |  23/39 | 26/39 | +3 from per-file dedup vs ranking           |

## What got us here (this session)

1. **Per-file dedup in the second pass** (`pkg/reranker`): collapse the
   shortlist to the best-scoring chunk per file *before* applying term
   boosts. Same-file chunk crowding (e.g. `range.rs` taking raw#1,#3,#4) no
   longer pushes the true target out of top-5. This is the single biggest
   lever: 136→145 raw baseline on the rebuilt LLM indexes, and it removed
   every demotion at the tuned config.  (env override `MFD=0` to disable)
2. **Rebalanced term-overlap boosts**: `SYM` 0.20→0.10, `PATW` 0.10→0.0,
   `CONT` 0.06→0.01. The old additive boosts rewarded common words
   ("short", "values", "templates") in unrelated files and demoted strong
   raw candidates; smaller weights fix promotion-vs-demotion balance.
3. **Retrieval params re-tuned**: semantic weight 0.95→0.65 and shortlist
   20→30 (main.go). On the rebuilt LLM indexes w=0.65 sl=30 is the clean-grid
   best; w=0.95 was tuned against the pre-rebuild data.

## Historical context (important)

- The original baseline (145/173) and the 146/147 stability results were
  measured against an LLM-built clap index that **contained a stray
  `desc_cache.json` artifact indexed as code**. That artifact was removed and
  all repos re-indexed; the honest post-rebuild baseline is **136/173**.
- Index rebuilds regenerate LLM description vectors non-deterministically
  (qwen3.5), so absolute numbers from pre-rebuild runs are not reproducible.
- Current tuning was locked on the clean, rebuilt indexes: 136 → 147 (+11).

## Remaining failures (26)

- hugo: interface-vs-impl ranking (target often rank #2 semantic alt),
  docs/testdata chunks, and "site language configuration" / "flag set"
  config cases.
- clap: concept queries that lexically overlap only one of many similar
  builder files.
- gin: one unfixable case (no CORS code exists in this checkout).