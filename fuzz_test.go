package exif_test

import (
	"bytes"
	"testing"

	"github.com/jotadrilo/exif"
)

// richTIFF holds every directory and a tag of every type, for seeds.
func richTIFF(o byteOrder) []byte {
	return buildTIFF(layout{
		order: o,
		ifd0: []entry{ascii(0x010F, "Maker"), ascii(0x0110, "Model"), short(o, 0x0112, 6),
			rational(o, 0x011A, 72, 1), {0x1005, 6, 2, []byte{0xFF, 0x7F}}, {0x100B, 12, 1, make([]byte, 8)}},
		exif: []entry{ascii(0x9003, "2024:07:14 18:30:05"), ascii(0x9011, "+02:00"), ascii(0x9291, "12"),
			long(o, 0xA002, 4000), long(o, 0xA003, 3000), {0x927C, 7, 6, []byte("maker!")}},
		gps: append(gpsEntries(o, "S", [3]uint32{33, 52, 0}, "W", [3]uint32{70, 1, 2}),
			byteTag(0x5, 1), rational(o, 0x6, 10, 1), ascii(0x9, "A")),
		ifd1: []entry{long(o, 0x0201, 8), long(o, 0x0202, 4)},
	})
}

// exercise runs every accessor on data, comparing Parse with Decode.
func exercise(t *testing.T, data []byte) {
	x, err := exif.Parse(data)
	y, errY := exif.Decode(onlyReader{bytes.NewReader(data)})
	if err == nil && errY == nil && !bytes.Equal(x.Raw(), y.Raw()) {
		t.Fatal("Parse and Decode found different EXIF data")
	}
	z, _ := exif.Decode(bytes.NewReader(data))
	for _, e := range []exif.Exif{x, y, z} {
		if o := e.Orientation(); o < 1 || o > 8 {
			t.Fatalf("orientation %d", o)
		}
		if g, ok := e.GPS(); ok && (g.Latitude < -90 || g.Latitude > 90 || g.Longitude < -180 || g.Longitude > 180) {
			t.Fatalf("GPS %+v", g)
		}
		_, _, _ = e.DateTimeOriginal()
		_, _ = e.Make(), e.Model()
		_, _, _ = e.Dimensions()
		n := 0
		for tag := range e.Tags() {
			n++
			_ = tag.Bytes()
			_ = tag.Valid()
			for i := range min(int(tag.Count), 4) {
				_, _ = tag.Int(i)
				_, _ = tag.Float(i)
				_, _, _ = tag.Rat(i)
			}
			_, _ = tag.Int(-1)
			_, _ = tag.Text()
		}
		if n > 5*65535 {
			t.Fatalf("%d tags", n)
		}
	}
}

func FuzzParseJPEG(f *testing.F) {
	jpg := testJPEG(f, 8, 8)
	f.Add(jpg)
	for _, o := range []byteOrder{le, be} {
		f.Add(withSegments(jpg, exifSegment(richTIFF(o))))
		f.Add(withSegments(jpg, segment(0xE0, []byte("JFIF\x00")), exifSegment(orientationTIFF(o, 8))))
	}
	f.Add([]byte("\xFF\xD8\xFF\xE1\x00\x10Exif\x00\x00II*\x00\x08\x00"))
	f.Fuzz(exercise)
}

func FuzzParseTIFF(f *testing.F) {
	for _, o := range []byteOrder{le, be} {
		f.Add(richTIFF(o))
		f.Add(orientationTIFF(o, 3))
	}
	f.Add([]byte("II*\x00\x08\x00\x00\x00\x01\x00\x69\x87\x04\x00\x01\x00\x00\x00\x08\x00\x00\x00\x08\x00\x00\x00"))
	f.Fuzz(exercise)
}

func FuzzParsePNG(f *testing.F) {
	f.Add(pngWithTIFF(f, nil))
	f.Add(pngWithTIFF(f, richTIFF(be)))
	f.Add(pngWithTIFF(f, append([]byte("Exif\x00\x00"), richTIFF(le)...)))
	f.Fuzz(exercise)
}

func FuzzParseWebP(f *testing.F) {
	f.Add(webpWithTIFF(nil))
	f.Add(webpWithTIFF(richTIFF(le)))
	f.Add(webpWithTIFF(append([]byte("Exif\x00\x00"), richTIFF(be)...)))
	f.Fuzz(exercise)
}

func FuzzStrip(f *testing.F) {
	jpg := testJPEG(f, 16, 16)
	f.Add(jpg)
	f.Add(withSegments(jpg, exifSegment(richTIFF(le)), segment(0xFE, []byte(private)), segment(0xEE, []byte("Adobe\x00"))))
	f.Add([]byte("\xFF\xD8\xFF\xDA\x00\x02\x12\xFF\x00\xFF\xD0\xFF\xD9"))
	f.Fuzz(func(t *testing.T, data []byte) {
		var out bytes.Buffer
		if err := exif.Strip(&out, bytes.NewReader(data), exif.KeepOrientation()); err != nil {
			return
		}
		if out.Len() > len(data)+64 {
			t.Fatalf("output %d bytes from %d", out.Len(), len(data))
		}
		got, err := exif.Parse(out.Bytes())
		if err != nil {
			return
		}
		n := 0
		for range got.Tags() {
			n++
		}
		if n > 1 {
			t.Fatalf("%d tags kept", n)
		}
		if in, err := exif.Parse(data); err == nil && in.Orientation() != got.Orientation() {
			t.Fatalf("orientation %d became %d", in.Orientation(), got.Orientation())
		}
		// Stripping twice changes nothing.
		var again bytes.Buffer
		if err := exif.Strip(&again, bytes.NewReader(out.Bytes()), exif.KeepOrientation()); err != nil || !bytes.Equal(again.Bytes(), out.Bytes()) {
			t.Fatalf("not idempotent: %v", err)
		}
	})
}
