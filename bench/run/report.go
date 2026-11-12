package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type config struct {
	corpus, native, raw string
	minTime             time.Duration
	rssRuns, count      int
	maxLoad             float64
	procs               int
	summary             string
}

// runner runs one measurement of a Go library (this binary as a child) or a native harness.
type runner struct {
	name string // as shown
	key  string // as benchstat keys it: no spaces
	cmd  func(mode, file, task string) *exec.Cmd
}

func run(r runner, mode, file, task string, v any) error {
	cmd := r.cmd(mode, file, task)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("%s %s: %v %s", r.name, mode, err, stderr.String())
	}
	return json.Unmarshal(bytes.TrimSpace(out), v)
}

type reference struct {
	SourceFile         string
	Orientation        *int
	GPSLatitude        *float64
	GPSLongitude       *float64
	GPSAltitude        *float64
	DateTimeOriginal   string
	OffsetTimeOriginal string
}

const (
	libexifKey = "libexif"
	exiv2Key   = "exiv2"
)

func runners(c config) []runner {
	self, err := os.Executable()
	must(err)
	child := func(name string) func(mode, file, task string) *exec.Cmd {
		return func(mode, file, task string) *exec.Cmd {
			cmd := exec.Command(self, "-child", mode, "-lib", name, "-file", file, "-task", task,
				"-time", c.minTime.String(), "-rss", fmt.Sprint(c.rssRuns), "-workers", fmt.Sprint(c.procs))
			cmd.Env = append(os.Environ(), fmt.Sprintf("GOMAXPROCS=%d", c.procs))
			return cmd
		}
	}
	var rs []runner
	for _, l := range libs {
		rs = append(rs, runner{l.name, strings.NewReplacer(" ", "-", "/", "_").Replace(l.name), child(l.name)})
	}
	for _, n := range []struct{ name, key, bin string }{
		{"libexif (C)", libexifKey, "libexif_bench"},
		{"exiv2 (C++)", exiv2Key, "exiv2_bench"},
	} {
		if c.native == "" {
			continue
		}
		bin := filepath.Join(c.native, n.bin)
		if _, err := os.Stat(bin); err != nil {
			continue
		}
		rs = append(rs, runner{n.name, n.key, func(mode, file, task string) *exec.Cmd {
			return exec.Command(bin, mode, file, task, fmt.Sprint(c.minTime.Seconds()), fmt.Sprint(c.rssRuns), fmt.Sprint(c.procs))
		}})
	}
	return rs
}

// loadAverage returns the 1-minute load average of the machine (the host's, in a container).
// EXIF_BENCH_LOADAVG_FILE names a file the host keeps its own load average in, since a
// container's /proc/loadavg shows only its VM's.
func loadAverage() float64 {
	if p := os.Getenv("EXIF_BENCH_LOADAVG_FILE"); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			if fs := strings.Fields(strings.Trim(string(b), "{} \n")); len(fs) > 0 {
				f, _ := strconv.ParseFloat(fs[0], 64)
				return f
			}
		}
	}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		f, _ := strconv.ParseFloat(strings.Fields(string(b))[0], 64)
		return f
	}
	out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	if err != nil {
		return -1
	}
	f, _ := strconv.ParseFloat(strings.Fields(strings.Trim(string(out), "{} \n"))[0], 64)
	return f
}

func cpuModel() string {
	if out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	if b, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if k, v, ok := strings.Cut(line, ":"); ok && (strings.TrimSpace(k) == "model name" || strings.TrimSpace(k) == "CPU part") {
				return strings.TrimSpace(v)
			}
		}
	}
	if out, err := exec.Command("lscpu").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if k, v, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(k) == "Model name" {
				return strings.TrimSpace(v)
			}
		}
	}
	return "unknown"
}

type caseKey struct{ file, task, lib string }

func report(c config) {
	rs := runners(c)
	files, err := filepath.Glob(filepath.Join(c.corpus, "*"))
	must(err)
	sort.Strings(files)
	tasks := []string{"orientation", "all"}

	var rawOut bytes.Buffer
	cpu := cpuModel()
	fmt.Fprintf(&rawOut, "goos: %s\ngoarch: %s\ncpu: %s\ngo: %s\ngomaxprocs: %d\ncpus: %d\n",
		runtime.GOOS, runtime.GOARCH, cpu, runtime.Version(), c.procs, runtime.NumCPU())
	startLoad := loadAverage()
	fmt.Fprintf(os.Stderr, "start: load %.2f, %d runners, %d files, %d rounds\n", startLoad, len(rs), len(files), c.count)

	// Warm up: one discarded sample of every case, filling caches and the page cache.
	for _, f := range files {
		for _, r := range rs {
			var t timing
			_ = run(r, "time", f, "all", &t)
		}
	}

	times := map[caseKey][]float64{}
	allocs := map[caseKey][]float64{}
	bytesOp := map[caseKey][]float64{}
	rss := map[caseKey][]float64{}
	tput := map[caseKey][]float64{}
	p50, p95, p99 := map[caseKey][]float64{}, map[caseKey][]float64{}, map[caseKey][]float64{}
	gcs, gcPause := map[caseKey][]float64{}, map[caseKey][]float64{}
	var loads []float64
	redone := 0
	for round := 0; round < c.count; round++ {
		before := waitForQuiet(c.maxLoad)
		var lines []string
		sample := map[caseKey]timing{}
		rssSample := map[caseKey]timing{}
		tputSample := map[caseKey]timing{}
		latSample := map[caseKey]timing{}
		// Rotate the order every round, so no runner always follows the same one.
		order := append(append([]runner(nil), rs[round%len(rs):]...), rs[:round%len(rs)]...)
		for _, f := range files {
			for _, task := range tasks {
				for _, r := range order {
					var t timing
					if err := run(r, "time", f, task, &t); err != nil {
						fmt.Fprintln(os.Stderr, short(err.Error()))
						continue
					}
					sample[caseKey{filepath.Base(f), task, r.key}] = t
				}
			}
			for _, r := range order {
				var t timing
				if err := run(r, "rss", f, "all", &t); err == nil {
					rssSample[caseKey{filepath.Base(f), "all", r.key}] = t
				}
				var lt timing
				if err := run(r, "latency", f, "all", &lt); err == nil && lt.P50 > 0 {
					latSample[caseKey{filepath.Base(f), "all", r.key}] = lt
				}
				var tp timing
				if err := run(r, "throughput", f, "all", &tp); err == nil && tp.OpsS > 0 {
					tputSample[caseKey{filepath.Base(f), "all", r.key}] = tp
				}
			}
		}
		after := loadAverage()
		if after > c.maxLoad && redone < 3 {
			redone++
			fmt.Fprintf(os.Stderr, "round %d: load rose to %.2f, redone\n", round+1, after)
			round--
			continue
		}
		loads = append(loads, before, after)
		fmt.Fprintf(os.Stderr, "round %d/%d: load %.2f -> %.2f\n", round+1, c.count, before, after)
		for k, t := range latSample {
			p50[k] = append(p50[k], t.P50)
			p95[k] = append(p95[k], t.P95)
			p99[k] = append(p99[k], t.P99)
			lines = append(lines, fmt.Sprintf("BenchmarkLatency/file=%s/lib=%s 1 %.1f p50-ns %.1f p95-ns %.1f p99-ns", k.file, k.lib, t.P50, t.P95, t.P99))
		}
		for k, t := range sample {
			if t.AllocsOp >= 0 && k.task == "all" {
				gcs[k] = append(gcs[k], t.GCs1k)
				gcPause[k] = append(gcPause[k], t.GCPause1k)
			}
			times[k] = append(times[k], t.NsOp)
			allocs[k] = append(allocs[k], t.AllocsOp)
			bytesOp[k] = append(bytesOp[k], t.BytesOp)
			line := fmt.Sprintf("BenchmarkRead/file=%s/task=%s/lib=%s 1 %.1f ns/op", k.file, k.task, k.lib, t.NsOp)
			if t.AllocsOp >= 0 {
				line += fmt.Sprintf(" %.0f B/op %.2f allocs/op", t.BytesOp, t.AllocsOp)
			}
			lines = append(lines, line)
		}
		for k, t := range rssSample {
			rss[k] = append(rss[k], float64(t.MaxRSSKB))
			lines = append(lines, fmt.Sprintf("BenchmarkPeakRSS/file=%s/lib=%s 1 %d peak-KiB", k.file, k.lib, t.MaxRSSKB))
		}
		for k, t := range tputSample {
			tput[k] = append(tput[k], t.OpsS)
			lines = append(lines, fmt.Sprintf("BenchmarkThroughput/file=%s/workers=%d/lib=%s 1 %.1f files/s", k.file, c.procs, k.lib, t.OpsS))
		}
		sort.Strings(lines)
		fmt.Fprintf(&rawOut, "# round %d load %.2f %.2f\n%s\n", round+1, before, after, strings.Join(lines, "\n"))
	}
	if c.raw != "" {
		must(os.WriteFile(c.raw, rawOut.Bytes(), 0o644))
	}
	if c.summary != "" {
		sum := summary{Procs: c.procs, CPU: cpu, Go: runtime.Version(), Names: map[string]string{}, Latency: map[string]float64{}, Throughput: map[string]float64{}}
		for _, r := range rs {
			sum.Order = append(sum.Order, r.key)
			sum.Names[r.key] = r.name
		}
		for _, f := range files {
			sum.Files = append(sum.Files, filepath.Base(f))
		}
		for k, v := range times {
			if m, _ := medianCI(v); !math.IsNaN(m) {
				sum.Latency[k.file+"|"+k.task+"|"+k.lib] = m
			}
		}
		for k, v := range tput {
			if m, _ := medianCI(v); !math.IsNaN(m) {
				sum.Throughput[k.file+"|"+k.lib] = m
			}
		}
		b, err := json.MarshalIndent(sum, "", " ")
		must(err)
		must(os.WriteFile(c.summary, b, 0o644))
	}

	// What each library reads, against exiftool.
	refs := map[string]reference{}
	out, err := exec.Command("exiftool", append([]string{"-n", "-j", "-Orientation", "-GPSLatitude", "-GPSLongitude", "-GPSAltitude", "-DateTimeOriginal", "-OffsetTimeOriginal"}, files...)...).Output()
	must(err)
	var list []reference
	must(json.Unmarshal(out, &list))
	for _, r := range list {
		refs[r.SourceFile] = r
	}

	sortedLoads := append([]float64(nil), loads...)
	sort.Float64s(sortedLoads)
	fmt.Printf("- Platform: %s/%s, %s, %d CPUs visible, GOMAXPROCS=%d for the Go children.\n", runtime.GOOS, runtime.GOARCH, cpu, runtime.NumCPU(), c.procs)
	fmt.Printf("- Go %s. %d interleaved rounds, each sample at least %s, after one warm-up round; %d rounds redone for load above %.1f.\n",
		runtime.Version(), c.count, c.minTime, redone, c.maxLoad)
	if len(sortedLoads) > 0 {
		fmt.Printf("- 1-minute load average during the kept rounds: median %.2f, max %.2f.\n", median(sortedLoads), sortedLoads[len(sortedLoads)-1])
	}
	fmt.Printf("- Latency percentiles time each parse alone and subtract the clock's own cost; the clock ticks every 41.7 ns on Apple silicon (24 MHz), which bounds the resolution of the fastest rows.\n")
	fmt.Printf("- Cells: median ± the 95%% confidence interval of the median (order statistics), and the ratio to libexif and exiv2 medians from the same run (below 1 is faster). Peak RSS is the median after %d parses from the file path.\n\n", c.rssRuns)

	agree := map[string][2]int{}
	agreeCell := map[caseKey]string{}
	fails := map[caseKey]bool{}
	for _, f := range files {
		for _, r := range rs {
			k := caseKey{filepath.Base(f), "", r.key}
			agreeCell[k], fails[k] = agreement(r, f, refs[f], agree)
		}
	}
	for _, f := range files {
		base := filepath.Base(f)
		st, _ := os.Stat(f)
		fmt.Printf("#### %s (%s)\n\n", base, size(st.Size()))
		fmt.Printf("| Library | orientation only | × libexif | × exiv2 | orientation + GPS + date | × libexif | × exiv2 | allocs/op | B/op | peak RSS | throughput, %d concurrent | agrees with exiftool |\n", c.procs)
		fmt.Println("|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|---|")
		for _, r := range rs {
			cells := []string{r.name}
			if r.key == "baseline" {
				k := caseKey{base, "all", r.key}
				rm, _ := medianCI(rss[k])
				fmt.Printf("| baseline: open and close the file | | | | | | | | | %.1f MiB | | |\n", rm/1024)
				continue
			}
			failed := fails[caseKey{base, "", r.key}]
			mark := ""
			if failed {
				mark = " †"
			}
			for _, task := range tasks {
				k := caseKey{base, task, r.key}
				m, ci := medianCI(times[k])
				if math.IsNaN(m) {
					cells = append(cells, "—", "", "")
					continue
				}
				rl, re := "n/a", "n/a"
				if !failed && !fails[caseKey{base, "", libexifKey}] {
					rl = ratio(m, times[caseKey{base, task, libexifKey}])
				}
				if !failed && !fails[caseKey{base, "", exiv2Key}] {
					re = ratio(m, times[caseKey{base, task, exiv2Key}])
				}
				cells = append(cells, fmt.Sprintf("%s ±%.0f%%%s", ns(m), ci, mark), rl, re)
			}
			k := caseKey{base, "all", r.key}
			am, _ := medianCI(allocs[k])
			bm, _ := medianCI(bytesOp[k])
			if math.IsNaN(am) || am < 0 {
				cells = append(cells, "n/a", "n/a")
			} else {
				cells = append(cells, num(am), size(int64(bm)))
			}
			if rm, _ := medianCI(rss[k]); !math.IsNaN(rm) {
				cells = append(cells, fmt.Sprintf("%.1f MiB", rm/1024))
			} else {
				cells = append(cells, "—")
			}
			if tm, ci := medianCI(tput[k]); !math.IsNaN(tm) {
				cells = append(cells, fmt.Sprintf("%s ±%.0f%%", rate(tm), ci))
			} else {
				cells = append(cells, "—")
			}
			cells = append(cells, agreeCell[caseKey{base, "", r.key}])
			fmt.Println("| " + strings.Join(cells, " | ") + " |")
		}
		fmt.Println()
		fmt.Println("Latency of each parse alone (orientation + GPS + date), and the Go garbage collector's work per 1,000 parses:")
		fmt.Println()
		fmt.Println("| Library | p50 | p95 | p99 | GC cycles / 1k parses | GC pause / 1k parses |")
		fmt.Println("|---|--:|--:|--:|--:|--:|")
		for _, r := range rs {
			if r.key == "baseline" {
				continue
			}
			k := caseKey{base, "all", r.key}
			a, _ := medianCI(p50[k])
			b, _ := medianCI(p95[k])
			c, _ := medianCI(p99[k])
			if math.IsNaN(a) {
				continue
			}
			mark := ""
			if fails[caseKey{base, "", r.key}] {
				mark = " †"
			}
			g, gp := "n/a", "n/a"
			if gm, _ := medianCI(gcs[k]); !math.IsNaN(gm) {
				pm, _ := medianCI(gcPause[k])
				g, gp = fmt.Sprintf("%.2f", gm), ns(pm)
			}
			fmt.Printf("| %s%s | %s | %s | %s | %s | %s |\n", r.name, mark, ns(a), ns(b), ns(c), g, gp)
		}
		fmt.Println()
	}
	fmt.Println("† The library cannot read this file: the time is how long it takes to give up, and no ratio is given.")
	fmt.Println()
	fmt.Println("#### Agreement with exiftool, all files")
	fmt.Println()
	fmt.Println("| Library | fields that agree |")
	fmt.Println("|---|--:|")
	for _, r := range rs {
		if a, ok := agree[r.key]; ok {
			fmt.Printf("| %s | %d/%d |\n", r.name, a[0], a[1])
		}
	}
}

// waitForQuiet waits, up to five minutes, for the load average to fall to max, and returns it.
func waitForQuiet(max float64) float64 {
	deadline := time.Now().Add(5 * time.Minute)
	for {
		l := loadAverage()
		if l <= max || time.Now().After(deadline) {
			return l
		}
		fmt.Fprintf(os.Stderr, "load %.2f above %.2f, waiting\n", l, max)
		time.Sleep(15 * time.Second)
	}
}

func agreement(r runner, f string, ref reference, agree map[string][2]int) (string, bool) {
	if r.key == "baseline" {
		return "", false
	}
	var v result
	hasMeta := ref.Orientation != nil || ref.GPSLatitude != nil || ref.DateTimeOriginal != ""
	if err := run(r, "values", f, "all", &v); err != nil {
		return "error", true
	}
	if v.Err != "" && hasMeta {
		_, total, _ := compare(result{}, ref)
		a := agree[r.key]
		a[1] += total
		agree[r.key] = a
		return "fails: " + short(v.Err), true
	}
	ok, total, diffs := compare(v, ref)
	a := agree[r.key]
	a[0] += ok
	a[1] += total
	agree[r.key] = a
	s := fmt.Sprintf("%d/%d", ok, total)
	if len(diffs) > 0 {
		s += " (" + strings.Join(diffs, "; ") + ")"
	}
	return s, false
}

func ratio(m float64, base []float64) string {
	b, _ := medianCI(base)
	if math.IsNaN(b) || b == 0 {
		return "n/a"
	}
	r := m / b
	if r < 0.1 {
		return fmt.Sprintf("%.3f", r)
	}
	return fmt.Sprintf("%.2f", r)
}

func median(sorted []float64) float64 {
	n := len(sorted)
	if n == 0 {
		return math.NaN()
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// medianCI returns the median and the half-width, in percent of it, of a distribution-free 95%
// confidence interval: the order statistics x(k) and x(n-k+1) with the largest k whose binomial
// coverage is still at least 95%.
func medianCI(xs []float64) (float64, float64) {
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	m := median(s)
	if n < 6 || math.IsNaN(m) || m == 0 {
		return m, math.NaN()
	}
	k := 0
	for j := 1; j <= n/2; j++ {
		if 1-2*binomCDF(n, j-1) >= 0.95 {
			k = j
		}
	}
	if k == 0 {
		return m, math.NaN()
	}
	lo, hi := s[k-1], s[n-k]
	return m, 100 * math.Max(m-lo, hi-m) / m
}

// binomCDF is P(X <= k) for X ~ Binomial(n, 1/2).
func binomCDF(n, k int) float64 {
	p := 0.0
	c := 1.0
	for i := 0; i <= k; i++ {
		if i > 0 {
			c = c * float64(n-i+1) / float64(i)
		}
		p += c
	}
	return p / math.Pow(2, float64(n))
}

// summary is one run's medians, keyed "file|task|lib" (latency, ns) and "file|lib" (files/s).
type summary struct {
	Procs      int
	CPU, Go    string
	Order      []string
	Names      map[string]string
	Files      []string
	Latency    map[string]float64
	Throughput map[string]float64
}

func rate(v float64) string {
	switch {
	case v >= 1e6:
		return fmt.Sprintf("%.2f M/s", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.1f k/s", v/1e3)
	}
	return fmt.Sprintf("%.0f /s", v)
}

// combine prints, from runs at several CPU counts, the latency at each count and the
// throughput scaling against the run with the fewest CPUs.
func combine(paths []string) {
	var sums []summary
	for _, p := range paths {
		b, err := os.ReadFile(p)
		must(err)
		var s summary
		must(json.Unmarshal(b, &s))
		sums = append(sums, s)
	}
	sort.Slice(sums, func(i, j int) bool { return sums[i].Procs < sums[j].Procs })
	base := sums[0]
	header := func(first string, per func(s summary) []string) {
		cols := []string{first}
		for _, s := range sums {
			cols = append(cols, per(s)...)
		}
		fmt.Println("| " + strings.Join(cols, " | ") + " |")
		align := []string{"---"}
		for range cols[1:] {
			align = append(align, "--:")
		}
		fmt.Println("|" + strings.Join(align, "|") + "|")
	}
	for _, f := range base.Files {
		fmt.Printf("#### %s: latency of one parse (orientation + GPS + date), median\n\n", f)
		header("Library", func(s summary) []string { return []string{fmt.Sprintf("%d CPU", s.Procs)} })
		for _, lib := range base.Order {
			if lib == "baseline" {
				continue
			}
			cells := []string{base.Names[lib]}
			for _, s := range sums {
				if v, ok := s.Latency[f+"|all|"+lib]; ok {
					cells = append(cells, ns(v))
				} else {
					cells = append(cells, "—")
				}
			}
			fmt.Println("| " + strings.Join(cells, " | ") + " |")
		}
		fmt.Println()
		fmt.Printf("#### %s: throughput with N concurrent parses on N CPUs, and scaling efficiency against %d CPU\n\n", f, base.Procs)
		header("Library", func(s summary) []string {
			if s.Procs == base.Procs {
				return []string{fmt.Sprintf("%d CPU", s.Procs)}
			}
			return []string{fmt.Sprintf("%d CPUs", s.Procs), "efficiency"}
		})
		for _, lib := range base.Order {
			if lib == "baseline" {
				continue
			}
			cells := []string{base.Names[lib]}
			one := base.Throughput[f+"|"+lib]
			for _, s := range sums {
				v, ok := s.Throughput[f+"|"+lib]
				if !ok {
					cells = append(cells, "—")
				} else {
					cells = append(cells, rate(v))
				}
				if s.Procs != base.Procs {
					if ok && one > 0 {
						cells = append(cells, fmt.Sprintf("%.0f%%", 100*v/(one*float64(s.Procs)/float64(base.Procs))))
					} else {
						cells = append(cells, "—")
					}
				}
			}
			fmt.Println("| " + strings.Join(cells, " | ") + " |")
		}
		fmt.Println()
	}
	for _, s := range sums {
		fmt.Printf("- %d CPU run: %s, %s.\n", s.Procs, s.CPU, s.Go)
	}
}
