# openai/codex Indexing Failure Log

Date: 2026-08-11
Status: ABANDONED — pivoted to a medium-size repo (clap-rs/clap) for the multi-repo eval.

> Pipeline note (2026-08-13): this log describes the pipeline as of 2026-08-11.
> v5 (2026-08-13) made chunk extraction parallel (bounded worker pool), so the
> single-threaded bottleneck described in Attempt 1 below has changed — the
> failure mode (hundreds of 20K+-line generated schema files, boundary `rs`
> subprocess memory) and the mitigation advice still hold for large monorepos.

## Why codex was chosen
The eval needed one "large" repo to test indexer scaling. openai/codex is a
large, popular, real-world Rust monorepo (the AI coding agent).

## Repo facts
- Clone: `git clone --depth 1` into `scratch/external_eval/codex` (~88MB working tree)
- 2,994 `.rs` files
- 6,107 files total (excluding `.git`), per scanner

## Attempt 1: full-repo index (`./semsearch index scratch/external_eval/codex`)
- Scanner found 5,897 files.
- The chunk-extraction loop (`main.go runIndexing`, single-threaded) was the bottleneck:
  - After ~28 min it had processed only ~653 / 6,107 files (~10.6%).
  - Projected total extraction time: ~4.4 hours. Aborted.
- `sample` profiler showed time spent mostly in `ts_parser_parse_string`
  (tree-sitter) — codex has 671 `.ts` schema files in `app-server-protocol`
  and several 20K+-line generated JSON schemas that chunk slowly.
- Files already inspected: scanner uses `filepath.Walk` (does not follow
  symlinks — symlinked crate index does not work), AST chunker covers
  `.go/.py/.js/.ts`, everything else falls back to the sliding-window chunker.

## Attempt 2: slim crate index (`scratch/external_eval/codex_slim`, copies of 14 crates)
- Copied only: core, tools, exec, apply-patch, sandboxing, config, file-search,
  tui, message-history, memories, skills, mcp-server, ollama, model-provider.
- 2,507 files, 1,227 `.rs`, 697 `.snap` (insta snapshots), 30MB.
- Chunk extraction itself was FAST (~34,436 chunks in under a minute).
- The slow phase became LLM description generation:
  - Every chunk >= 30 lines (default `SEMSEARCH_MIN_DESC_LINES=30`) gets an
    Ollama call at concurrency 4 (`SEMSEARCH_LLM_CONCURRENCY`).
  - ~34K chunks means several thousand LLM calls at ~1s+ each => 30+ minutes,
    network-bound (process sat at 0% CPU, only 4 ESTABLISHED sockets to Ollama).
  - Aborted mid-way.

## Key insight
`scratch/eval_bench.go` evaluates with `HybridSearch(..., codeOnly=true, docsOnly=false)`,
so the LLM description vectors do NOT affect eval results. The eval only needs
the code vector index — `--no-llm` is sufficient and dramatically faster.
The gin/cobra/mux indices already have descs, but their eval numbers are code-only
as well, so comparisons remain apples-to-apples.

## Takeaways for the README / future work
1. Indexing a very large monorepo is single-threaded in chunk extraction and
   LLM-desc generation — big repos (thousands of files, heavy `.ts`/schema
   content) take hours. Consider:
   - parallel chunk extraction,
   - skipping AST chunking for generated/schema files,
   - batching + higher LLM concurrency for desc generation.
2. For large repos, use `--no-llm` for a code-only index unless the desc vectors
   are actually needed (eval and code-only search don't need them).
3. Docs to write later: document `SEMSEARCH_MIN_DESC_LINES`, `SEMSEARCH_LLM_CONCURRENCY`,
   and this scaling behavior.

## Result
Pivoted to a medium-size repo (clap-rs/clap) that indexes quickly and gives a
fair signal. See `scratch/multi_repo_results.json` and `scratch/run_code_only.txt`.
