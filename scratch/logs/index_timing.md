# Index Timing Log

Machine: macOS (Apple Silicon), ONNX Runtime 1.28.0
Model: CodeMiniLM 384-dim (`scratch/code_minilm/model_quantized.onnx`) for eval repos
Command: `./semsearch index <repo> [--no-llm]`

| repo        | files | chunks | mode    | wall time | notes                                  |
|-------------|-------|--------|---------|-----------|----------------------------------------|
| gin         |   40  |    447 | llm     | seconds   | LLM descs for 31 chunks                |
| mux         |   10  |    160 | llm     | seconds   | LLM descs for 24 chunks                |
| clap        |  600  |   6391 | no-llm  | ~1m15s    | Rust repo                              |
| hugo        | 1868  |  10847 | no-llm  | ~4m51s    | Go repo + markdown docs; large         |

Full-pipeline timing (with Ollama LLM descriptions) is network-bound and was
only measured up to the point of abort for large repos — see
`scratch/codex_indexing_log.md`.

## Notes

- `--no-llm` produces code-only descriptions (description vector == code vector)
  and is the recommended mode for large repos. Search/eval results are
  identical because the eval uses `codeOnly=true`.
- Chunk extraction is now parallel (v5; bounded worker pool, deterministic
  scan-order results). Embedding is CPU-bound and parallel (~229% CPU
  utilization observed).
- The full LLM description pipeline on a ~10K-chunk repo would take much longer
  (hundreds of Ollama calls at ~1s each with concurrency 4).
