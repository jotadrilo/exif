package exif_test

import (
	"bytes"
	"math"
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
		g, ok := e.GPS()
		if ok && (g.Latitude < -90 || g.Latitude > 90 || g.Longitude < -180 || g.Longitude > 180) {
			t.Fatalf("GPS %+v", g)
		}
		if sg, sok := slowGPS(e); sok != ok || (ok && sg != g) {
			t.Fatalf("GPS %+v %v, slow path %+v %v", g, ok, sg, sok)
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

// slowGPS is GPS's documented behavior written with the public accessors only, to check the
// fast path against.
func slowGPS(x exif.Exif) (exif.GPS, bool) {
	get := func(id uint16) (exif.Tag, bool) {
		var last exif.Tag
		found := false
		for tag := range x.Tags() {
			if tag.IFD == exif.GPSIFD && tag.ID == id {
				last, found = tag, true
			}
		}
		return last, found
	}
	letter := func(t exif.Tag) byte {
		s, err := t.Text()
		if err != nil || s == "" {
			return 0
		}
		c := s[0]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		return c
	}
	coord := func(v, ref exif.Tag, pos, neg byte, limit float64) (float64, bool) {
		sign := 0.0
		switch letter(ref) {
		case pos:
			sign = 1
		case neg:
			sign = -1
		default:
			return 0, false
		}
		if v.Count == 0 {
			return 0, false
		}
		d, scale := 0.0, 1.0
		for i := 0; i < 3 && i < int(v.Count); i++ {
			f, err := v.Float(i)
			if err != nil || f < 0 || math.IsInf(f, 0) || math.IsNaN(f) {
				return 0, false
			}
			d += f / scale
			scale *= 60
		}
		return sign * d, d <= limit
	}
	latRef, ok1 := get(1)
	lat, ok2 := get(2)
	lonRef, ok3 := get(3)
	lon, ok4 := get(4)
	if !ok1 || !ok2 || !ok3 || !ok4 {
		return exif.GPS{}, false
	}
	if st, ok := get(9); ok && letter(st) == 'V' {
		return exif.GPS{}, false
	}
	la, okA := coord(lat, latRef, 'N', 'S', 90)
	lo, okB := coord(lon, lonRef, 'E', 'W', 180)
	if !okA || !okB || (la == 0 && lo == 0) {
		return exif.GPS{}, false
	}
	g := exif.GPS{Latitude: la, Longitude: lo}
	if alt, ok := get(6); ok {
		if a, err := alt.Float(0); err == nil && a >= 0 && !math.IsInf(a, 0) {
			g.Altitude, g.HasAltitude = a, true
			if ref, ok := get(5); ok {
				if r, err := ref.Int(0); err == nil && r == 1 {
					g.Altitude = -a
				}
			}
		}
	}
	return g, true
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
