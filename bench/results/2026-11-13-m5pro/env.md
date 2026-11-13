Command: `./bench.sh ` with COUNT=10 TIME=300ms MAXLOAD=4.5 SOAK_FILES=200000, the benchmark as `flock /tmp/jotadrilo-bench.lock taskpolicy -t 0 -l 0 run ...`.

Before:

| | |
|---|---|
| Mac model | Mac17,9 |
| Chip | Apple M5 Pro |
| Core level 0 | Super: 6 cores |
| Core level 1 | Performance: 12 cores |
| Memory | 48 GiB |
| macOS | 26.3.2 (25D2150) |
| Power source | AC Power |
| Low power mode (pmset powermode) | 0 |
| Load average (1, 5, 15 min) |  1.65 1.61 1.90  |
| Go (bench module) | go1.27.0 |
| C compiler | Apple clang version 21.0.0 (clang-2100.1.1.101) |
| libexif | 0.6.26 |
| exiv2 | 0.28.9 |
| exiftool | 13.55 |
| github.com/rwcarlsen/goexif | v0.0.0-20190401172101-9e8deecbddbd |
| github.com/dsoprea/go-exif/v3 | v3.0.1 |
| github.com/evanoberholster/imagemeta | v1.1.0 |
| github.com/bep/imagemeta | v1.1.1 |
| jotadrilo/exif | 7150ea3 |

After:

| | |
|---|---|
| Mac model | Mac17,9 |
| Chip | Apple M5 Pro |
| Core level 0 | Super: 6 cores |
| Core level 1 | Performance: 12 cores |
| Memory | 48 GiB |
| macOS | 26.3.2 (25D2150) |
| Power source | AC Power |
| Low power mode (pmset powermode) | 0 |
| Load average (1, 5, 15 min) |  7.81 5.28 4.29  |
| Go (bench module) | go1.27.0 |
| C compiler | Apple clang version 21.0.0 (clang-2100.1.1.101) |
| libexif | 0.6.26 |
| exiv2 | 0.28.9 |
| exiftool | 13.55 |
| github.com/rwcarlsen/goexif | v0.0.0-20190401172101-9e8deecbddbd |
| github.com/dsoprea/go-exif/v3 | v3.0.1 |
| github.com/evanoberholster/imagemeta | v1.1.0 |
| github.com/bep/imagemeta | v1.1.1 |
| jotadrilo/exif | 7150ea3 |
