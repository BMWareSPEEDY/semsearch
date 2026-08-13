package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type M struct{ Hashes map[string]string `json:"file_hashes"` }

func main() {
	for _, r := range []string{"clap", "hugo", "gin", "cobra", "mux"} {
		b, _ := os.ReadFile("scratch/external_eval/" + r + "/.semsearch/manifest.json")
		var m M
		json.Unmarshal(b, &m)
		n := 0
		for p := range m.Hashes {
			if strings.HasSuffix(p, "desc_cache.json") {
				n++
			}
		}
		fmt.Printf("%-6s total=%d desc_cache_indexed=%d\n", r, len(m.Hashes), n)
	}
}
