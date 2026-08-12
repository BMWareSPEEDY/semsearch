package main

import "os"

func main() {
	only := map[string]bool{}
	for _, a := range os.Args[1:] {
		only[a] = true
	}
	repos := []struct {
		key   string
		cases []BenchCase
	}{
		{"gin", ginCases},
		{"cobra", cobraCases},
		{"mux", muxCases},
		{"clap", clapCases},
		{"hugo", hugoCases},
	}
	for _, r := range repos {
		if len(only) > 0 && !only[r.key] {
			continue
		}
		diagRepo(r.key, r.cases)
	}
}
