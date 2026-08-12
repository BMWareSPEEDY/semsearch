# semsearch — Evaluation Methodology

This document explains how semsearch is evaluated and how to reproduce the
numbers in `scratch/multi_repo_summary.txt`.

## Goal

Measure end-to-end semantic search quality on real, popular open-source repos:
can the tool find the file that actually implements a given query? We measure
"hit rate" at top-1 / top-3 / top-5.

## Repos

| repo        | language | source files | index size | why                                        |
|-------------|----------|-------------|------------|--------------------------------------------|
| gin-gonic/gin | Go      | ~40         | 856 chunks | small, well-known web framework            |
| spf13/cobra | Go       | ~15         | 447 chunks | small CLI framework                        |
| gorilla/mux | Go       | ~10         | 160 chunks | tiny router                                |
| clap-rs/clap| Rust     | 330         | 6391 chunks| medium Rust CLI parser                     |
| gohugoio/hugo | Go     | 906         | 10847 chunks| large Go codebase incl. docs              |

Repos are shallow-cloned into `scratch/external_eval/` (gitignored).

## Case sets

Each repo has a hand-written case file in `scratch/evalcases_<repo>.go`
(e.g. `evalcases_gin.go`). A case is:

```go
{Query: "rendering markdown content to html", Category: "conceptual", Expected: "markup/goldmark/convert.go"}
```

- `Query` — a natural language query (conceptual) or exact symbol name.
- `Expected` — the file that implements the answer (substring-matched against
  the result file path).
- Categories: `conceptual`, `exact_symbol`, `error_handling`,
  `request_pipeline`, `rendering`, `config`.

Queries were written against each repo's real source after inspection, so the
expected answers exist and are unambiguous.

## Driver

`scratch/eval_bench.go` (run with all case files):

```bash
go run scratch/eval_bench.go \
  scratch/evalcases_gin.go \
  scratch/evalcases_cobra.go \
  scratch/evalcases_mux.go \
  scratch/evalcases_clap.go \
  scratch/evalcases_hugo.go
```

For each case it:
1. Embeds the query with CodeMiniLM (384-dim) at `scratch/code_minilm/`.
2. Runs `HybridSearch(..., codeOnly=true)` (top 30 raw, semantic weight 0.65).
3. Reranks to top-5 (per-file dedup + tuned term-overlap boosts).
4. Marks hit if `Expected` is a suffix/substring of the result file path.
5. Aggregates per-category and overall top-1/3/5, writes
   `scratch/multi_repo_results.json` and prints to stdout.

**Important:** the eval runs with `codeOnly=true`, which **filters the result
set by file type** — Markdown/doc chunks are excluded. It does **not** disable
description-vector similarity: code chunks are still scored with the
max-blend `max(codeSim, descSim)`, so LLM description vectors are used for
code chunks whenever the index has them.

Consequently `--no-llm` indices do **not** produce identical eval numbers —
with `--no-llm` every chunk receives a heuristic *synthetic* description whose
vectors differ from both the code vector and an LLM description. `--no-llm` is
simply a faster, dependency-free indexing path; the source of the description
vectors is a separate axis from the `codeOnly` file-type filter. To reproduce
eval numbers exactly, use the same indexing mode that produced the index you
are measuring.

## Timing

Each repo must be indexed first (the driver only searches):

```bash
./semsearch index scratch/external_eval/gin
./semsearch index scratch/external_eval/cobra --no-llm
...
```

Recorded timings on a Mac (Apple Silicon): clap 1m15s, hugo 4m51s (both
`--no-llm`). Full-pipeline (with Ollama LLM descriptions) is slower and
network-bound.

## Reading the results

- Small Go repos: 81–84% top-5.
- Large mixed repo (hugo): 44% — degraded by docs + testdata chunks. Code-only
  search (`--code-only`) improves exact-symbol hits.
- Exact-symbol queries are the strongest category; conceptual phrases that
  don't lexically overlap code are the weakest.
- Overall top-5 across the 5 repos (after index rebuilds) is ~85% (147/173)
  with the tuned reranker (`SYM=0.10`, `PATW=0.0`, `CONT=0.01`, per-file
  dedup, shortlist 30, semantic weight 0.65).

See `scratch/FINDINGS.md` for the full "works / doesn't work" analysis.
