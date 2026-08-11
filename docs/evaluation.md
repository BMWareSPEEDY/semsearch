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
2. Runs `HybridSearch(..., codeOnly=true)` (top 20 raw, semantic weight 0.6).
3. Reranks to top-5.
4. Marks hit if `Expected` is a suffix/substring of the result file path.
5. Aggregates per-category and overall top-1/3/5, writes
   `scratch/multi_repo_results.json` and prints to stdout.

**Important:** the eval is code-only. LLM description vectors are not used for
scoring, so `--no-llm` indices produce identical eval numbers while indexing
much faster.

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

See `scratch/FINDINGS.md` for the full "works / doesn't work" analysis.
