#!/bin/sh
# assemble.sh RESULTS STATUS: writes ../BENCHMARKS.md from a bench.sh results directory.
# STATUS is a line put at the top, such as "Provisional: ...".
set -eu
here=$(cd "$(dirname "$0")" && pwd)
r=$1
status=$2
rel=${r#"$here/"}
{
	cat <<HEAD
# Benchmarks

> $status

This package against the maintained (and the most used) EXIF readers for Go, and against the
native libraries other ecosystems use:

- [rwcarlsen/goexif](https://github.com/rwcarlsen/goexif), the commit pinned since 2019 by many projects;
- [dsoprea/go-exif](https://github.com/dsoprea/go-exif) v3;
- [evanoberholster/imagemeta](https://github.com/evanoberholster/imagemeta);
- [bep/imagemeta](https://github.com/bep/imagemeta), Hugo's reader;
- [libexif](https://libexif.github.io/) (C) and [exiv2](https://exiv2.org/) (C++), through small
  harnesses in \`bench/native\`;
- [exiftool](https://exiftool.org/) as the reference for what each one reads.

## Method

Everything is in [\`bench/\`](bench) and runs with \`bench/bench.sh\`:

- **Corpus.** \`bench/corpus/corpus.sh\` writes the corpus. exiftool, not this package, gives each
  file its metadata. The corpus holds:
  - a 12-megapixel camera JPEG (little-endian EXIF with a thumbnail, GPS south and east below
    sea level, and a time offset);
  - a phone JPEG (big-endian, north and west);
  - the same camera metadata in a PNG, a WebP and a TIFF;
  - a JPEG without EXIF;
  - a 50 MB JPEG with its EXIF at the start.
- **Tasks.**
  - Orientation only.
  - Orientation, GPS and DateTimeOriginal, each library through its documented API.
- **Isolation.**
  - Every sample runs in its own process, so peak RSS belongs to one library.
  - Every run holds the machine-wide lock \`/tmp/jotadrilo-bench.lock\`, so no other benchmark
    shares the machine.
  - The libraries are interleaved in rounds, and the order rotates every round.
  - A warm-up round comes first.
  - A round is redone when the host's 1-minute load average rises above a quarter of its cores.
- **Statistics.** Each cell is the median of 10 rounds, with the half-width of a
  distribution-free 95% confidence interval of the median (order statistics). Every row also
  gives its ratio to libexif and to exiv2, both measured in the same run. The raw samples are in
  benchstat format under \`$rel/raw\`, with benchstat's tables, including p-values, beside them.
- **Platforms.**
  - The macOS host, GOMAXPROCS=1.
  - A linux/arm64 container (Debian, the same Go) with \`--cpus=N --cpuset-cpus\` pinned to N
    cores and GOMAXPROCS=N, for N = 1, 2 and 4.
- **What each measurement means.**
  - *Time*: a parse from memory, as an upload handler holding the bytes would do.
  - *Peak RSS*: 50 parses from the file path, as each library documents for files. The baseline
    row only opens and closes the file.
  - *Latency*: each parse timed alone, as p50, p95 and p99.
  - *Throughput*: N concurrent parses on N CPUs.
- **Reading the values.** "Agrees with exiftool" checks orientation, latitude, longitude,
  altitude and DateTimeOriginal. A library that cannot read a format is marked †. Its time is how
  long it takes to give up, and it gets no ratio.

HEAD
	echo "## Scaling: 1, 2 and 4 CPUs (linux/arm64 container)"
	echo
	cat "$r/scaling.md"
	echo
	echo "## macOS host, 1 CPU"
	echo
	sed 's/^#### /### /' "$r/darwin-1cpu.md"
	for n in 1 2 4; do
		echo
		echo "## linux/arm64 container, $n CPU"
		echo
		sed 's/^#### /### /' "$r/linux-${n}cpu.md"
	done
	echo
	echo "## Memory over many files (soak)"
	echo
	echo "\`soak/\` parsed this many files in a loop with Parse, Decode, every accessor and Strip. The files are valid and broken, with a burst of 8 concurrent goroutines every 1,000 files. It sampled after a GC every 5,000 files:"
	echo
	echo '```'
	grep "files:" "$r/soak/soak.txt" | sed 's/^ *soak_test.go:[0-9]*: //' | awk 'NR==1 || NR%5==0'
	echo '```'
	echo
	echo "The heap in use stays flat. HeapObjects settles as the runtime's goroutine and pool caches fill, and the test fails if the last third of the samples rises past the middle third's peak. The in-use objects of the heap profile at 10,000 files, diffed against the last one ($rel/soak):"
	echo
	echo '```'
	sed -n '1,14p' "$r/soak/heap-diff-objects.txt"
	echo '```'
} > "$here/../BENCHMARKS.md"
