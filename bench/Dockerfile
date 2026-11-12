# The Linux benchmark environment: the same Go as the host's bench module, libexif, exiv2 and
# exiftool from Debian. bench.sh mounts the repository, the corpus and the module cache.
FROM golang:1.27.0-trixie
RUN apt-get update \
	&& apt-get install -y --no-install-recommends libexif-dev libexiv2-dev libimage-exiftool-perl pkg-config g++ \
	&& rm -rf /var/lib/apt/lists/*
ENV GOFLAGS=-mod=mod GOTOOLCHAIN=local
WORKDIR /src/bench
