// Package soak checks that parsing many files leaks neither memory nor goroutines. It is its own
// module so that the library itself keeps no dependencies, not even test-only ones.
//
//	go test ./...                         # the short soak, 50,000 files, as CI runs it
//	SOAK_FILES=200000 SOAK_PROFILES=DIR go test -run Soak -v -timeout 30m ./...
package soak

import (
	"bytes"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"sync"
	"testing"

	"go.uber.org/goleak"

	"github.com/jotadrilo/exif"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

// inputs are the testdata files, and copies of them cut short or with bytes flipped.
func inputs(t *testing.T) [][]byte {
	t.Helper()
	paths, err := filepath.Glob("../testdata/*.*")
	if err != nil || len(paths) == 0 {
		t.Fatal("no testdata", err)
	}
	rng := rand.New(rand.NewPCG(1, 2))
	var out [][]byte
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, data)
		for _, cut := range []int{2, 12, 64, 200, len(data) / 2, len(data) - 1} {
			if cut < len(data) {
				out = append(out, data[:cut])
			}
		}
		for range 6 {
			flipped := append([]byte(nil), data...)
			for range 8 {
				flipped[rng.IntN(min(len(flipped), 4096))] ^= byte(1 + rng.IntN(255))
			}
			out = append(out, flipped)
		}
	}
	return out
}

// use reads everything a caller would, so nothing is optimized away.
func use(data []byte) int {
	n := 0
	x, _ := exif.Parse(data)
	n += x.Orientation()
	if g, ok := x.GPS(); ok {
		n += int(g.Latitude)
	}
	if t, _, ok := x.DateTimeOriginal(); ok {
		n += t.Second()
	}
	n += len(x.Make()) + len(x.Model())
	for range x.Tags() {
		n++
	}
	y, _ := exif.Decode(bytes.NewReader(data))
	n += y.Orientation()
	_ = exif.Strip(io.Discard, bytes.NewReader(data), exif.KeepOrientation())
	return n
}

type sample struct {
	files              int
	heapInuse, objects uint64
	numGC              uint32
	rssKB              int64
}

func TestSoak_TheHeapPlateaus(t *testing.T) {
	files := 50_000
	if s := os.Getenv("SOAK_FILES"); s != "" {
		var err error
		if files, err = strconv.Atoi(s); err != nil {
			t.Fatal(err)
		}
	}
	profiles := os.Getenv("SOAK_PROFILES")
	if profiles != "" {
		// Record every allocation, so that the profiles diff exactly.
		old := runtime.MemProfileRate
		runtime.MemProfileRate = 1
		defer func() { runtime.MemProfileRate = old }()
	}
	in := inputs(t)
	const every = 5_000
	var samples []sample
	sink := 0
	for i := 1; i <= files; i++ {
		sink += use(in[i%len(in)])
		if i%1_000 == 0 && os.Getenv("SOAK_NO_BURSTS") == "" {
			// A concurrent burst: 8 goroutines, 50 files each.
			var wg sync.WaitGroup
			for w := range 8 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for j := range 50 {
						_ = use(in[(i+w*50+j)%len(in)])
					}
				}()
			}
			wg.Wait()
		}
		if i%every == 0 {
			runtime.GC()
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			samples = append(samples, sample{i, m.HeapInuse, m.HeapObjects, m.NumGC, rssKB()})
			if profiles != "" && (i == 10_000 || i == files) {
				writeHeapProfile(t, filepath.Join(profiles, fmt.Sprintf("heap-%d.pprof", i)))
			}
		}
	}
	t.Logf("checksum %d", sink)
	for _, s := range samples {
		t.Logf("%7d files: HeapInuse %6.2f MiB, HeapObjects %6d, NumGC %5d, RSS %6.1f MiB", s.files,
			float64(s.heapInuse)/(1<<20), s.objects, s.numGC, float64(s.rssKB)/1024)
	}
	if len(samples) < 6 {
		t.Fatalf("%d samples; run at least %d files", len(samples), 6*every)
	}
	// The runtime's own bookkeeping (goroutine and pool structures) settles during the first
	// samples, so the plateau is the peak of the middle third: the last third may not rise above
	// it by more than a margin.
	n := len(samples)
	var plateau sample
	for _, s := range samples[n/3 : 2*n/3] {
		plateau.heapInuse = max(plateau.heapInuse, s.heapInuse)
		plateau.objects = max(plateau.objects, s.objects)
		plateau.rssKB = max(plateau.rssKB, s.rssKB)
	}
	for _, s := range samples[2*n/3:] {
		if s.heapInuse > plateau.heapInuse+plateau.heapInuse/4+1<<20 || s.objects > plateau.objects+plateau.objects/7+256 {
			t.Errorf("at %d files the heap grew past its plateau: %d bytes in %d objects, plateau %d bytes in %d objects",
				s.files, s.heapInuse, s.objects, plateau.heapInuse, plateau.objects)
		}
		if s.rssKB > plateau.rssKB+plateau.rssKB/4+4096 {
			t.Errorf("at %d files RSS grew past its plateau: %d KiB, plateau %d KiB", s.files, s.rssKB, plateau.rssKB)
		}
	}
	base, last := samples[0], samples[n-1]
	if last.numGC <= base.numGC {
		t.Logf("no collection after %d files", base.files)
	}
}

func writeHeapProfile(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := pprof.Lookup("heap").WriteTo(f, 0); err != nil {
		t.Fatal(err)
	}
}

// rssKB returns the resident set size now, on Linux, or the peak elsewhere.
func rssKB() int64 {
	if b, err := os.ReadFile("/proc/self/statm"); err == nil {
		var size, resident int64
		if _, err := fmt.Sscan(string(b), &size, &resident); err == nil {
			return resident * int64(os.Getpagesize()) / 1024
		}
	}
	return peakRSSKB()
}
