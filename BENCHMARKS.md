# Benchmarks

> **Provisional.** The full pass (`bench/bench.sh`) was stopped partway to free the machine and has
> not finished yet. Every number below comes from shorter runs on a loaded machine, and is labeled
> with how it was taken. A final pass on a quiet machine will replace this file with the tables
> `bench/assemble.sh` writes.

This package against the most used EXIF readers for Go, and against the native libraries other
ecosystems use:

- [rwcarlsen/goexif](https://github.com/rwcarlsen/goexif), the 2019 commit many projects still pin;
- [dsoprea/go-exif](https://github.com/dsoprea/go-exif) v3;
- [evanoberholster/imagemeta](https://github.com/evanoberholster/imagemeta);
- [bep/imagemeta](https://github.com/bep/imagemeta), Hugo's reader;
- [libexif](https://libexif.github.io/) (C) and [exiv2](https://exiv2.org/) (C++), through small
  harnesses in `bench/native`;
- [exiftool](https://exiftool.org/) as the reference for what each one reads.

## Method

Everything is in [`bench/`](bench) and runs with `bench/bench.sh`. There is one environment,
native macOS on one Mac, and one configuration, the same every time it runs:

- **Configuration.** GOMAXPROCS is left at its default (every CPU), every library runs at its
  defaults, and the native harnesses run their usual way.
- **Isolation.**
  - The run holds the machine-wide lock `/tmp/jotadrilo-bench.lock`.
  - It runs under `taskpolicy -t 0 -l 0`: the highest throughput and latency tiers, with no QoS
    clamp. taskpolicy can only clamp QoS down, so it cannot raise a process to user-interactive.
  - The libraries are interleaved in rounds, and the order rotates every round.
  - A warm-up round comes first.
  - A round is redone when the load average rises above a quarter of the cores.
- **Statistics.** Each cell is the median of 10 rounds, with the half-width of a
  distribution-free 95% confidence interval of the median. Every row also gives its ratio to
  libexif and to exiv2, both measured in the same run. The raw samples are kept in benchstat
  format, with benchstat's tables beside them.
- **Measurements.**
  - time per parse from memory, and allocations;
  - peak RSS when parsing from the path;
  - p50, p95 and p99 latency;
  - GC work per 1,000 parses;
  - throughput with as many concurrent parses as CPUs (18 here);
  - agreement with exiftool;
  - the long soak.
- **Environment.** `bench/env.sh` records it before and after each run: Mac model, chip and core
  levels, memory, macOS version, power source, low-power mode, load average, and the Go, C
  compiler and library versions, along with the exact command. The machine of the provisional
  numbers:

| | |
|---|---|
| Mac model | Mac17,9 |
| Chip | Apple M5 Pro: 6 "Super" + 12 "Performance" cores |
| Memory | 48 GiB |
| macOS | 26.3.2 (25D2150) |
| Power | AC power, low-power mode off (pmset powermode 0) |
| Go (bench module) | go1.27.0 (the library itself needs Go 1.23) |
| C compiler | Apple clang 21.0.0 |
| libexif / exiv2 / exiftool | 0.6.26 / 0.28.9 / 13.55 (Homebrew) |
| rwcarlsen/goexif | v0.0.0-20190401172101-9e8deecbddbd |
| dsoprea/go-exif/v3 | v3.0.1 |
| evanoberholster/imagemeta | v1.1.0 |
| bep/imagemeta | v1.1.1 |

A container script (`bench/container.sh`) remains as an optional extra. Its numbers are never the
published ones.

## Preliminary: camera JPEG (4.8 MiB), macOS host

These come from a smoke run in an earlier configuration: GOMAXPROCS=1, one worker for throughput, 6 interleaved rounds of 50 ms samples,
under the lock, with the load average between 6 and 11. They predate the GPS fast path, which made
this package's "orientation + GPS + date" about 3× faster (see the next section).

| Library | orientation only | × libexif | orientation + GPS + date | × libexif | allocs/op | B/op | peak RSS | throughput, 1 CPU | agrees with exiftool |
|---|--:|--:|--:|--:|--:|--:|--:|--:|---|
| **jotadrilo/exif** Parse | 34 ns | 0.002 | 242 ns | 0.012 | 0 | 0 B | 7.7 MiB | 4.07 M/s | 5/5 |
| **jotadrilo/exif** Decode (bytes.Reader) | 640 ns | 0.033 | 863 ns | 0.045 | 3 | 10.8 KiB | 7.7 MiB | 1.17 M/s | 5/5 |
| evanoberholster/imagemeta | 1.78 µs | 0.091 | 1.90 µs | 0.098 | 10 | 392 B | 7.8 MiB | 531 k/s | 5/5 |
| bep/imagemeta | 3.85 µs | 0.20 | 9.73 µs | 0.50 | 110 | 12.1 KiB | 7.8 MiB | 102 k/s | 5/5 |
| rwcarlsen/goexif | 19.2 µs | 0.98 | 18.8 µs | 0.97 | 513 | 129 KiB | 8.5 MiB | 51 k/s | 5/5 |
| libexif (C) | 19.6 µs | 1.00 | 19.4 µs | 1.00 | n/a | n/a | 5.8 MiB | 51 k/s | 5/5 |
| exiv2 (C++) | 47.6 µs | 2.44 | 49.9 µs | 2.58 | n/a | n/a | 7.6 MiB | 20 k/s | 5/5 |
| dsoprea/go-exif v3 | 599 µs | 30.6 | 643 µs | 33.2 | 596 | 15.2 MiB | 26.3 MiB | 1.4 k/s | 4/5 (whole-meter altitude) |

The confidence intervals were ±9% to ±19%, wide because of the load. Rankings were the same on the
other files:

- phone JPEG: Parse 30 ns / 213 ns; evanoberholster 0.96 µs; libexif 14.1 µs; exiv2 10.3 µs.
- PNG, WebP and TIFF:
  - this package: 43 to 62 ns for the orientation;
  - libexif cannot read them;
  - goexif reads only the TIFF;
  - evanoberholster takes 0.72 ms on the WebP, because it reads the file through.
- Agreement with exiftool over all files: this package, evanoberholster, bep and exiv2 28/28;
  dsoprea 24/28 (it truncates the altitude to whole meters); goexif 18/26 (no PNG or WebP);
  libexif 13/25 (JPEG only).

## Hot paths after optimization (this package alone)

`go test -bench Parse/camera.jpg`, before and after the GPS fast path: 10 interleaved runs each,
under the lock, compared with benchstat.

| Benchmark (testdata/camera.jpg) | before | after | change |
|---|--:|--:|--:|
| Parse + Orientation | 26.9 ns ± 4% | 27.4 ns ± 2% | ~ (p=0.165) |
| Parse + GPS | 119.2 ns ± 4% | 39.0 ns ± 3% | −67% (p=0.000) |
| Parse + DateTimeOriginal | 80.5 ns ± 0% | 68.5 ns ± 2% | −15% (p=0.000) |

All three paths make 0 allocations. One rejected attempt is worth recording: scanning IFD0
eagerly in `ParseTIFF`, to cache the orientation entry and the sub-IFD pointers, made Orientation
146% slower and GPS 48% slower (p=0.000, n=10). The parse stays lazy.

## Peak RSS on a 50 MB JPEG (EXIF at the start)

20 parses from the file path each, one process per library, under the lock. The baseline only
opens and closes the file.

| Library | peak RSS | over the baseline |
|---|--:|--:|
| baseline (Go: open + close) | 8.2 MiB | — |
| **jotadrilo/exif** Decode(*os.File) | 8.3 MiB | +0.1 MiB |
| rwcarlsen/goexif | 9.3 MiB | +1.1 MiB |
| evanoberholster/imagemeta | 8.6 MiB | +0.4 MiB |
| bep/imagemeta | 8.5 MiB | +0.3 MiB |
| dsoprea/go-exif v3 (SearchFileAndExtractExif) | 236.9 MiB | +228.7 MiB |
| libexif (C, its own baseline) | 5.9 MiB | — |
| exiv2 (C++) | 7.8 MiB | — |

`Decode` reads a few kilobytes of the 50 MB file: the SOI, the segment headers and the EXIF
segment.

## Memory over many files (soak)

`soak/` parsed 1,000,000 files in a loop with Parse, Decode, every accessor and Strip. The files
were valid and broken, with a burst of 8 concurrent goroutines every 1,000 files. It sampled after
a GC every 5,000 files (macOS, not under the lock):

```
   5000 files: HeapInuse 2.56 MiB, HeapObjects  784, NumGC   27, RSS 17.0 MiB
 200000 files: HeapInuse 2.34 MiB, HeapObjects 1497, NumGC 1007, RSS 27.1 MiB
 400000 files: HeapInuse 2.57 MiB, HeapObjects 1608, NumGC 2204, RSS 27.2 MiB
 600000 files: HeapInuse 3.44 MiB, HeapObjects 1843, NumGC 3411, RSS 27.3 MiB
 800000 files: HeapInuse 2.27 MiB, HeapObjects 1834, NumGC 4624, RSS 27.3 MiB
1000000 files: HeapInuse 2.48 MiB, HeapObjects 1857, NumGC 5831, RSS 27.3 MiB
```

- The heap in use stays flat. RSS stops growing at 27 MiB, the peak.
- HeapObjects settles at about 1,850 as the runtime's goroutine and pool caches fill. Without the
  concurrent bursts it settles at about 890.
- The heap profile at 10,000 files diffed against the one at 100,000 (`MemProfileRate=1`) shows
  no growth that this package's code holds on to. The difference is runtime bookkeeping
  (`mallocgc`, semaphore and WaitGroup internals) and objects allocated by the last parses before
  the profile.
- The test's assertion catches a planted leak of 64 bytes per file within 35,000 files. CI runs
  it with 50,000 files on every push.

## Pending in the final pass

The official run, `bench/bench.sh`, then `bench/assemble.sh`, in the configuration above:

- every file's tables with 10 rounds of 300 ms samples and confidence intervals;
- latency percentiles, GC work and 18-way throughput for every library;
- the 200,000-file soak with its heap profile diff;
- the environment, recorded before and after.
