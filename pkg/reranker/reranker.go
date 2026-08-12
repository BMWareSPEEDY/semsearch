package reranker

import (
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/ashishpaliwal/semsearch/pkg/indexer"
)

type Reranker struct {
	enabled bool
}

func NewReranker() *Reranker {
	return &Reranker{enabled: true}
}

func boostParams() (symbol, path, content, stab float32, segMode int) {
	f := func(k string, def float32) float32 {
		if v := os.Getenv(k); v != "" {
			if fv, err := strconv.ParseFloat(v, 32); err == nil {
				return float32(fv)
			}
		}
		return def
	}
	segMode = 0
	if v := os.Getenv("SEG"); v != "" {
		if iv, err := strconv.Atoi(v); err == nil {
			segMode = iv
		}
	}
	return f("SYM", 0.10), f("PATW", 0.0), f("CONT", 0.01), f("STABW", 0.0), segMode
}

// Rerank performs a term-overlap second-pass reranking over the top candidates.
func (r *Reranker) Rerank(query string, candidates []indexer.SearchResult, topN int) []indexer.SearchResult {
	if len(candidates) <= 1 || query == "" {
		return candidates
	}

	terms := indexer.TokenizeTerms(query)
	if len(terms) == 0 {
		return candidates
	}

	n := len(candidates)
	symW, pathW, contW, stabW, segMode := boostParams()

	if os.Getenv("MFD") != "0" && len(candidates) > 1 {
		best := map[string]int{}
		for i := range candidates {
			fp := candidates[i].Chunk.FilePath
			if j, ok := best[fp]; !ok || candidates[i].Score > candidates[j].Score {
				best[fp] = i
			}
		}
		kept := make([]indexer.SearchResult, 0, len(best))
		for _, i := range best {
			kept = append(kept, candidates[i])
		}
		sort.Slice(kept, func(a, b int) bool { return kept[a].Score > kept[b].Score })
		candidates = kept
		n = len(candidates)
	}

	for i := range candidates {
		c := &candidates[i]
		contentLower := strings.ToLower(c.Chunk.Content)
		symbolLower := strings.ToLower(c.Chunk.SymbolName)
		fileLower := strings.ToLower(c.Chunk.FilePath)
		var symbolSegs, pathSegs []string
		if segMode > 0 {
			symbolSegs = segmentize(symbolLower)
			pathSegs = segmentize(fileLower)
		}

		var symbolBoost, contentBoost, pathBoost float32 = 0.0, 0.0, 0.0

		for _, term := range terms {
			if segMode == 3 {
				symbolBoost += float32(segMatch(symbolSegs, term)) * symW
				pathBoost += float32(segMatch(pathSegs, term)) * pathW
				if containsWord(contentLower, term) {
					contentBoost += contW
				}
			} else if symbolLower != "" && containsWord(symbolLower, term) {
				symbolBoost += symW
			} else if segMode == 2 && symbolLower != "" && segMatch(symbolSegs, term) > 0 {
				symbolBoost += symW * 0.8
			}
			if containsWord(fileLower, term) {
				pathBoost += pathW
			} else if segMode == 2 && segMatch(pathSegs, term) > 0 {
				pathBoost += pathW * 0.8
			}
			if containsWord(contentLower, term) {
				contentBoost += contW
			}
		}

		candidates[i].Score += symbolBoost + pathBoost + contentBoost
		if stabW > 0 {
			candidates[i].Score += stabW * float32(n-i)
		}
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

// segMatch returns 1.0 on exact segment match, 0.5 on a ≥4-char shared prefix.
func segMatch(segs []string, term string) float64 {
	for _, s := range segs {
		if s == term {
			return 1.0
		}
	}
	if len(term) < 4 {
		return 0
	}
	for _, s := range segs {
		if len(s) < 4 {
			continue
		}
		if strings.HasPrefix(s, term) || strings.HasPrefix(term, s) {
			return 0.5
		}
	}
	return 0
}

// segmentize lowercases text and splits identifiers on punctuation and camelCase
// boundaries, producing the set of unique meaningful segments.
func segmentize(s string) []string {
	if s == "" {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	parts := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == '/' || r == '\\' || r == '(' ||
			r == ')' || r == '[' || r == ']' || r == '{' || r == '}' || r == ' ' ||
			r == ':' || r == '*' || r == '"' || r == '\'' || r == '<' || r == '>' ||
			r == ',' || r == ';' || r == '=' || r == '+' || r == '&' || r == '|'
	})
	for _, p := range parts {
		for _, seg := range splitCamel(p) {
			if seg == "" || len(seg) < 2 {
				continue
			}
			if !seen[seg] {
				seen[seg] = true
				out = append(out, seg)
			}
		}
	}
	return out
}

func splitCamel(s string) []string {
	var words []string
	var cur strings.Builder
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			cur.WriteRune(r)
		} else if r >= 'A' && r <= 'Z' {
			if cur.Len() > 0 {
				words = append(words, cur.String())
				cur.Reset()
			}
			cur.WriteRune(r + ('a' - 'A'))
		}
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	return words
}