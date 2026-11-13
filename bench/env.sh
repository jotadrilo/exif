#!/bin/sh
# env.sh: prints, as Markdown, the environment a benchmark ran in, so that a later run can be
# matched to it: the Mac, its cores, macOS, power, load, and every compiler and library version.
set -u
here=$(cd "$(dirname "$0")" && pwd)
v() { sysctl -n "$1" 2>/dev/null || echo "?"; }
echo "| | |"
echo "|---|---|"
echo "| Mac model | $(v hw.model) |"
echo "| Chip | $(v machdep.cpu.brand_string) |"
n=$(v hw.nperflevels)
i=0
while [ "$i" -lt "${n:-0}" ]; do
	echo "| Core level $i | $(v hw.perflevel$i.name): $(v hw.perflevel$i.physicalcpu) cores |"
	i=$((i + 1))
done
echo "| Memory | $(($(v hw.memsize) / 1073741824)) GiB |"
echo "| macOS | $(sw_vers -productVersion) ($(sw_vers -buildVersion)) |"
echo "| Power source | $(pmset -g batt | head -1 | sed "s/.*'\(.*\)'.*/\1/") |"
echo "| Low power mode (pmset powermode) | $(pmset -g | awk '/powermode/ {print $2; exit}') |"
echo "| Load average (1, 5, 15 min) | $(sysctl -n vm.loadavg | tr -d '{}') |"
echo "| Go (bench module) | $(cd "$here" && go version | awk '{print $3}') |"
echo "| C compiler | $(cc --version | head -1) |"
echo "| libexif | $(pkg-config --modversion libexif) |"
echo "| exiv2 | $(pkg-config --modversion exiv2) |"
echo "| exiftool | $(exiftool -ver) |"
(cd "$here" && go list -m -f '| {{.Path}} | {{.Version}} |' \
	github.com/rwcarlsen/goexif github.com/dsoprea/go-exif/v3 github.com/evanoberholster/imagemeta github.com/bep/imagemeta)
echo "| jotadrilo/exif | $(git -C "$here/.." rev-parse --short HEAD)$(git -C "$here/.." diff --quiet || echo ' (with uncommitted changes)') |"
