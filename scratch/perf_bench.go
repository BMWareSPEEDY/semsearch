package main

// perf_bench.go — performance harness for semsearch.
//
// Usage:
//   go run scratch/perf_bench.go scratch/evalcases_gin.go scratch/evalcases_cobra.go \
//          scratch/evalcases_mux.go scratch/evalcases_clap.go scratch/evalcases_hugo.go \
//          [out.json]
//
// Measures, per eval repo: index load, embedder init, per-query embed + hybrid
// search + rerank latency, plus process peak RSS and CPU time (rusage). Writes a
// JSON report and prints a summary. Run it before and after code changes to
// compare.

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/ashishpaliwal/semsearch/pkg/embedder"
	"github.com/ashishpaliwal/semsearch/pkg/indexer"
	"github.com/ashishpaliwal/semsearch/pkg/reranker"
)

type BenchCase struct {
	Query    string
	Category string
	Expected string
}

type repoSpec struct {
	key   string
	label string
	dir   string
	cases []BenchCase
}

var repos = []repoSpec{
	{key: "gin", label: "gin-gonic/gin (Go web framework)", dir: "scratch/external_eval/gin", cases: ginCases},
	{key: "cobra", label: "spf13/cobra (Go CLI framework)", dir: "scratch/external_eval/cobra", cases: cobraCases},
	{key: "mux", label: "gorilla/mux (Go HTTP router)", dir: "scratch/external_eval/mux", cases: muxCases},
	{key: "clap", label: "clap-rs/clap (Rust CLI argument parser)", dir: "scratch/external_eval/clap", cases: clapCases},
	{key: "hugo", label: "gohugoio/hugo (Go static site generator)", dir: "scratch/external_eval/hugo", cases: hugoCases},
}

type phaseStat struct {
	Total    time.Duration
	Min      time.Duration
	Max      time.Duration
	Count    int
	Median   time.Duration
	P95      time.Duration
	TotalMS  float64
	MinMS    float64
	MaxMS    float64
	AvgMS    float64
	MedianMS float64
	P95MS    float64
}

type repoReport struct {
	Key              string                 `json:"key"`
	Label            string                 `json:"label"`
	Chunks           int                    `json:"chunks"`
	LoadMS           float64                `json:"load_ms"`
	EmbedderInitMS   float64                `json:"embedder_init_ms"`
	FirstQueryMS     float64                `json:"first_query_ms"`
	AvgQueryMS       float64                `json:"avg_query_ms"`
	PerPhase         map[string]phaseStat   `json:"per_phase"`
	EmbedMS          float64                `json:"embed_ms_avg"`
	SearchMS         float64                `json:"search_ms_avg"`
	RerankMS         float64                `json:"rerank_ms_avg"`
	Queries          int                    `json:"queries"`
	Top5Hits         int                    `json:"top5_hits"`
	Top5Total        int                    `json:"top5_total"`
	Top1Hits         int                    `json:"top1_hits"`
	VectorBytes      int64                  `json:"vector_bytes_read"`
	PerQuerySamples  []querySample          `json:"per_query_samples,omitempty"`
	KeywordCacheUsed bool                   `json:"-"`
}

type querySample struct {
	Query   string  `json:"query"`
	EmbedMS float64 `json:"embed_ms"`
	Search  float64 `json:"search_ms"`
	Rerank  float64 `json:"rerank_ms"`
	Total   float64 `json:"total_ms"`
}

type report struct {
	Timestamp   string                   `json:"timestamp"`
	OS          string                   `json:"os"`
	Arch        string                   `json:"arch"`
	CPU         string                   `json:"cpu"`
	Cores       int                      `json:"cores"`
	MemGB       float64                  `json:"mem_gb"`
	GoVersion   string                   `json:"go_version"`
	PeakRSSMB   float64                  `json:"peak_rss_mb"`
	CPUUserSec  float64                  `json:"cpu_user_sec"`
	CPUSysSec   float64                  `json:"cpu_sys_sec"`
	WallSec     float64                  `json:"wall_sec"`
	Note        string                   `json:"note"`
	Repos       []repoReport             `json:"repos"`
	Totals      map[string]phaseStat     `json:"totals"`
}

func nowMs() float64 { return float64(time.Since(startTime).Microseconds()) / 1000.0 }

var startTime time.Time

func phaseStats(ds []time.Duration) phaseStat {
	var s phaseStat
	if len(ds) == 0 {
		return s
	}
	s.Count = len(ds)
	s.Min = ds[0]
	s.Max = ds[0]
	var total time.Duration
	for _, d := range ds {
		total += d
		if d < s.Min {
			s.Min = d
		}
		if d > s.Max {
			s.Max = d
		}
	}
	s.Total = total
	s.TotalMS = total.Seconds() * 1000
	s.MinMS = s.Min.Seconds() * 1000
	s.MaxMS = s.Max.Seconds() * 1000
	s.AvgMS = (total.Seconds() * 1000) / float64(len(ds))
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	med := ds[len(ds)/2]
	s.Median = med
	s.MedianMS = med.Seconds() * 1000
	idx := int(float64(len(ds)) * 0.95)
	if idx >= len(ds) {
		idx = len(ds) - 1
	}
	s.P95 = ds[idx]
	s.P95MS = ds[idx].Seconds() * 1000
	return s
}

func durationMS(d time.Duration) float64 { return d.Seconds() * 1000 }

func sysInfo() (string, string, float64) {
	cpu := runtime.GOARCH
	mem := float64(0)
	if runtime.GOOS == "darwin" {
		if b, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
			cpu = strings.TrimSpace(string(b))
		}
		if b, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
			fmt.Sscanf(strings.TrimSpace(string(b)), "%f", &mem)
			mem /= 1024 * 1024 * 1024
		}
	}
	return cpu, runtime.GOOS + "/" + runtime.GOARCH, mem
}

func rusage() (rssMB float64, userSec, sysSec float64) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0, 0, 0
	}
	ruMaxrss := float64(ru.Maxrss)
	if runtime.GOOS == "darwin" {
		rssMB = ruMaxrss / (1024 * 1024)
	} else {
		rssMB = ruMaxrss / 1024
	}
	userSec = float64(ru.Utime.Sec) + float64(ru.Utime.Usec)/1e6
	sysSec = float64(ru.Stime.Sec) + float64(ru.Stime.Usec)/1e6
	return
}

func main() {
	outPath := "scratch/perf_report.json"
	only := map[string]bool{}
	args := os.Args[1:]
	// last arg that is not a .go file is the output path
	var goFiles []string
	for _, a := range args {
		if strings.HasSuffix(a, ".go") {
			goFiles = append(goFiles, a)
		} else if strings.HasSuffix(a, ".json") {
			outPath = a
		} else {
			only[a] = true
		}
	}
	_ = goFiles

	cpu, osarch, memGB := sysInfo()
	rss0, _, _ := rusage()
	startTime = time.Now()

	tokenizerJSON, err := os.ReadFile("scratch/code_minilm/tokenizer.json")
	if err != nil {
		log.Fatalf("tokenizer: %v", err)
	}

	rep := &report{
		Timestamp: time.Now().Format(time.RFC3339),
		OS:        osarch,
		Arch:      runtime.GOARCH,
		CPU:       cpu,
		Cores:     runtime.NumCPU(),
		MemGB:     memGB,
		GoVersion: runtime.Version(),
		Note:      "",
		Repos:     []repoReport{},
		Totals:    map[string]phaseStat{},
	}

	allEmbed := []time.Duration{}
	allSearch := []time.Duration{}
	allRerank := []time.Duration{}
	allLoad := []time.Duration{}

	// embedder init (one-time, cold)
	t0 := time.Now()
	emb, err := embedder.NewEmbedder("scratch/code_minilm/model_quantized.onnx", tokenizerJSON)
	if err != nil {
		log.Fatalf("embedder: %v", err)
	}
	embedderInit := time.Since(t0)

	for _, rp := range repos {
		if len(only) > 0 && !only[rp.key] {
			continue
		}
		rr := repoReport{Key: rp.key, Label: rp.label, PerPhase: map[string]phaseStat{}}

		st := indexer.NewIndexStore(rp.dir)
		tLoad := time.Now()
		if err := st.Load(); err != nil {
			fmt.Printf("SKIP %-6s no index: %v\n", rp.key, err)
			continue
		}
		loadDur := time.Since(tLoad)
		allLoad = append(allLoad, loadDur)
		rr.LoadMS = durationMS(loadDur)
		rr.Chunks = st.Manifest.TotalChunks

		firstTotal := time.Duration(0)
		var embedD, searchD, rerankD []time.Duration
		top1, top5 := 0, 0

		r := reranker.NewReranker()
		for _, tc := range rp.cases {
			t := time.Now()
			qv, err := emb.EmbedBatch([]string{tc.Query}, "search_query: ")
			ed := time.Since(t)
			if err != nil {
				continue
			}

			t = time.Now()
			raw := st.HybridSearch(qv, tc.Query, 50, 0.0, 0.75, true, false)
			sd := time.Since(t)

			t = time.Now()
			cp := make([]indexer.SearchResult, len(raw))
			copy(cp, raw)
			res := r.Rerank(tc.Query, cp, 5)
			rd := time.Since(t)

			embedD = append(embedD, ed)
			searchD = append(searchD, sd)
			rerankD = append(rerankD, rd)

			hit := -1
			for k, x := range res {
				if strings.HasSuffix(x.Chunk.FilePath, tc.Expected) || strings.Contains(x.Chunk.FilePath, tc.Expected) {
					hit = k + 1
					break
				}
			}
			if hit >= 1 && hit <= 5 {
				top5++
			}
			if hit == 1 {
				top1++
			}

			total := ed + sd + rd
			if firstTotal == 0 {
				firstTotal = total
			}
			rr.PerQuerySamples = append(rr.PerQuerySamples, querySample{
				Query:   tc.Query,
				EmbedMS: durationMS(ed),
				Search:  durationMS(sd),
				Rerank:  durationMS(rd),
				Total:   durationMS(total),
			})
		}

		rr.Queries = len(rp.cases)
		rr.Top5Hits = top5
		rr.Top5Total = len(rp.cases)
		rr.Top1Hits = top1
		rr.EmbedderInitMS = durationMS(embedderInit)
		rr.FirstQueryMS = durationMS(firstTotal)
		rr.EmbedMS = phaseStats(embedD).AvgMS
		rr.SearchMS = phaseStats(searchD).AvgMS
		rr.RerankMS = phaseStats(rerankD).AvgMS
		rr.PerPhase["embed"] = phaseStats(embedD)
		rr.PerPhase["search"] = phaseStats(searchD)
		rr.PerPhase["rerank"] = phaseStats(rerankD)
		rr.PerPhase["load"] = phaseStats(allLoad)
		var sumQuery float64
		for _, s := range rr.PerQuerySamples {
			sumQuery += s.Total
		}
		rr.AvgQueryMS = sumQuery / float64(len(rr.PerQuerySamples))

		rep.Repos = append(rep.Repos, rr)
		allEmbed = append(allEmbed, embedD...)
		allSearch = append(allSearch, searchD...)
		allRerank = append(allRerank, rerankD...)

		fmt.Printf("%-6s chunks=%-6d load=%.1fms embed=%.1fms search=%.1fms rerank=%.1fms first=%.1fms top1=%d/%d top5=%d/%d\n",
			rp.key, rr.Chunks, rr.LoadMS, rr.EmbedMS, rr.SearchMS, rr.RerankMS, rr.FirstQueryMS,
			top1, len(rp.cases), top5, len(rp.cases))
	}

	rep.Totals["embed"] = phaseStats(allEmbed)
	rep.Totals["search"] = phaseStats(allSearch)
	rep.Totals["rerank"] = phaseStats(allRerank)
	rep.Totals["load"] = phaseStats(allLoad)

	rep.WallSec = time.Since(startTime).Seconds()
	rssMB, userSec, sysSec := rusage()
	rep.PeakRSSMB = rssMB
	rep.CPUUserSec = userSec
	rep.CPUSysSec = sysSec
	_ = rss0

	out, _ := json.MarshalIndent(rep, "", "  ")
	if err := os.WriteFile(outPath, out, 0644); err != nil {
		log.Printf("write report: %v", err)
	}
	fmt.Printf("\nreport written to %s\n", outPath)
	fmt.Printf("peak RSS: %.0f MB | cpu user %.1fs sys %.1fs | wall %.1fs\n", rssMB, userSec, sysSec, rep.WallSec)
}
