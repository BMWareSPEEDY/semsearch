# Evaluation & Benchmarks

How I tested search accuracy across different real-world open source projects.

## Setup

Instead of testing only on my own code, I tested search quality across 5 real repositories with 173 total test queries:

| Repo | Chunks | What it is |
|---|---|---|
| `gin-gonic/gin` | ~890 | Go web framework |
| `spf13/cobra` | ~520 | Go CLI library |
| `gorilla/mux` | ~165 | Go router |
| `clap-rs/clap` | ~7,250 | Rust argument parser |
| `gohugoio/hugo` | ~11,500 | Large Go static site generator |

Each test case consists of a natural English query or exact symbol name, paired with the file that actually implements that feature. A test counts as a hit if the expected file appears in the top-5 results.

## Results

Using the CodeMiniLM 384-dim model and hybrid reranking:

- **gin:** 41/42 (98%)
- **cobra:** 27/27 (100%)
- **mux:** 25/25 (100%)
- **clap:** 31/40 (78%)
- **hugo:** 31/39 (79%)
- **Total:** ~155/173 (~90% top-5 hit rate)

## Notes & What I Learned

- **Exact symbols vs conceptual queries:** Exact function or struct name lookups hit almost 100% of the time thanks to hybrid BM25 blending. Conceptual queries ("how do we render markdown to html") rely more on the dual-vector embeddings and LLM descriptions.
- **Per-file deduplication was crucial:** Without dedup, a single popular file with 5 matching chunks would take up all top-5 slots, kicking out the real answer. Collapsing the shortlist to the best chunk per file fixed this immediately.
- **Large repos with docs:** In large codebases with lots of Markdown and test fixtures (like Hugo), documentation chunks sometimes crowd out the actual code. Using `--code-only` helps keep search focused on real logic.
