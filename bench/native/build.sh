#!/bin/sh
# build.sh [DIR]: builds the native harnesses into DIR (default native/bin) with pkg-config's flags.
set -eu
cd "$(dirname "$0")"
bin=${1:-bin}
mkdir -p "$bin"
cc -O2 -pthread -o "$bin/libexif_bench" libexif_bench.c $(pkg-config --cflags --libs libexif)
c++ -O2 -std=c++17 -pthread -o "$bin/exiv2_bench" exiv2_bench.cpp $(pkg-config --cflags --libs exiv2)
