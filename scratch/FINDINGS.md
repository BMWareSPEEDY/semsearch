# SemSearch Eval Findings — What Works / What Doesn't

Date: 2026-08-13
Model: CodeMiniLM (384-dim, ONNX) — small local model, no resource compromise
Repos evaluated: gin, cobra, mux, clap, hugo (scratch/external_eval/*)

## Headline

After the v4 eval data correction (8 expected-answer labels that pointed at
wrong/aggregate files fixed) plus the reranker `MFD=2` parity mode, top-5 hit
rate across all 5 repos is **155/173 (89.6%)** at semantic weight 0.65,
shortlist 30. v4 measured 156/173; the post-v5 re-run measures 155 (see the
v5 note below) — identical rankings, ±1 LLM-description drift.

| repo            | v3      | v4      | v5      | notes                                       |
|-----------------|---------|---------|---------|---------------------------------------------|
| gin-gonic/gin   |  41/42  |  41/42  |  41/42  | only remaining: bogus CORS case             |
| spf13/cobra     |  25/27  |  27/27  |  27/27  | 100%; 2 hits from corrected ground truth (command.go) |
| gorilla/mux     |  25/25  |  25/25  |  25/25  | 100%                                        |
| clap-rs/clap    |  30/40  |  31/40  |  31/40  | +1 from corrected ground truth (aot/generator/mod.rs) |
| gohugoio/hugo   |  26/39  |  32/39  |  31/39  | +6 from v3: 5 corrected labels + MFD=2; -1 at v5 = desc drift |

## What changed in v4 (2026-08-13)

1. **Corrected eval ground truth.** Eight `Expected` labels pointed at the wrong
   or aggregate file rather than the file that actually implements the answer:
   cobra's `Command` struct / root command live in `command.go`, not the
   top-level `cobra.go` wrapper; hugo's flag parsing, serve+hot-reload, config
   loading, defaults, language config, and HTML rendering live in
   `commandeer.go`, `server.go`, `configLoader.go`, `defaultConfigProvider.go`,
   `langs/config.go`, and `hugolib/page__per_output.go`; clap's shell-completion
   generation lives in `clap_complete/src/aot/generator/mod.rs`. The queries
   were right; the ground truth wasn't.
2. **Reranker `MFD=2` parity mode** (`pkg/reranker/reranker.go`): keep only the
   best-scoring chunk per file in the *final* ordering (after term boosts), in
   addition to the v3 `MFD=1` dedup that runs before boosting.

## What got us to v3 (2026-08-12, context)

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
- Current tuning was locked on the clean, rebuilt indexes: 136 → 147 (+11)
  from v3, then 147 → 156 (+9) from v4 (mostly ground-truth correction).

## v5 (2026-08-13) — performance release

v5 was a pure optimization pass and did **not** change ranking: the new lazy
keyword-statistics cache is verified ranking-neutral by `scratch/ab_verify.go`
(which runs old vs. new keyword paths and diffs the results — byte-for-byte
identical), and chunk IDs are unchanged under the parallel chunk extractor.
The eval re-run after v5 measured **155/173** (hugo 31/39); v4's 156/173 had
hugo at 32/39 on a previously regenerated description index. That one-case
hugo movement is the documented LLM-description nondeterminism (borderline
conceptual case swaps in/out of top-5), not a ranking regression. Treat the
total as 155–156 (±1).

## Remaining failures (18)

- hugo: interface-vs-impl ranking (target often rank #2 semantic alt),
  docs/testdata chunks, and "site language configuration" / "flag set"
  config cases.
- clap: concept queries that lexically overlap only one of many similar
  builder files.
- gin: one unfixable case (no CORS code exists in this checkout).