#!/bin/sh
# bench.sh [OUT]: runs the whole benchmark into OUT (default bench/results/provisional).
#
#   1. macOS host, GOMAXPROCS=1, native harnesses from Homebrew.
#   2. A linux container per CPU count (1, 2, 4): --cpus=N on a fixed --cpuset of N cores,
#      GOMAXPROCS=N, latency per parse and throughput with N concurrent parses.
#   3. The scaling tables, combined from the container runs.
#   4. The long soak: SOAK_FILES (default 200,000) files with heap profiles at 10,000 and at
#      the end, and their diff.
#
# Every timed run holds the machine-wide lock /tmp/jotadrilo-bench.lock (shared with the other
# jotadrilo benchmarks), so no two benchmarks run on the machine at once. The host's load
# average is written to OUT/hostload every 5 seconds; the runner redoes a round when it rises
# above MAXLOAD (default: a quarter of the host's cores).
set -eu
here=$(cd "$(dirname "$0")" && pwd)
out=$(mkdir -p "${1:-$here/results/provisional}" && cd "${1:-$here/results/provisional}" && pwd)
corpus=${CORPUS:-$here/results/corpus}
lock=/tmp/jotadrilo-bench.lock
cores=$(sysctl -n hw.ncpu 2>/dev/null || nproc)
maxload=${MAXLOAD:-$(echo "$cores / 4" | bc -l)}
count=${COUNT:-10}
sampletime=${TIME:-300ms}
mkdir -p "$out/raw"
[ -d "$corpus" ] || "$here/corpus/corpus.sh" "$corpus" large

(while :; do (sysctl -n vm.loadavg 2>/dev/null || cat /proc/loadavg) > "$out/hostload.tmp" && mv "$out/hostload.tmp" "$out/hostload"; sleep 5; done) &
loadloop=$!
trap 'kill $loadloop 2>/dev/null || true' EXIT

cd "$here"
"$here/native/build.sh" bin/darwin
go build -o "$out/bin/run" ./run
echo "host run" >&2
EXIF_BENCH_LOADAVG_FILE="$out/hostload" flock "$lock" "$out/bin/run" -corpus "$corpus" -native native/bin/darwin \
	-procs 1 -count "$count" -time "$sampletime" -maxload "$maxload" -raw "$out/raw/darwin-1cpu.txt" -summary "$out/darwin-1cpu.json" > "$out/darwin-1cpu.md"

docker build -q -t jotadrilo-exif-bench "$here" >/dev/null
for n in 1 2 4; do
	echo "container run, $n CPU" >&2
	cpus=$(seq -s, 2 $((n + 1)))
	flock "$lock" docker run --rm --cpus="$n" --cpuset-cpus="$cpus" -e GOMAXPROCS="$n" \
		-e EXIF_BENCH_LOADAVG_FILE=/out/hostload \
		-v "$here/..:/src" -v "$corpus:/corpus:ro" -v "$out:/out" -v "$(go env GOMODCACHE):/go/pkg/mod" \
		jotadrilo-exif-bench sh -c "native/build.sh /tmp/native && go build -o /tmp/run ./run && \
			/tmp/run -corpus /corpus -native /tmp/native -procs $n -count $count -time $sampletime -maxload $maxload \
			-raw /out/raw/linux-${n}cpu.txt -summary /out/linux-${n}cpu.json > /out/linux-${n}cpu.md"
done
"$out/bin/run" -combine "$out/linux-1cpu.json,$out/linux-2cpu.json,$out/linux-4cpu.json" > "$out/scaling.md"
soakfiles=${SOAK_FILES:-200000}
mkdir -p "$out/soak"
echo "soak, $soakfiles files" >&2
(cd "$here/../soak" && flock "$lock" env SOAK_FILES="$soakfiles" SOAK_PROFILES="$out/soak" \
	go test -count=1 -run Soak -v -timeout 60m ./... > "$out/soak/soak.txt")
go tool pprof -top -sample_index=inuse_objects -base "$out/soak/heap-10000.pprof" "$out/soak/heap-$soakfiles.pprof" \
	> "$out/soak/heap-diff-objects.txt" 2>/dev/null || true
go tool pprof -top -sample_index=inuse_space -base "$out/soak/heap-10000.pprof" "$out/soak/heap-$soakfiles.pprof" \
	> "$out/soak/heap-diff-space.txt" 2>/dev/null || true
for f in "$out"/raw/*.txt; do
	benchstat -col /lib -row /file,/task "$f" > "${f%.txt}.benchstat" 2>/dev/null || true
done
rm -f "$out/hostload"
echo "done: $out" >&2
