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

// Rerank performs a term-overlap second-pass reranking over the top candidates.
// It boosts chunks whose symbol name, file path or content token-wise match the
// query's meaningful tokens (matching the indexer's tokenization so the boosts
// are consistent).
func (r *Reranker) Rerank(query string, candidates []indexer.SearchResult, topN int) []indexer.SearchResult {
	if len(candidates) <= 1 || query == "" {
		return candidates
	}

	terms := indexer.TokenizeTerms(query)

	for i := range candidates {
		contentLower := strings.ToLower(candidates[i].Chunk.Content)
		symbolLower := strings.ToLower(candidates[i].Chunk.SymbolName)
		fileLower := strings.ToLower(candidates[i].Chunk.FilePath)

		var symbolBoost, contentBoost, pathBoost float32 = 0.0, 0.0, 0.0

		for _, term := range terms {
			// Boost if the symbol name contains the query token.
			if symbolLower != "" && containsWord(symbolLower, term) {
				symbolBoost += 0.20
			}
			// Boost if the file path contains the query token.
			if containsWord(fileLower, term) {
				pathBoost += 0.10
			}
			// Boost if the content contains the query token as a word.
			if containsWord(contentLower, term) {
				contentBoost += 0.06
			}
		}

		candidates[i].Score += symbolBoost + pathBoost + contentBoost
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	if len(candidates) > topN {
		candidates = candidates[:topN]
	}

	return candidates
}

// containsWord reports whether the token appears as a standalone word in s.
// s is expected to be already lowercased.
func containsWord(s, token string) bool {
	start := 0
	for {
		idx := strings.Index(s[start:], token)
		if idx == -1 {
			return false
		}
		pos := start + idx
		before := pos == 0 || !isWordChar(s[pos-1])
		after := pos+len(token) >= len(s) || !isWordChar(s[pos+len(token)])
		if before && after {
			return true
		}
		start = pos + len(token)
	}
}

func isWordChar(r byte) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_'
}
