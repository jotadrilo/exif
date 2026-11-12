// Command run benchmarks EXIF readers on a corpus and checks what they read against exiftool.
//
//	go run ./run -corpus DIR [-native DIR] > results.md
//
// Every measurement runs in its own child process, so that peak RSS belongs to one library.
// A child reads the file once, then parses it from memory in a loop for at least -time (the
// time and allocation columns), or parses it from its path -rss times (the peak RSS column).
// The native harnesses in ../native follow the same protocol.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// result is what a library reads. Taken is DateTimeOriginal as written, "YYYY:MM:DD HH:MM:SS".
type result struct {
	Orientation int     `json:"orientation"`
	HasGPS      bool    `json:"has_gps"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	HasAlt      bool    `json:"has_alt"`
	Alt         float64 `json:"alt"`
	Taken       string  `json:"taken"`
	Err         string  `json:"err,omitempty"`
}

type timing struct {
	NsOp     float64 `json:"ns_op"`
	AllocsOp float64 `json:"allocs_op"`
	BytesOp  float64 `json:"bytes_op"`
	MaxRSSKB int64   `json:"maxrss_kb"`
	OpsS     float64 `json:"ops_s"`
	Err      string  `json:"err,omitempty"`
}

// lib reads one image. mem parses bytes in memory; file parses from a path, as a server
// handed an upload on disk would. all asks for orientation, GPS and the date, else only the
// orientation.
type lib struct {
	name string
	mem  func(data []byte, all bool) (result, error)
	file func(path string, all bool) (result, error)
}

func main() {
	child := flag.String("child", "", "internal: time|rss|values")
	libName := flag.String("lib", "", "internal: library")
	file := flag.String("file", "", "internal: image")
	task := flag.String("task", "all", "internal: all|orientation")
	corpus := flag.String("corpus", "", "directory of images")
	native := flag.String("native", "", "directory of the native harness binaries")
	minTime := flag.Duration("time", 300*time.Millisecond, "minimum timed duration per sample")
	rssRuns := flag.Int("rss", 50, "parses from the path per peak-RSS sample")
	count := flag.Int("count", 10, "rounds; each round takes one sample of every case")
	maxLoad := flag.Float64("maxload", float64(runtime.NumCPU())/4, "1-minute load average above which a round is redone")
	raw := flag.String("raw", "", "file for the raw samples in benchstat format")
	procs := flag.Int("procs", 1, "GOMAXPROCS of the Go children, and the workers of the throughput runs")
	summary := flag.String("summary", "", "file for the medians in JSON, for -combine")
	combineFiles := flag.String("combine", "", "comma-separated summaries: print the scaling tables")
	workers := flag.Int("workers", 1, "internal: concurrent parses")
	flag.Parse()
	if *combineFiles != "" {
		combine(strings.Split(*combineFiles, ","))
		return
	}

	if *child != "" {
		runChild(*child, *libName, *file, *task == "all", *minTime, *rssRuns, *workers)
		return
	}
	if *corpus == "" {
		flag.Usage()
		os.Exit(2)
	}
	report(config{corpus: *corpus, native: *native, minTime: *minTime, rssRuns: *rssRuns,
		count: *count, maxLoad: *maxLoad, raw: *raw, procs: *procs, summary: *summary})
}

func findLib(name string) lib {
	for _, l := range libs {
		if l.name == name {
			return l
		}
	}
	fmt.Fprintln(os.Stderr, "unknown library", name)
	os.Exit(2)
	return lib{}
}

func runChild(mode, name, file string, all bool, minTime time.Duration, rssRuns, workers int) {
	l := findLib(name)
	out := json.NewEncoder(os.Stdout)
	switch mode {
	case "values":
		data, err := os.ReadFile(file)
		must(err)
		r, err := l.mem(data, true)
		if err != nil {
			r = result{Orientation: 1, Err: err.Error()}
		}
		_ = out.Encode(r)
	case "time":
		data, err := os.ReadFile(file)
		must(err)
		n := 1
		for {
			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			start := time.Now()
			for range n {
				_, _ = l.mem(data, all)
			}
			elapsed := time.Since(start)
			runtime.ReadMemStats(&after)
			if elapsed >= minTime {
				_ = out.Encode(timing{
					NsOp:     float64(elapsed.Nanoseconds()) / float64(n),
					AllocsOp: float64(after.Mallocs-before.Mallocs) / float64(n),
					BytesOp:  float64(after.TotalAlloc-before.TotalAlloc) / float64(n),
				})
				return
			}
			n = int(float64(n) * math.Max(2, 1.2*float64(minTime)/float64(max(elapsed, 1))))
		}
	case "throughput":
		data, err := os.ReadFile(file)
		must(err)
		_, _ = l.mem(data, all)
		var wg sync.WaitGroup
		counts := make([]int64, workers)
		start := time.Now()
		deadline := start.Add(minTime)
		for w := range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var n int64
				for time.Now().Before(deadline) {
					for range 16 {
						_, _ = l.mem(data, all)
					}
					n += 16
				}
				counts[w] = n
			}()
		}
		wg.Wait()
		var total int64
		for _, n := range counts {
			total += n
		}
		_ = out.Encode(timing{OpsS: float64(total) / time.Since(start).Seconds()})
	case "rss":
		for range rssRuns {
			_, _ = l.file(file, all)
		}
		var ru syscall.Rusage
		must(syscall.Getrusage(syscall.RUSAGE_SELF, &ru))
		_ = out.Encode(timing{MaxRSSKB: maxRSSKB(ru.Maxrss)})
	}
}

// maxRSSKB normalizes ru_maxrss, bytes on macOS and kilobytes on Linux.
func maxRSSKB(v int64) int64 {
	if runtime.GOOS == "darwin" {
		return v / 1024
	}
	return v
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func short(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	s = strings.ReplaceAll(s, "|", "/")
	if len(s) > 60 {
		s = s[:60] + "…"
	}
	return s
}

// compare checks orientation, latitude, longitude, altitude and DateTimeOriginal. A field
// exiftool does not find counts when the library also reports it absent.
func compare(v result, ref reference) (ok, total int, diffs []string) {
	check := func(name string, good bool, got string) {
		total++
		if good {
			ok++
		} else {
			diffs = append(diffs, name+" "+got)
		}
	}
	wantO := 1
	if ref.Orientation != nil {
		wantO = *ref.Orientation
	}
	check("orientation", v.Orientation == wantO, fmt.Sprint(v.Orientation))
	near := func(a float64, b *float64) bool { return b != nil && math.Abs(a-*b) < 1e-5 }
	if ref.GPSLatitude == nil {
		check("GPS", !v.HasGPS, "reported")
	} else {
		check("lat", v.HasGPS && near(v.Lat, ref.GPSLatitude), fmt.Sprintf("%.6f", v.Lat))
		check("lon", v.HasGPS && near(v.Lon, ref.GPSLongitude), fmt.Sprintf("%.6f", v.Lon))
	}
	if ref.GPSAltitude != nil {
		check("alt", v.HasAlt && near(v.Alt, ref.GPSAltitude), fmt.Sprintf("%g", v.Alt))
	}
	check("date", v.Taken == ref.DateTimeOriginal, fmt.Sprintf("%q", v.Taken))
	return ok, total, diffs
}

func ns(v float64) string {
	switch {
	case v >= 1e6:
		return fmt.Sprintf("%.2f ms", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.2f µs", v/1e3)
	}
	return fmt.Sprintf("%.0f ns", v)
}

func num(v float64) string {
	if v < 0 {
		return "n/a"
	}
	if v == math.Trunc(v) || v > 100 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

func bytesCell(v float64, name string) string {
	if v < 0 {
		return "n/a"
	}
	return size(int64(v))
}

func size(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
