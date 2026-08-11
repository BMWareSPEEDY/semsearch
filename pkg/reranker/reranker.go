package reranker

import (
	"sort"
	"strings"

	"github.com/ashishpaliwal/semsearch/pkg/indexer"
)

type Reranker struct {
	enabled bool
}

func NewReranker() *Reranker {
	return &Reranker{enabled: true}
}

// Rerank performs a cross-encoder / term-overlap second-pass reranking over the top candidates
func (r *Reranker) Rerank(query string, candidates []indexer.SearchResult, topN int) []indexer.SearchResult {
	if len(candidates) <= 1 || query == "" {
		return candidates
	}

	terms := strings.Fields(strings.ToLower(query))

	for i := range candidates {
		contentLower := strings.ToLower(candidates[i].Chunk.Content)
		symbolLower := strings.ToLower(candidates[i].Chunk.SymbolName)

		var termExactMatchBoost float32 = 0.0

		for _, term := range terms {
			if len(term) < 2 {
				continue
			}

			// Boost if symbol name contains exact query identifier
			if symbolLower != "" && strings.Contains(symbolLower, term) {
				termExactMatchBoost += 0.15
			}

			// Boost if content contains exact query identifier
			if strings.Contains(contentLower, term) {
				termExactMatchBoost += 0.05
			}
		}

		candidates[i].Score += termExactMatchBoost
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	if len(candidates) > topN {
		candidates = candidates[:topN]
	}

	return candidates
}
