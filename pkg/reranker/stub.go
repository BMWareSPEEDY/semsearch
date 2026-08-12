package reranker

// NewStubReranker returns a no-op reranker (used for A/B comparisons in evals).
func NewStubReranker() *Reranker {
	return &Reranker{enabled: false}
}
