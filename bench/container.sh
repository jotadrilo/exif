#!/bin/sh
# container.sh [OUT]: an optional extra, never the published numbers. It runs the same
# benchmark once in a linux/arm64 container with every CPU of the Docker VM, under the
# machine-wide lock. Its results depend on the VM, so they are not comparable from one machine
# or Docker setup to another.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
out=$(mkdir -p "${1:-$here/results/container}" && cd "${1:-$here/results/container}" && pwd)
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
docker build -q -t jotadrilo-exif-bench "$here" >/dev/null
flock "$lock" docker run --rm -e EXIF_BENCH_LOADAVG_FILE=/out/hostload \
	-v "$here/..:/src" -v "$corpus:/corpus:ro" -v "$out:/out" -v "$(go env GOMODCACHE):/go/pkg/mod" \
	jotadrilo-exif-bench sh -c "native/build.sh /tmp/native && go build -o /tmp/run ./run && \
		/tmp/run -corpus /corpus -native /tmp/native -workers \$(nproc) -count $count -time $sampletime \
		-maxload $maxload -raw /out/raw/bench.txt > /out/bench.md"
