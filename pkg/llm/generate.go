package llm

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/ashishpaliwal/semsearch/pkg/chunker"
)

// GenerateDescriptions fills descriptions for chunks in order, one LLM call per
// chunk (temperature 0) with hash-gated cache reuse. Chunks for which skip is
// true get an empty description and no LLM call — the caller decides how to
// handle those (e.g. reuse the code vector so max-blend degenerates to code-only).
//
// Generation runs on a small worker pool (concurrency > 1 is safe: the cache is
// mutex-guarded and each worker writes to its own slice slot). Index-time only;
// the query path never touches the LLM.
func GenerateDescriptions(ctx context.Context, client LLMClient, cache *DescriptionCache, chunks []chunker.Chunk, skip func(i int) bool, concurrency int) ([]string, int) {
	if concurrency < 1 {
		concurrency = 1
	}
	_ = cache.Load()

	descs := make([]string, len(chunks))
	var newDescs int64
	available := client.IsAvailable()

	var wg sync.WaitGroup
	jobs := make(chan int)

	worker := func() {
		defer wg.Done()
		for i := range jobs {
			if skip(i) {
				continue
			}
			if cached, ok := cache.Get(chunks[i].ContentHash); ok {
				descs[i] = cached
				continue
			}
			if !available {
				continue
			}
			generated, err := GenerateDescription(ctx, client, chunks[i])
			if err != nil {
				continue
			}
			if generated == "" {
				continue
			}
			descs[i] = generated
			cache.Put(chunks[i].ContentHash, generated)
			atomic.AddInt64(&newDescs, 1)
		}
	}

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go worker()
	}
	for i := range chunks {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	_ = cache.Save()

	return descs, int(newDescs)
}
