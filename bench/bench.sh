#!/bin/sh
# bench.sh [OUT]: the official benchmark, native on this Mac, into OUT (default
# bench/results/latest). One environment and one configuration, the same every time: no
# containers, GOMAXPROCS at its default (every CPU), every library at its defaults. It measures
# time, allocations, peak RSS, p50/p95/p99 latency, GC work, throughput with as many concurrent
# parses as CPUs (goroutines for Go, threads in one process for the native harnesses) and
# agreement with exiftool, then runs the long soak: SOAK_FILES (default 200,000) files with
# heap profiles at 10,000 and at the end, and their diff.
#
# The benchmark and the soak hold the machine-wide lock /tmp/jotadrilo-bench.lock, shared with
# the other jotadrilo benchmarks, and run under `taskpolicy -t 0 -l 0`: the highest throughput and
# latency tiers, with no QoS clamp, so macOS keeps it off the efficiency path. (taskpolicy can
# only clamp QoS down; it cannot raise a process to user-interactive.) The host's load average
# is sampled every 5 seconds; a round is redone when it rises above MAXLOAD (default: a
# quarter of the cores). env.md records the machine, power, versions and load before and
# after.
#
# Environment: COUNT (rounds, default 10), TIME (minimum per sample, default 300ms), MAXLOAD,
# SOAK_FILES, CORPUS (default bench/results/corpus, written by corpus/corpus.sh when absent).
set -eu
here=$(cd "$(dirname "$0")" && pwd)
out=$(mkdir -p "${1:-$here/results/latest}" && cd "${1:-$here/results/latest}" && pwd)
corpus=${CORPUS:-$here/results/corpus}
lock=/tmp/jotadrilo-bench.lock
cores=$(sysctl -n hw.ncpu)
maxload=${MAXLOAD:-$(echo "$cores / 4" | bc -l)}
count=${COUNT:-10}
sampletime=${TIME:-300ms}
soakfiles=${SOAK_FILES:-200000}
priority="taskpolicy -t 0 -l 0"
mkdir -p "$out/raw" "$out/soak"
[ -d "$corpus" ] || "$here/corpus/corpus.sh" "$corpus" large

(while :; do sysctl -n vm.loadavg > "$out/hostload.tmp" && mv "$out/hostload.tmp" "$out/hostload"; sleep 5; done) &
loadloop=$!
trap 'kill $loadloop 2>/dev/null || true; rm -f "$out/hostload" "$out/hostload.tmp"' EXIT

cd "$here"
"$here/native/build.sh" bin/darwin
go build -o "$out/bin/run" ./run
{
	echo "Command: \`$0 $*\` with COUNT=$count TIME=$sampletime MAXLOAD=$maxload SOAK_FILES=$soakfiles, the benchmark as \`flock $lock $priority run ...\`."
	echo
	echo "Before:"
	echo
	"$here/env.sh"
} > "$out/env.md"

workers=$(sysctl -n hw.ncpu)
echo "benchmark: $count rounds, $workers concurrent parses for throughput" >&2
env -u GOMAXPROCS EXIF_BENCH_LOADAVG_FILE="$out/hostload" flock "$lock" $priority "$out/bin/run" -corpus "$corpus" \
	-native native/bin/darwin -workers "$workers" -count "$count" -time "$sampletime" -maxload "$maxload" \
	-raw "$out/raw/bench.txt" > "$out/bench.md"

echo "soak: $soakfiles files" >&2
(cd "$here/../soak" && flock "$lock" $priority env -u GOMAXPROCS SOAK_FILES="$soakfiles" SOAK_PROFILES="$out/soak" \
	go test -count=1 -run Soak -v -timeout 60m ./... > "$out/soak/soak.txt")
for idx in inuse_objects inuse_space; do
	go tool pprof -top -sample_index=$idx -base "$out/soak/heap-10000.pprof" "$out/soak/heap-$soakfiles.pprof" \
		> "$out/soak/heap-diff-$idx.txt" 2>/dev/null || true
done

for f in "$out"/raw/*.txt; do
	benchstat -col /lib -row /file,/task "$f" > "${f%.txt}.benchstat" 2>/dev/null || true
done
{
	echo
	echo "After:"
	echo
	"$here/env.sh"
} >> "$out/env.md"
echo "done: $out; now: bench/assemble.sh $out \"<status line>\"" >&2
