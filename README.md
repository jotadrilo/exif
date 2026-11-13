# exif

[![CI](https://github.com/jotadrilo/exif/actions/workflows/ci.yml/badge.svg)](https://github.com/jotadrilo/exif/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/jotadrilo/exif.svg)](https://pkg.go.dev/github.com/jotadrilo/exif)

A fast, hardened EXIF reader for Go, built for untrusted uploads, plus a JPEG metadata
stripper. Pure Go, standard library only, works with `CGO_ENABLED=0`.

```sh
go get github.com/jotadrilo/exif
```

Go 1.27.4 or later.

## What it does

- Reads EXIF from **JPEG** (APP1 `Exif\0\0`), **TIFF**, **PNG** (`eXIf` chunk) and **WebP**
  (`EXIF` chunk of an extended, VP8X file), in either byte order.
- Reads **IFD0, the Exif IFD, the GPS IFD, the Interop IFD and IFD1**, and every TIFF type:
  BYTE, ASCII, SHORT, LONG, RATIONAL, SBYTE, UNDEFINED, SSHORT, SLONG, SRATIONAL, FLOAT, DOUBLE,
  and the IFD/BigTIFF integer types.
- Gives typed answers to the common questions: `Orientation`, `GPS`, `DateTimeOriginal`, `Make`,
  `Model` and `Dimensions`. Any other tag is available by IFD and ID, with typed accessors.
- **Strips** metadata from a JPEG with `Strip`. It copies the image bytes unchanged and can
  keep the orientation.

## What it does not do

- No HEIC/HEIF or AVIF yet. Their EXIF sits in an ISO-BMFF item and could be added later
  (`ErrUnknownFormat` for now). The same goes for JPEG XL, camera RAW formats and BigTIFF
  containers.
- No writing or editing of EXIF, no maker notes (`MakerNote` is returned as raw bytes), and no
  XMP or IPTC.
- `Strip` handles JPEG only.

## Usage

### Orientation

```go
x, _ := exif.Parse(data) // data is the uploaded file
switch x.Orientation() {  // 1–8; 1 when there is no orientation, or none that can be read
case 6:
	// rotate 90° clockwise to show it upright
}
```

A failed `Parse` returns a zero `Exif`, whose `Orientation` is 1. That is why the error can be
ignored when the orientation is all you need. Orientations 5 to 8 swap width and height.

### GPS

```go
x, err := exif.Parse(data)
if err != nil {
	return // exif.ErrNoExif, exif.ErrUnknownFormat, exif.ErrMalformed, ...
}
if g, ok := x.GPS(); ok {
	fmt.Println(g.Latitude, g.Longitude) // decimal degrees; south and west are negative
	if g.HasAltitude {
		fmt.Println(g.Altitude) // meters; below sea level is negative
	}
}
```

`GPS` reports `ok == false` unless the latitude, the longitude and both references are present
and readable. It also rejects any value that cannot be a real fix:

- a zero denominator;
- a negative component;
- a value out of range;
- `GPSStatus` "V" (measurement void);
- exactly (0, 0), the "Null Island" placeholder that devices write when they have no fix.

### Date taken, camera and any other tag

```go
if t, hasOffset, ok := x.DateTimeOriginal(); ok {
	// With OffsetTimeOriginal, t carries that zone. Without it, t is the camera's
	// wall-clock time, in UTC.
}
fmt.Println(x.Make(), x.Model())

if tag, ok := x.Tag(exif.ExifIFD, exif.TagFNumber); ok {
	f, err := tag.Float(0) // Int, Rat, Float, Text and Bytes are the typed accessors
}
for tag := range x.Tags() { // every tag of every IFD
	_ = tag
}
```

### Streams and files

`Parse` takes the file already in memory. `Decode` takes an `io.Reader` and reads only the
metadata:

- for a JPEG, it stops after the EXIF segment;
- for a PNG or a WebP, it skips the chunks before the metadata, using `Seek` when the reader
  supports it, so a large photo costs a few kilobytes of reads;
- a TIFF is read whole, up to `Decoder.MaxTIFF`.

```go
f, _ := os.Open(path)
defer f.Close()
x, err := exif.Decode(f)
```

`Decoder` sets the limits: `MaxMetadata` (4 MiB), `MaxTIFF` (64 MiB) and `MaxScan` (1 GiB).

### Strip

```go
err := exif.Strip(dst, src, exif.KeepOrientation())
```

`Strip` drops every APP segment (JFIF, EXIF, XMP, ICC, Photoshop, MPF and maker data), comments,
and anything after the end of the image. It copies frames, tables and scans byte for byte. It
keeps the Adobe APP14 segment, because decoders need it to get the colors of CMYK and YCCK
images right.

**Orientation is metadata too.** A plain `Strip` makes a rotated phone photo show sideways.
Either:

- pass `KeepOrientation()`, which writes a minimal APP1 holding the orientation and nothing
  else; or
- turn the pixels upright first (using `Orientation`) and encode the image again.

Pass `KeepICCProfile()` to keep wide-gamut colors, such as Display P3. A profile may name the
device that made it.

`Strip` streams with about 192 KiB of pooled buffers, in time linear in the input. On error,
`dst` may already hold part of the output.

## Performance

The design goal is to do as little as possible:

- `Parse` does not allocate. The `Exif` it returns is a small value that points into your bytes.
- Nothing is decoded up front. `Orientation` reads one IFD0 entry, `GPS` reads one IFD0 pointer
  and the GPS IFD, and `DateTimeOriginal` reads the Exif IFD.
- `Orientation`, `GPS` and `DateTimeOriginal` do not allocate.
- There is no reflection and no maps. Errors are constants.

[BENCHMARKS.md](BENCHMARKS.md) compares this package with rwcarlsen/goexif, dsoprea/go-exif,
evanoberholster/imagemeta, bep/imagemeta, libexif (C) and exiv2 (C++), checking each against
exiftool:

- time, allocations, peak RSS, latency percentiles and GC work;
- throughput on 1, 2 and 4 CPUs.

On a 12-megapixel camera JPEG (Apple M5 Pro), reading the orientation takes 27 ns, and reading
the orientation, GPS and date together takes 133 ns, with no allocations. That is 0.008× libexif's
time and about 12× faster than the next Go reader.

## Hardening

The input is assumed hostile:

- Every read is bounds-checked, and offset arithmetic is done in 64 bits, so a value that points
  past the data is refused rather than read.
- Sizes are capped. A value is a sub-slice of the input and is never copied, so a huge declared
  count costs nothing. `Decoder` caps what it reads from a stream.
- Sub-IFD pointers that point back at IFD0, at their own directory, at the header or outside
  the data are ignored. Walking `Tags` visits each directory once, so loops and shared offsets
  terminate.
- Each lookup has a fixed depth (IFD0 → Exif → Interop, and IFD0 → IFD1), and time is linear in
  the input.
- A truncated directory yields the entries that fit. One broken tag does not hide the others.
- Errors are typed constants (`ErrUnknownFormat`, `ErrNoExif`, `ErrMalformed`, `ErrTooLarge`,
  `ErrType`, `ErrIndex`, `ErrZeroDenominator`) for `errors.Is`.

The tests that back this up:

- Table tests cover all eight orientations in every format and byte order, GPS in all four
  hemispheres and below sea level, and malformed files.
- Fixtures written by exiftool, an independent implementation, check the reading. The cases that
  unpixel-web and flora pinned with goexif are ported as tests.
- Five fuzz targets (JPEG, TIFF, PNG, WebP, Strip) compare `Parse` with `Decode`, check the fast
  GPS path against the generic accessors, and check that `Strip` is idempotent. Their seeds are
  in `testdata/fuzz`.
- A soak test in [`soak/`](soak) parses tens of thousands of valid and broken files, including
  concurrent bursts. It fails if the heap keeps growing (and current RSS, where Linux exposes it), and checks for leaked goroutines
  with goleak. The soak is its own module, so the library keeps no dependencies, not even
  test-only ones.

## License

BSD 3-Clause, see [LICENSE](LICENSE).

## No support promises

This is a personal project, shared as is. Issues and pull requests are welcome, but there is no
promise of answers, fixes, releases or API stability before a v1 tag.
