# Reranker & Retrieval Tuning Log — v3/v4

This file records how semsearch's search accuracy is tuned and how to reproduce
every number. Update it whenever the reranker, retrieval weights, or eval
config change. It is the source of truth for "why is the accuracy what it is".

## Current Production Tuning (v4)

Hardcoded defaults in `main.go` and `pkg/reranker/reranker.go`:

| param          | value | where                                | notes                                             |
|----------------|-------|--------------------------------------|---------------------------------------------------|
| semantic weight| 0.65  | main.go `HybridSearch(...)`          | semantic/(lexical) blend in fused cosine+lexical |
| shortlist      | 30    | main.go `candidateLimit`             | top-N fed to reranker                             |
| `SYM`          | 0.10  | reranker `boostParams()`             | term-overlap boost on symbol name                 |
| `PATW`         | 0.0   | reranker `boostParams()`             | term-overlap boost on file path                   |
| `CONT`         | 0.01  | reranker `boostParams()`             | term-overlap boost on content                     |
| `MFD`          | 1     | reranker (env)                       | per-file dedup: 1 = best chunk/file before boosting, 2 = after boosting (v4), 0 = off |
| `STABW`        | 0.0   | reranker `boostParams()`             | stability bonus (experimental, off by default)    |
| `SEG`          | 0     | reranker `boostParams()`             | segment-matching mode (0/2/3, experimental)       |

## Results (rebuilt clean LLM indexes)

| config                          | top-5 | notes                              |
|---------------------------------|-------|------------------------------------|
| honest baseline (rebuilt idx)   | 136   | w=0.65 sl=30, old additive boosts  |
| + per-file dedup (MFD=1)        | 145   | removes same-file chunk crowding   |
| + rebalanced boosts (v3 final)  | 147   | SYM=0.10 PATW=0 CONT=0.01          |
| + v4 (corrected labels + MFD=2) | 156   | ground-truth fixes + final-order dedup |

Per-repo (v4): gin 41/42, cobra 27/27, mux 25/25, clap 31/40, hugo 32/39.
Run: `eval_bench.go` with env `W=0.65 SL=30`.

## Reproducing the eval

```bash
# all five repos must be LLM-indexed (desc != code):
for r in gin cobra mux clap hugo; do
  ./semsearch index scratch/external_eval/$r      # Ollama running, qwen3.5:0.8b
done
W=0.65 SL=30 go run scratch/eval_bench.go \
  scratch/evalcases_{gin,cobra,mux,clap,hugo}.go
```

IMPORTANT: LLM description vectors are generated non-deterministically
(qwen3.5, temperature 0 but upstream nondeterminism). Re-indexing a repo
regenerates desc vectors and can shift absolute numbers by ±1–3. Treat
"156/173" as the number on *these* particular index files; always re-confirm
after any reindex. Code vectors and eval cases are deterministic.

## Pitfalls learned (do not repeat)

1. **Never let `.semsearch`/`desc_cache.json`/`history.json` be indexed.**
   v2's clap index silently included a stray `desc_cache.json` (semsearch's
   own cache written into the repo root) as a code chunk. It inflated the
   "145/173" baseline and was only caught because the file carried
   descriptions that lexically matched queries. Fix: scanner now ignores
   `desc_cache.json`, `history.json` (pkg/scanner/scanner.go
   `defaultIgnoredFiles`). A rebuild was required → baseline dropped 145→136.
2. **The old 145/146/147 stability ("STABW") results applied to the
   contaminated clap index and are no longer reproducible. Do not cite them.**
3. **`scratch/eval_sweep.go` is NOT a trustworthy scorer** — it shared cached
   shortlists across configs under the hood (cache/truncation bug). Use only
   harnesses that call `HybridSearch` with the limit directly:
   `eval_cleangrid.go`, `eval_pd.go`, `eval_pds.go`, `eval_pds2.go`,
   `eval_dem.go`, `eval_bench.go`.
4. **`codeOnly` is a file-type filter, not "no desc vectors"**. Code chunks
   always score `max(codeSim, descSim)`. A `--no-llm` index produces
   *synthetic* desc vectors, so numbers differ from LLM indexes.
5. **Env pitfall**: do not name an eval env var `PATH` (clobbers CGO build).
   Use `PATW`. Also prefer the full path `/opt/homebrew/bin/go` (shell can
   lose `go` on PATH in long sessions).
6. **`go run` scratch conflicts**: scratch/ is one `package main` with many
   `func main()`. Running one harness requires passing exactly
   `scratch/<harness>_main.go` + the model/case files, not `./...`.
   `go build ./...` fails by design; build with `go build -o semsearch .`.

## Remaining failure modes (targets for v5)

- hugo interface-vs-impl ranking: correct file often ranked as a
  semantically-arguable alternative (e.g. `Page` interface vs `pageState`).
- clap concept queries that lexically overlap one of many similar builder
  files (e.g. "specifying how many values an argument takes" vs `range.rs`).
- hugo `config` category (3 cases, 1 hit) and docs/testdata chunk dilution.
- gin has one unfixable case: "CORS" — no CORS code exists in this checkout.

## Idea backlog

- SEG=3 (segment-matched symbol+path) tied the old 145 but on clean indexes
  is untested with MFD; could recover clap builder-file cases.
- Stability bonus (`STABW`) previously helped the contaminated index; retest
  on clean indexes with MFD on.
- Cross-encoder reranker stub exists (`pkg/reranker/cross_encoder.go`) but no
  trained model; a cross-encoder would attack the hugo interface-vs-impl
  failures directly.
- Retrieval-side fix for expected-evicted cases (shortlist 30 shows ~28 cases
  still outside top-5 entirely; raising shortlist/desc quality are the levers).