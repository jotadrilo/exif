- Command: `bench/results/latest/bin/run -corpus bench/results/corpus -native native/bin/darwin -workers 18 -count 10 -time 300ms -maxload 4.5 -raw bench/results/latest/raw/bench.txt`
- Platform: darwin/arm64, Apple M5 Pro, 18 CPUs, GOMAXPROCS at its default (18); every library at its defaults. Throughput runs 18 concurrent parses: goroutines for Go, threads in one process for the native harnesses.
- Go go1.27.0. 10 interleaved rounds, each sample at least 300ms, after one warm-up round; 3 rounds redone for load above 4.5.
- 1-minute load average during the kept rounds: median 3.93, max 6.14.
- Latency percentiles time each parse alone and subtract the clock's own cost; the clock ticks every 41.7 ns on Apple silicon (24 MHz), which bounds the resolution of the fastest rows.
- Cells: median ± the 95% confidence interval of the median (order statistics), and the ratio to libexif and exiv2 medians from the same run (below 1 is faster). Peak RSS is the median after 50 parses from the file path.

#### camera.jpg (4.8 MiB)

| Library | orientation only | × libexif | × exiv2 | orientation + GPS + date | × libexif | × exiv2 | allocs/op | B/op | peak RSS | throughput, 18 concurrent | agrees with exiftool |
|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|---|
| baseline: open and close the file | | | | | | | | | 8.2 MiB | | |
| jotadrilo/exif Parse | 27 ns ±1% | 0.002 | 0.001 | 133 ns ±2% | 0.008 | 0.003 | 0 | 0 B | 8.8 MiB | 106.54 M/s ±2% | 5/5 |
| jotadrilo/exif Decode | 620 ns ±4% | 0.036 | 0.015 | 713 ns ±4% | 0.042 | 0.016 | 3.0 | 10.8 KiB | 8.8 MiB | 1.64 M/s ±2% | 5/5 |
| rwcarlsen/goexif | 15.14 µs ±2% | 0.89 | 0.38 | 15.24 µs ±1% | 0.89 | 0.34 | 513 | 129.4 KiB | 13.5 MiB | 85.0 k/s ±2% | 5/5 |
| dsoprea/go-exif v3 | 598.42 µs ±5% | 35.18 | 14.86 | 596.38 µs ±4% | 35.02 | 13.16 | 597 | 15.2 MiB | 36.8 MiB | 5.1 k/s ±1% | 4/5 (alt -12) |
| evanoberholster/imagemeta | 1.52 µs ±1% | 0.089 | 0.038 | 1.60 µs ±2% | 0.094 | 0.035 | 10.0 | 393 B | 8.7 MiB | 4.58 M/s ±2% | 5/5 |
| bep/imagemeta | 3.25 µs ±2% | 0.19 | 0.081 | 7.35 µs ±2% | 0.43 | 0.16 | 110 | 12.1 KiB | 9.0 MiB | 445.4 k/s ±1% | 5/5 |
| libexif (C) | 17.01 µs ±3% | 1.00 | 0.42 | 17.03 µs ±2% | 1.00 | 0.38 | n/a | n/a | 5.9 MiB | 629.8 k/s ±7% | 5/5 |
| exiv2 (C++) | 40.28 µs ±3% | 2.37 | 1.00 | 45.33 µs ±4% | 2.66 | 1.00 | n/a | n/a | 7.8 MiB | 21.4 k/s ±1% | 5/5 |

Latency of each parse alone (orientation + GPS + date), and the Go garbage collector's work per 1,000 parses:

| Library | p50 | p95 | p99 | GC cycles / 1k parses | GC pause / 1k parses |
|---|--:|--:|--:|--:|--:|
| jotadrilo/exif Parse | 125 ns | 167 ns | 167 ns | 0.00 | 0 ns |
| jotadrilo/exif Decode | 500 ns | 875 ns | 2.50 µs | 1.65 | 53.17 µs |
| rwcarlsen/goexif | 11.56 µs | 24.29 µs | 76.54 µs | 23.29 | 768.77 µs |
| dsoprea/go-exif v3 | 471.92 µs | 634.40 µs | 747.90 µs | 1491.76 | 38.23 ms |
| evanoberholster/imagemeta | 1.58 µs | 1.71 µs | 2.00 µs | 0.07 | 1.69 µs |
| bep/imagemeta | 7.08 µs | 7.71 µs | 12.63 µs | 2.22 | 69.73 µs |
| libexif (C) | 17.00 µs | 18.00 µs | 19.00 µs | n/a | n/a |
| exiv2 (C++) | 45.12 µs | 48.31 µs | 55.21 µs | n/a | n/a |

#### camera.png (1.1 MiB)

| Library | orientation only | × libexif | × exiv2 | orientation + GPS + date | × libexif | × exiv2 | allocs/op | B/op | peak RSS | throughput, 18 concurrent | agrees with exiftool |
|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|---|
| baseline: open and close the file | | | | | | | | | 8.2 MiB | | |
| jotadrilo/exif Parse | 52 ns ±3% | n/a | 0.002 | 161 ns ±1% | n/a | 0.005 | 0 | 0 B | 8.3 MiB | 87.83 M/s ±2% | 5/5 |
| jotadrilo/exif Decode | 232 ns ±1% | n/a | 0.008 | 340 ns ±2% | n/a | 0.010 | 3.0 | 848 B | 8.3 MiB | 6.24 M/s ±1% | 5/5 |
| rwcarlsen/goexif | 12.43 µs ±2% † | n/a | n/a | 12.39 µs ±6% † | n/a | n/a | 139 | 93.0 KiB | 12.6 MiB | 61.7 k/s ±8% | fails: exif: failed to find exif intro marker |
| dsoprea/go-exif v3 | 281.67 µs ±6% | n/a | 9.43 | 281.23 µs ±5% | n/a | 8.32 | 504 | 4.0 MiB | 17.7 MiB | 11.1 k/s ±1% | 4/5 (alt -12) |
| evanoberholster/imagemeta | 1.28 µs ±1% | n/a | 0.043 | 1.36 µs ±1% | n/a | 0.040 | 7.0 | 137 B | 8.5 MiB | 5.66 M/s ±1% | 5/5 |
| bep/imagemeta | 2.73 µs ±1% | n/a | 0.091 | 7.49 µs ±2% | n/a | 0.22 | 108 | 11.8 KiB | 8.9 MiB | 357.3 k/s ±2% | 5/5 |
| libexif (C) | 602 ns ±2% † | n/a | n/a | 600 ns ±1% † | n/a | n/a | n/a | n/a | 5.8 MiB | 916.0 k/s ±2% | fails: no EXIF found |
| exiv2 (C++) | 29.87 µs ±2% | n/a | 1.00 | 33.82 µs ±1% | n/a | 1.00 | n/a | n/a | 7.8 MiB | 24.5 k/s ±5% | 5/5 |

Latency of each parse alone (orientation + GPS + date), and the Go garbage collector's work per 1,000 parses:

| Library | p50 | p95 | p99 | GC cycles / 1k parses | GC pause / 1k parses |
|---|--:|--:|--:|--:|--:|
| jotadrilo/exif Parse | 167 ns | 209 ns | 209 ns | 0.00 | 0 ns |
| jotadrilo/exif Decode | 292 ns | 375 ns | 625 ns | 0.40 | 12.99 µs |
| rwcarlsen/goexif † | 6.71 µs | 36.96 µs | 63.38 µs | 42.48 | 1.41 ms |
| dsoprea/go-exif v3 | 267.52 µs | 318.90 µs | 346.48 µs | 1022.66 | 35.45 ms |
| evanoberholster/imagemeta | 1.33 µs | 1.42 µs | 1.62 µs | 0.06 | 1.75 µs |
| bep/imagemeta | 6.48 µs | 7.56 µs | 22.62 µs | 6.33 | 202.76 µs |
| libexif (C) † | 1.00 µs | 1.00 µs | 1.00 µs | n/a | n/a |
| exiv2 (C++) | 33.85 µs | 35.83 µs | 41.62 µs | n/a | n/a |

#### camera.tif (2.3 MiB)

| Library | orientation only | × libexif | × exiv2 | orientation + GPS + date | × libexif | × exiv2 | allocs/op | B/op | peak RSS | throughput, 18 concurrent | agrees with exiftool |
|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|---|
| baseline: open and close the file | | | | | | | | | 8.2 MiB | | |
| jotadrilo/exif Parse | 33 ns ±3% | n/a | 0.001 | 153 ns ±1% | n/a | 0.003 | 0 | 0 B | 22.5 MiB | 90.13 M/s ±3% | 5/5 |
| jotadrilo/exif Decode | 628.01 µs ±3% | n/a | 14.98 | 627.52 µs ±4% | n/a | 13.39 | 28.4 | 12.7 MiB | 21.7 MiB | 5.2 k/s ±2% | 5/5 |
| rwcarlsen/goexif | 693.01 µs ±3% | n/a | 16.53 | 691.38 µs ±4% | n/a | 14.75 | 3483 | 17.6 MiB | 29.0 MiB | 4.6 k/s ±1% | 5/5 |
| dsoprea/go-exif v3 | 346.55 µs ±5% | n/a | 8.27 | 359.26 µs ±7% | n/a | 7.66 | 607 | 7.0 MiB | 22.9 MiB | 8.9 k/s ±1% | 4/5 (alt -12) |
| evanoberholster/imagemeta | 1.38 µs ±1% | n/a | 0.033 | 1.46 µs ±1% | n/a | 0.031 | 7.0 | 136 B | 8.5 MiB | 5.80 M/s ±1% | 5/5 |
| bep/imagemeta | 3.56 µs ±3% | n/a | 0.085 | 60.31 µs ±2% | n/a | 1.29 | 206 | 84.3 KiB | 12.6 MiB | 61.5 k/s ±2% | 5/5 |
| libexif (C) | 596 ns ±1% † | n/a | n/a | 596 ns ±5% † | n/a | n/a | n/a | n/a | 5.8 MiB | 921.3 k/s ±2% | fails: no EXIF found |
| exiv2 (C++) | 41.92 µs ±1% | n/a | 1.00 | 46.87 µs ±2% | n/a | 1.00 | n/a | n/a | 8.0 MiB | 21.5 k/s ±2% | 5/5 |

Latency of each parse alone (orientation + GPS + date), and the Go garbage collector's work per 1,000 parses:

| Library | p50 | p95 | p99 | GC cycles / 1k parses | GC pause / 1k parses |
|---|--:|--:|--:|--:|--:|
| jotadrilo/exif Parse | 167 ns | 167 ns | 209 ns | 0.00 | 0 ns |
| jotadrilo/exif Decode | 546.19 µs | 675.23 µs | 772.73 µs | 2449.76 | 84.90 ms |
| rwcarlsen/goexif | 669.96 µs | 822.77 µs | 888.94 µs | 1905.15 | 74.23 ms |
| dsoprea/go-exif v3 | 359.46 µs | 517.27 µs | 571.21 µs | 1034.89 | 31.30 ms |
| evanoberholster/imagemeta | 1.46 µs | 1.52 µs | 1.75 µs | 0.04 | 1.06 µs |
| bep/imagemeta | 56.96 µs | 64.69 µs | 176.10 µs | 27.76 | 873.90 µs |
| libexif (C) † | 1.00 µs | 1.00 µs | 1.00 µs | n/a | n/a |
| exiv2 (C++) | 47.06 µs | 50.17 µs | 58.15 µs | n/a | n/a |

#### camera.webp (258.3 KiB)

| Library | orientation only | × libexif | × exiv2 | orientation + GPS + date | × libexif | × exiv2 | allocs/op | B/op | peak RSS | throughput, 18 concurrent | agrees with exiftool |
|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|---|
| baseline: open and close the file | | | | | | | | | 8.2 MiB | | |
| jotadrilo/exif Parse | 35 ns ±1% | n/a | 0.001 | 140 ns ±1% | n/a | 0.003 | 0.0 | 0 B | 8.3 MiB | 100.08 M/s ±3% | 5/5 |
| jotadrilo/exif Decode | 173 ns ±1% | n/a | 0.005 | 280 ns ±1% | n/a | 0.007 | 3.0 | 848 B | 8.3 MiB | 5.45 M/s ±3% | 5/5 |
| rwcarlsen/goexif | 28.76 µs ±1% † | n/a | n/a | 28.87 µs ±1% † | n/a | n/a | 377 | 231.7 KiB | 14.2 MiB | 16.7 k/s ±1% | fails: exif: failed to find exif intro marker |
| dsoprea/go-exif v3 | 1.74 ms ±2% | n/a | 46.87 | 1.76 ms ±2% | n/a | 42.67 | 483 | 48.8 KiB | 12.1 MiB | 5.0 k/s ±2% | 4/5 (alt -12) |
| evanoberholster/imagemeta | 641.83 µs ±2% | n/a | 17.31 | 636.42 µs ±3% | n/a | 15.40 | 7.0 | 167 B | 8.5 MiB | 23.8 k/s ±3% | 5/5 |
| bep/imagemeta | 2.67 µs ±2% | n/a | 0.072 | 7.14 µs ±1% | n/a | 0.17 | 109 | 11.8 KiB | 8.9 MiB | 225.6 k/s ±6% | 5/5 |
| libexif (C) | 596 ns ±1% † | n/a | n/a | 602 ns ±2% † | n/a | n/a | n/a | n/a | 5.8 MiB | 920.9 k/s ±2% | fails: no EXIF found |
| exiv2 (C++) | 37.07 µs ±2% | n/a | 1.00 | 41.31 µs ±2% | n/a | 1.00 | n/a | n/a | 8.1 MiB | 21.6 k/s ±3% | 5/5 |

Latency of each parse alone (orientation + GPS + date), and the Go garbage collector's work per 1,000 parses:

| Library | p50 | p95 | p99 | GC cycles / 1k parses | GC pause / 1k parses |
|---|--:|--:|--:|--:|--:|
| jotadrilo/exif Parse | 166 ns | 167 ns | 167 ns | 0.00 | 0 ns |
| jotadrilo/exif Decode | 250 ns | 334 ns | 688 ns | 0.29 | 9.07 µs |
| rwcarlsen/goexif † | 15.79 µs | 79.40 µs | 102.63 µs | 103.75 | 3.45 ms |
| dsoprea/go-exif v3 | 1.75 ms | 1.84 ms | 1.91 ms | 17.12 | 612.13 µs |
| evanoberholster/imagemeta | 630.06 µs | 677.27 µs | 708.58 µs | 0.00 | 0 ns |
| bep/imagemeta | 6.46 µs | 8.65 µs | 40.08 µs | 4.30 | 135.31 µs |
| libexif (C) † | 1.00 µs | 1.00 µs | 1.00 µs | n/a | n/a |
| exiv2 (C++) | 41.48 µs | 43.46 µs | 50.56 µs | n/a | n/a |

#### huge.jpg (47.1 MiB)

| Library | orientation only | × libexif | × exiv2 | orientation + GPS + date | × libexif | × exiv2 | allocs/op | B/op | peak RSS | throughput, 18 concurrent | agrees with exiftool |
|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|---|
| baseline: open and close the file | | | | | | | | | 8.2 MiB | | |
| jotadrilo/exif Parse | 27 ns ±0% | 0.002 | 0.001 | 134 ns ±1% | 0.009 | 0.003 | 0 | 0 B | 8.3 MiB | 105.24 M/s ±3% | 5/5 |
| jotadrilo/exif Decode | 121 ns ±1% | 0.009 | 0.003 | 230 ns ±1% | 0.016 | 0.006 | 3.0 | 848 B | 8.3 MiB | 13.92 M/s ±2% | 5/5 |
| rwcarlsen/goexif | 9.96 µs ±2% | 0.71 | 0.28 | 10.04 µs ±1% | 0.71 | 0.26 | 425 | 48.2 KiB | 11.3 MiB | 242.3 k/s ±2% | 5/5 |
| dsoprea/go-exif v3 | 3.28 ms ±4% | 232.88 | 93.53 | 3.36 ms ±3% | 237.99 | 85.84 | 513 | 156.8 MiB | 263.3 MiB | 518 /s ±2% | 4/5 (alt -12) |
| evanoberholster/imagemeta | 1.24 µs ±0% | 0.088 | 0.035 | 1.33 µs ±1% | 0.094 | 0.034 | 9.0 | 200 B | 8.5 MiB | 8.35 M/s ±2% | 5/5 |
| bep/imagemeta | 2.62 µs ±1% | 0.19 | 0.075 | 6.66 µs ±1% | 0.47 | 0.17 | 108 | 11.8 KiB | 8.9 MiB | 938.9 k/s ±5% | 5/5 |
| libexif (C) | 14.10 µs ±1% | 1.00 | 0.40 | 14.13 µs ±1% | 1.00 | 0.36 | n/a | n/a | 5.9 MiB | 741.3 k/s ±8% | 5/5 |
| exiv2 (C++) | 35.11 µs ±1% | 2.49 | 1.00 | 39.17 µs ±2% | 2.77 | 1.00 | n/a | n/a | 7.8 MiB | 22.0 k/s ±2% | 5/5 |

Latency of each parse alone (orientation + GPS + date), and the Go garbage collector's work per 1,000 parses:

| Library | p50 | p95 | p99 | GC cycles / 1k parses | GC pause / 1k parses |
|---|--:|--:|--:|--:|--:|
| jotadrilo/exif Parse | 125 ns | 167 ns | 167 ns | 0.00 | 0 ns |
| jotadrilo/exif Decode | 209 ns | 334 ns | 980 ns | 0.02 | 369 ns |
| rwcarlsen/goexif | 9.77 µs | 12.67 µs | 15.92 µs | 1.04 | 38.01 µs |
| dsoprea/go-exif v3 | 3.32 ms | 3.65 ms | 4.54 ms | 1431.27 | 56.05 ms |
| evanoberholster/imagemeta | 1.29 µs | 1.38 µs | 2.12 µs | 0.00 | 137 ns |
| bep/imagemeta | 6.54 µs | 7.56 µs | 10.02 µs | 0.25 | 9.82 µs |
| libexif (C) | 14.00 µs | 15.00 µs | 15.00 µs | n/a | n/a |
| exiv2 (C++) | 39.00 µs | 41.71 µs | 47.52 µs | n/a | n/a |

#### phone.jpg (4.8 MiB)

| Library | orientation only | × libexif | × exiv2 | orientation + GPS + date | × libexif | × exiv2 | allocs/op | B/op | peak RSS | throughput, 18 concurrent | agrees with exiftool |
|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|---|
| baseline: open and close the file | | | | | | | | | 8.2 MiB | | |
| jotadrilo/exif Parse | 27 ns ±1% | 0.002 | 0.003 | 117 ns ±1% | 0.009 | 0.010 | 0 | 0 B | 8.3 MiB | 119.55 M/s ±3% | 5/5 |
| jotadrilo/exif Decode | 115 ns ±2% | 0.009 | 0.013 | 206 ns ±1% | 0.017 | 0.017 | 3.0 | 528 B | 8.3 MiB | 12.38 M/s ±1% | 5/5 |
| rwcarlsen/goexif | 5.24 µs ±1% | 0.42 | 0.57 | 5.34 µs ±2% | 0.43 | 0.44 | 260 | 24.4 KiB | 9.6 MiB | 290.8 k/s ±1% | 5/5 |
| dsoprea/go-exif v3 | 550.36 µs ±10% | 44.09 | 59.86 | 540.59 µs ±7% | 43.40 | 44.62 | 358 | 15.2 MiB | 36.6 MiB | 5.2 k/s ±2% | 5/5 |
| evanoberholster/imagemeta | 811 ns ±1% | 0.065 | 0.088 | 896 ns ±1% | 0.072 | 0.074 | 5.0 | 152 B | 8.5 MiB | 8.89 M/s ±2% | 5/5 |
| bep/imagemeta | 1.82 µs ±1% | 0.15 | 0.20 | 3.96 µs ±2% | 0.32 | 0.33 | 80.0 | 6.6 KiB | 8.7 MiB | 731.0 k/s ±1% | 5/5 |
| libexif (C) | 12.48 µs ±1% | 1.00 | 1.36 | 12.46 µs ±1% | 1.00 | 1.03 | n/a | n/a | 5.8 MiB | 872.2 k/s ±8% | 5/5 |
| exiv2 (C++) | 9.19 µs ±3% | 0.74 | 1.00 | 12.11 µs ±1% | 0.97 | 1.00 | n/a | n/a | 7.4 MiB | 484.2 k/s ±11% | 5/5 |

Latency of each parse alone (orientation + GPS + date), and the Go garbage collector's work per 1,000 parses:

| Library | p50 | p95 | p99 | GC cycles / 1k parses | GC pause / 1k parses |
|---|--:|--:|--:|--:|--:|
| jotadrilo/exif Parse | 125 ns | 167 ns | 167 ns | 0.00 | 0 ns |
| jotadrilo/exif Decode | 208 ns | 250 ns | 375 ns | 0.09 | 2.82 µs |
| rwcarlsen/goexif | 4.62 µs | 6.52 µs | 18.40 µs | 4.36 | 139.65 µs |
| dsoprea/go-exif v3 | 448.73 µs | 619.48 µs | 743.96 µs | 1364.27 | 39.85 ms |
| evanoberholster/imagemeta | 916 ns | 958 ns | 1.12 µs | 0.03 | 553 ns |
| bep/imagemeta | 3.75 µs | 4.19 µs | 6.77 µs | 1.23 | 40.11 µs |
| libexif (C) | 13.00 µs | 13.00 µs | 13.00 µs | n/a | n/a |
| exiv2 (C++) | 12.23 µs | 13.21 µs | 14.15 µs | n/a | n/a |

#### plain.jpg (4.8 MiB)

| Library | orientation only | × libexif | × exiv2 | orientation + GPS + date | × libexif | × exiv2 | allocs/op | B/op | peak RSS | throughput, 18 concurrent | agrees with exiftool |
|---|--:|--:|--:|--:|--:|--:|--:|--:|--:|--:|---|
| baseline: open and close the file | | | | | | | | | 8.2 MiB | | |
| jotadrilo/exif Parse | 9 ns ±1% | 0.016 | 0.026 | 9 ns ±1% | 0.016 | 0.009 | 0 | 0 B | 8.2 MiB | 1246.23 M/s ±3% | 3/3 |
| jotadrilo/exif Decode | 105 ns ±2% | 0.18 | 0.29 | 105 ns ±2% | 0.17 | 0.10 | 2.0 | 144 B | 8.2 MiB | 33.77 M/s ±2% | 3/3 |
| rwcarlsen/goexif | 788.28 µs ±1% | 1323.95 | 2153.48 | 783.57 µs ±1% | 1287.92 | 751.52 | 11106 | 5.1 MiB | 15.5 MiB | 1.7 k/s ±2% | 3/3 |
| dsoprea/go-exif v3 | 32.38 ms ±4% | 54381.27 | 88454.06 | 32.80 ms ±1% | 53913.85 | 31459.44 | 3 | 4.1 KiB | 8.6 MiB | 429 /s ±2% | 3/3 |
| evanoberholster/imagemeta | 274 ns ±2% | 0.46 | 0.75 | 295 ns ±1% | 0.49 | 0.28 | 2.0 | 64 B | 8.4 MiB | 27.79 M/s ±3% | 3/3 |
| bep/imagemeta | 337 ns ±0% | 0.57 | 0.92 | 361 ns ±1% | 0.59 | 0.35 | 15.0 | 736 B | 8.3 MiB | 6.89 M/s ±1% | 3/3 |
| libexif (C) | 595 ns ±1% | 1.00 | 1.63 | 608 ns ±5% | 1.00 | 0.58 | n/a | n/a | 5.8 MiB | 913.2 k/s ±2% | 3/3 |
| exiv2 (C++) | 366 ns ±3% | 0.61 | 1.00 | 1.04 µs ±3% | 1.71 | 1.00 | n/a | n/a | 7.0 MiB | 6.27 M/s ±18% | 3/3 |

Latency of each parse alone (orientation + GPS + date), and the Go garbage collector's work per 1,000 parses:

| Library | p50 | p95 | p99 | GC cycles / 1k parses | GC pause / 1k parses |
|---|--:|--:|--:|--:|--:|
| jotadrilo/exif Decode | 125 ns | 125 ns | 208 ns | 0.03 | 805 ns |
| rwcarlsen/goexif | 788.92 µs | 834.00 µs | 891.21 µs | 974.97 | 31.65 ms |
| dsoprea/go-exif v3 | 32.69 ms | 33.97 ms | 33.97 ms | 0.00 | 0 ns |
| evanoberholster/imagemeta | 292 ns | 334 ns | 375 ns | 0.01 | 292 ns |
| bep/imagemeta | 334 ns | 438 ns | 625 ns | 0.13 | 4.11 µs |
| libexif (C) | 1.00 µs | 1.00 µs | 1.00 µs | n/a | n/a |
| exiv2 (C++) | 1.04 µs | 1.17 µs | 1.23 µs | n/a | n/a |

† The library cannot read this file: the time is how long it takes to give up, and no ratio is given.

#### Agreement with exiftool, all files

| Library | fields that agree |
|---|--:|
| jotadrilo/exif Parse | 33/33 |
| jotadrilo/exif Decode | 33/33 |
| rwcarlsen/goexif | 23/33 |
| dsoprea/go-exif v3 | 28/33 |
| evanoberholster/imagemeta | 33/33 |
| bep/imagemeta | 33/33 |
| libexif (C) | 18/33 |
| exiv2 (C++) | 33/33 |
