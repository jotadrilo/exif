package exif_test

import (
	"bytes"
	"errors"
	"io"
	"math"
	"testing"
	"time"

	"github.com/jotadrilo/exif"
)

var orders = map[string]byteOrder{"little-endian": le, "big-endian": be}

// containers wrap a TIFF structure in each supported format.
func containers(t testing.TB) map[string]func([]byte) []byte {
	return map[string]func([]byte) []byte{
		"JPEG": func(tiff []byte) []byte { return jpegWithTIFF(t, tiff) },
		"PNG":  func(tiff []byte) []byte { return pngWithTIFF(t, tiff) },
		"WebP": webpWithTIFF,
		"TIFF": func(tiff []byte) []byte { return tiff },
		"raw":  func(tiff []byte) []byte { return append([]byte("Exif\x00\x00"), tiff...) },
	}
}

// onlyReader hides every method but Read, so Decode cannot seek.
type onlyReader struct{ r io.Reader }

func (o onlyReader) Read(p []byte) (int, error) { return o.r.Read(p) }

// both parses data in memory and as a stream, with and without Seek, and checks that the
// three agree.
func both(t *testing.T, data []byte) (exif.Exif, error) {
	t.Helper()
	x, err := exif.Parse(data)
	for name, r := range map[string]io.Reader{"seeker": bytes.NewReader(data), "reader": onlyReader{bytes.NewReader(data)}} {
		y, errY := exif.Decode(r)
		if (err == nil) != (errY == nil) {
			t.Fatalf("Decode (%s) error %v, Parse error %v", name, errY, err)
		}
		if !bytes.Equal(x.Raw(), y.Raw()) {
			t.Fatalf("Decode (%s) found other EXIF bytes than Parse", name)
		}
	}
	return x, err
}

func TestOrientation_AllEightInEveryFormatAndByteOrder(t *testing.T) {
	for format, wrap := range containers(t) {
		for name, o := range orders {
			for want := 1; want <= 8; want++ {
				x, err := both(t, wrap(orientationTIFF(o, uint16(want))))
				if err != nil {
					t.Fatalf("%s %s %d: %v", format, name, want, err)
				}
				if got := x.Orientation(); got != want {
					t.Errorf("%s %s: orientation %d, want %d", format, name, got, want)
				}
			}
		}
	}
}

// TestOrientation_ReadAsGoexifDoes pins the cases unpixel-web's tests pin against the reader it
// used, rwcarlsen/goexif.
func TestOrientation_ReadAsGoexifDoes(t *testing.T) {
	type raw struct {
		kind  uint16
		count uint32
		value []byte
	}
	tiffOf := func(o byteOrder, data []byte, es ...raw) []byte {
		mark := "II"
		if o == be {
			mark = "MM"
		}
		b := o.AppendUint16([]byte(mark), 42)
		b = o.AppendUint32(b, 8)
		b = o.AppendUint16(b, uint16(len(es)))
		for _, e := range es {
			b = o.AppendUint16(b, 0x0112)
			b = o.AppendUint16(b, e.kind)
			b = o.AppendUint32(b, e.count)
			b = append(b, e.value...)
		}
		b = o.AppendUint32(b, 0)
		return append(b, data...)
	}
	sh := func(o byteOrder, v uint16) []byte { return append(o.AppendUint16(nil, v), 0, 0) }
	dataAt := uint32(8 + 2 + 12 + 4)
	cases := map[string]struct {
		tiff []byte
		want int
	}{
		"a SHORT of count 2":           {tiffOf(le, nil, raw{3, 2, []byte{6, 0, 3, 0}}), 6},
		"a SHORT of count 3 at offset": {tiffOf(be, []byte{0, 8, 0, 1, 0, 1}, raw{3, 3, be.AppendUint32(nil, dataAt)}), 8},
		"a LONG":                       {tiffOf(le, nil, raw{4, 1, le.AppendUint32(nil, 5)}), 5},
		"a BYTE":                       {tiffOf(be, nil, raw{1, 1, []byte{7, 0, 0, 0}}), 7},
		"an SSHORT":                    {tiffOf(be, nil, raw{8, 1, sh(be, 3)}), 3},
		"an SLONG":                     {tiffOf(le, nil, raw{9, 1, le.AppendUint32(nil, 2)}), 2},
		"the last of two":              {tiffOf(le, nil, raw{3, 1, sh(le, 3)}, raw{3, 1, sh(le, 6)}), 6},
		"a negative SSHORT":            {tiffOf(le, nil, raw{8, 1, sh(le, 0xFFFA)}), 1},
		"a count of 0":                 {tiffOf(le, nil, raw{3, 0, sh(le, 6)}), 1},
		"a RATIONAL":                   {tiffOf(be, be.AppendUint32(be.AppendUint32(nil, 6), 1), raw{5, 1, be.AppendUint32(nil, dataAt)}), 1},
		"an offset past the end":       {tiffOf(le, nil, raw{3, 3, le.AppendUint32(nil, 0xFFFF)}), 1},
		"a normal orientation":         {orientationTIFF(le, 1), 1},
		"no orientation":               {buildTIFF(layout{order: le, ifd0: []entry{ascii(0x010F, "Maker")}}), 1},
		"an orientation past 8":        {orientationTIFF(be, 9), 1},
		"an orientation of 0":          {orientationTIFF(be, 0), 1},
		"nonsense":                     {[]byte("private-name.png"), 1},
		"cut in its IFD":               {orientationTIFF(le, 6)[:14], 1},
		"an IFD past its end":          {[]byte("II*\x00\xff\xff\x00\x00"), 1},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			x, _ := both(t, jpegWithTIFF(t, c.tiff))
			if got := x.Orientation(); got != c.want {
				t.Errorf("orientation %d, want %d", got, c.want)
			}
		})
	}
}

func gpsTIFF(o byteOrder, gps ...entry) []byte {
	return buildTIFF(layout{order: o, ifd0: []entry{short(o, 0x0112, 1)}, gps: gps})
}

func TestGPS(t *testing.T) {
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	for name, o := range orders {
		madrid := gpsEntries(o, "N", [3]uint32{40, 25, 0}, "W", [3]uint32{3, 42, 0})
		sydney := gpsEntries(o, "S", [3]uint32{33, 52, 0}, "E", [3]uint32{151, 12, 0})
		cases := map[string]struct {
			gps  []entry
			want exif.GPS
			ok   bool
		}{
			"north and west": {madrid, exif.GPS{Latitude: 40 + 25.0/60, Longitude: -3.7}, true},
			"south and east": {sydney, exif.GPS{Latitude: -(33 + 52.0/60), Longitude: 151.2}, true},
			"above sea level": {append(append([]entry(nil), madrid...), byteTag(0x5, 0), rational(o, 0x6, 6571, 10)),
				exif.GPS{Latitude: 40 + 25.0/60, Longitude: -3.7, Altitude: 657.1, HasAltitude: true}, true},
			"below sea level": {append(append([]entry(nil), sydney...), byteTag(0x5, 1), rational(o, 0x6, 430, 1)),
				exif.GPS{Latitude: -(33 + 52.0/60), Longitude: 151.2, Altitude: -430, HasAltitude: true}, true},
			"an altitude without its reference": {append(append([]entry(nil), madrid...), rational(o, 0x6, 12, 1)),
				exif.GPS{Latitude: 40 + 25.0/60, Longitude: -3.7, Altitude: 12, HasAltitude: true}, true},
			"an unreadable altitude": {append(append([]entry(nil), madrid...), rational(o, 0x6, 12, 0)),
				exif.GPS{Latitude: 40 + 25.0/60, Longitude: -3.7}, true},
			"fractional minutes": {[]entry{ascii(1, "n"), rational(o, 2, 48, 1, 5129, 100, 0, 1), ascii(3, "e"), rational(o, 4, 2, 1, 1765, 100, 0, 1)},
				exif.GPS{Latitude: 48 + 51.29/60, Longitude: 2 + 17.65/60}, true},
			"a valid status": {append(append([]entry(nil), madrid...), ascii(0x9, "A")),
				exif.GPS{Latitude: 40 + 25.0/60, Longitude: -3.7}, true},
			"a void status":         {append(append([]entry(nil), madrid...), ascii(0x9, "V")), exif.GPS{}, false},
			"null island":           {gpsEntries(o, "N", [3]uint32{}, "E", [3]uint32{}), exif.GPS{}, false},
			"a latitude past 90":    {gpsEntries(o, "N", [3]uint32{91, 0, 0}, "E", [3]uint32{1, 0, 0}), exif.GPS{}, false},
			"a longitude past 180":  {gpsEntries(o, "N", [3]uint32{1, 0, 0}, "E", [3]uint32{180, 0, 1}), exif.GPS{}, false},
			"no longitude":          {madrid[:2], exif.GPS{}, false},
			"no latitude reference": {madrid[1:], exif.GPS{}, false},
			"an unknown reference":  {gpsEntries(o, "X", [3]uint32{1, 0, 0}, "E", [3]uint32{1, 0, 0}), exif.GPS{}, false},
			"a zero denominator": {[]entry{ascii(1, "N"), rational(o, 2, 1, 0, 0, 1, 0, 1), ascii(3, "W"), rational(o, 4, 3, 1, 42, 1, 0, 1)},
				exif.GPS{}, false},
			"a latitude of another type": {[]entry{ascii(1, "N"), ascii(2, "40"), ascii(3, "W"), rational(o, 4, 3, 1, 42, 1, 0, 1)},
				exif.GPS{}, false},
		}
		for cname, c := range cases {
			t.Run(name+"/"+cname, func(t *testing.T) {
				for format, wrap := range containers(t) {
					x, err := both(t, wrap(gpsTIFF(o, c.gps...)))
					if err != nil {
						t.Fatal(err)
					}
					g, ok := x.GPS()
					if ok != c.ok || !near(g.Latitude, c.want.Latitude) || !near(g.Longitude, c.want.Longitude) ||
						!near(g.Altitude, c.want.Altitude) || g.HasAltitude != c.want.HasAltitude {
						t.Errorf("%s: GPS() = %+v, %v; want %+v, %v", format, g, ok, c.want, c.ok)
					}
				}
			})
		}
	}
}

func TestGPS_AbsentWithoutAGPSDirectory(t *testing.T) {
	for name, data := range map[string][]byte{
		"orientation only": jpegWithTIFF(t, orientationTIFF(le, 6)),
		"no EXIF":          testJPEG(t, 8, 8),
	} {
		x, _ := exif.Parse(data)
		if _, ok := x.GPS(); ok {
			t.Errorf("%s: GPS reported", name)
		}
	}
}

// TestGPS_ALatitudeAloneIsReadableAsATag pins unpixel-web's GPS fixture: a GPS IFD holding
// only 40° 25' N.
func TestGPS_ALatitudeAloneIsReadableAsATag(t *testing.T) {
	tiffGPS := "II*\x00\x08\x00\x00\x00" +
		"\x01\x00\x25\x88\x04\x00\x01\x00\x00\x00\x1a\x00\x00\x00\x00\x00\x00\x00" +
		"\x02\x00" +
		"\x01\x00\x02\x00\x02\x00\x00\x00N\x00\x00\x00" +
		"\x02\x00\x05\x00\x03\x00\x00\x00\x38\x00\x00\x00" +
		"\x00\x00\x00\x00" +
		"\x28\x00\x00\x00\x01\x00\x00\x00\x19\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x01\x00\x00\x00"
	x, err := both(t, jpegWithTIFF(t, []byte(tiffGPS)))
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := x.Tag(exif.GPSIFD, exif.TagGPSLatitude)
	if !ok {
		t.Fatal("no GPSLatitude")
	}
	if m, err := tag.Float(1); err != nil || m != 25 {
		t.Errorf("minutes %v, %v", m, err)
	}
	if _, ok := x.GPS(); ok {
		t.Error("a position without a longitude")
	}
}

func dateTIFF(o byteOrder, exifIFD ...entry) []byte {
	return buildTIFF(layout{order: o, ifd0: []entry{ascii(0x010F, "Maker")}, exif: exifIFD})
}

func TestDateTimeOriginal(t *testing.T) {
	const dt = "2024:07:14 18:30:05"
	cases := map[string]struct {
		entries   []entry
		want      time.Time
		hasOffset bool
		ok        bool
	}{
		"with an offset east": {[]entry{ascii(0x9003, dt), ascii(0x9011, "+02:00")},
			time.Date(2024, 7, 14, 18, 30, 5, 0, time.FixedZone("", 7200)), true, true},
		"with an offset west": {[]entry{ascii(0x9003, dt), ascii(0x9011, "-05:30")},
			time.Date(2024, 7, 14, 18, 30, 5, 0, time.FixedZone("", -19800)), true, true},
		"with a zero offset": {[]entry{ascii(0x9003, dt), ascii(0x9011, "+00:00")},
			time.Date(2024, 7, 14, 18, 30, 5, 0, time.UTC), true, true},
		"without an offset": {[]entry{ascii(0x9003, dt)},
			time.Date(2024, 7, 14, 18, 30, 5, 0, time.UTC), false, true},
		"with sub-seconds": {[]entry{ascii(0x9003, dt), ascii(0x9291, "042"), ascii(0x9011, "+01:00")},
			time.Date(2024, 7, 14, 18, 30, 5, 42_000_000, time.FixedZone("", 3600)), true, true},
		"with a broken offset": {[]entry{ascii(0x9003, dt), ascii(0x9011, "02:00")},
			time.Date(2024, 7, 14, 18, 30, 5, 0, time.UTC), false, true},
		"blank":         {[]entry{ascii(0x9003, "    :  :     :  :  ")}, time.Time{}, false, false},
		"zeroed":        {[]entry{ascii(0x9003, "0000:00:00 00:00:00")}, time.Time{}, false, false},
		"February 30th": {[]entry{ascii(0x9003, "2023:02:30 10:00:00")}, time.Time{}, false, false},
		"too short":     {[]entry{ascii(0x9003, "2023:02:01")}, time.Time{}, false, false},
		"absent":        {[]entry{ascii(0x9004, dt)}, time.Time{}, false, false},
	}
	for name, o := range orders {
		for cname, c := range cases {
			x, err := both(t, jpegWithTIFF(t, dateTIFF(o, c.entries...)))
			if err != nil {
				t.Fatal(err)
			}
			got, hasOffset, ok := x.DateTimeOriginal()
			if ok != c.ok || hasOffset != c.hasOffset || !got.Equal(c.want) {
				t.Errorf("%s/%s: %v %v %v, want %v %v %v", name, cname, got, hasOffset, ok, c.want, c.hasOffset, c.ok)
			}
			if ok {
				_, gotOff := got.Zone()
				_, wantOff := c.want.Zone()
				if gotOff != wantOff {
					t.Errorf("%s/%s: offset %d, want %d", name, cname, gotOff, wantOff)
				}
			}
		}
	}
}

func TestMakeModelAndDimensions(t *testing.T) {
	for name, o := range orders {
		x, err := both(t, jpegWithTIFF(t, buildTIFF(layout{
			order: o,
			ifd0:  []entry{ascii(0x010F, "Canon"), ascii(0x0110, "Canon EOS R5   "), long(o, 0x0100, 640), long(o, 0x0101, 480)},
			exif:  []entry{long(o, 0xA002, 8192), short(o, 0xA003, 5464)},
		})))
		if err != nil {
			t.Fatal(err)
		}
		if x.Make() != "Canon" || x.Model() != "Canon EOS R5" {
			t.Errorf("%s: make %q model %q", name, x.Make(), x.Model())
		}
		if w, h, ok := x.Dimensions(); !ok || w != 8192 || h != 5464 {
			t.Errorf("%s: dimensions %d x %d %v", name, w, h, ok)
		}
		y, _ := exif.Parse(buildTIFF(layout{order: o, ifd0: []entry{short(o, 0x0100, 640), short(o, 0x0101, 480)}}))
		if w, h, ok := y.Dimensions(); !ok || w != 640 || h != 480 {
			t.Errorf("%s: IFD0 dimensions %d x %d %v", name, w, h, ok)
		}
		var zero exif.Exif
		if zero.Make() != "" || zero.Orientation() != 1 {
			t.Error("zero Exif")
		}
		if _, _, ok := zero.Dimensions(); ok {
			t.Error("zero Exif has dimensions")
		}
		if _, _, ok := zero.DateTimeOriginal(); ok {
			t.Error("zero Exif has a date")
		}
	}
}

func TestTag_EveryType(t *testing.T) {
	for name, o := range orders {
		f32 := o.AppendUint32(nil, math.Float32bits(1.5))
		f64 := o.AppendUint64(nil, math.Float64bits(-2.25))
		ifd0 := []entry{
			byteTag(0x1000, 1, 2, 255),
			ascii(0x1001, "hello"),
			short(o, 0x1002, 1, 65535),
			long(o, 0x1003, 4294967295),
			rational(o, 0x1004, 1, 3, 5, 2),
			{0x1005, 6, 2, []byte{0xFF, 0x7F}},
			{0x1006, 7, 5, []byte("\x00\x01\x02\x03\x04")},
			{0x1007, 8, 1, o.AppendUint16(nil, 0x8000)},
			{0x1008, 9, 1, o.AppendUint32(nil, 0xFFFFFFFF)},
			{0x1009, 10, 1, append(o.AppendUint32(nil, 0xFFFFFFFE), o.AppendUint32(nil, 4)...)},
			{0x100A, 11, 1, f32},
			{0x100B, 12, 1, f64},
			{0x100C, 99, 1, []byte{1}},
			{0x100D, 3, 100, o.AppendUint32(nil, 1<<30)}, // past the end
		}
		x, err := exif.Parse(buildTIFF(layout{order: o, ifd0: ifd0}))
		if err != nil {
			t.Fatal(err)
		}
		get := func(id uint16) exif.Tag {
			tag, ok := x.Tag(exif.IFD0, id)
			if !ok {
				t.Fatalf("%s: no tag %#x", name, id)
			}
			return tag
		}
		ints := map[uint16][]int64{
			0x1000: {1, 2, 255}, 0x1002: {1, 65535}, 0x1003: {4294967295},
			0x1005: {-1, 127}, 0x1007: {-32768}, 0x1008: {-1},
		}
		for id, want := range ints {
			tag := get(id)
			for i, w := range want {
				if v, err := tag.Int(i); err != nil || v != w {
					t.Errorf("%s: %#x[%d] = %d, %v; want %d", name, id, i, v, err, w)
				}
			}
			if _, err := tag.Int(len(want)); !errors.Is(err, exif.ErrIndex) {
				t.Errorf("%s: %#x past its count: %v", name, id, err)
			}
		}
		if s, err := get(0x1001).Text(); err != nil || s != "hello" {
			t.Errorf("%s: text %q %v", name, s, err)
		}
		if n, d, err := get(0x1004).Rat(1); err != nil || n != 5 || d != 2 {
			t.Errorf("%s: rational %d/%d %v", name, n, d, err)
		}
		if f, err := get(0x1004).Float(0); err != nil || math.Abs(f-1.0/3) > 1e-12 {
			t.Errorf("%s: rational float %v %v", name, f, err)
		}
		if n, d, err := get(0x1009).Rat(0); err != nil || n != -2 || d != 4 {
			t.Errorf("%s: srational %d/%d %v", name, n, d, err)
		}
		if b := get(0x1006).Bytes(); string(b) != "\x00\x01\x02\x03\x04" {
			t.Errorf("%s: undefined %q", name, b)
		}
		if f, err := get(0x100A).Float(0); err != nil || f != 1.5 {
			t.Errorf("%s: float %v %v", name, f, err)
		}
		if f, err := get(0x100B).Float(0); err != nil || f != -2.25 {
			t.Errorf("%s: double %v %v", name, f, err)
		}
		if f, err := get(0x1003).Float(0); err != nil || f != 4294967295 {
			t.Errorf("%s: long as float %v %v", name, f, err)
		}
		if _, err := get(0x1004).Int(0); !errors.Is(err, exif.ErrType) {
			t.Errorf("%s: rational as int: %v", name, err)
		}
		if _, err := get(0x1003).Text(); !errors.Is(err, exif.ErrType) {
			t.Errorf("%s: long as text: %v", name, err)
		}
		if tag := get(0x100C); tag.Valid() || tag.Bytes() != nil {
			t.Errorf("%s: unknown type readable", name)
		}
		if _, err := get(0x100C).Float(0); !errors.Is(err, exif.ErrType) {
			t.Errorf("%s: unknown type: %v", name, err)
		}
		if tag := get(0x100D); tag.Valid() {
			t.Errorf("%s: value past the end is valid", name)
		} else if _, err := tag.Int(0); !errors.Is(err, exif.ErrMalformed) {
			t.Errorf("%s: value past the end: %v", name, err)
		}
		if x.ByteOrder() != o {
			t.Errorf("%s: byte order", name)
		}
	}
}

func TestTags_VisitsEveryDirectoryOnce(t *testing.T) {
	o := le
	data := buildTIFF(layout{
		order: o,
		ifd0:  []entry{short(o, 0x0112, 6)},
		exif:  []entry{ascii(0x9003, "2024:01:02 03:04:05")},
		gps:   gpsEntries(o, "N", [3]uint32{1, 2, 3}, "E", [3]uint32{4, 5, 6}),
		ifd1:  []entry{long(o, 0x0201, 0), long(o, 0x0202, 0)},
	})
	x, err := exif.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	count := map[exif.IFD]int{}
	for tag := range x.Tags() {
		count[tag.IFD]++
	}
	want := map[exif.IFD]int{exif.IFD0: 3, exif.ExifIFD: 1, exif.GPSIFD: 4, exif.IFD1: 2}
	for d, n := range want {
		if count[d] != n {
			t.Errorf("%v: %d tags, want %d", d, count[d], n)
		}
	}
	n := 0
	for range x.Tags() {
		n++
		break
	}
	if n != 1 {
		t.Error("Tags did not stop")
	}
}

func TestLoopsAndSharedOffsets(t *testing.T) {
	o := le
	// IFD0 at 8 with one entry; its pointers and its next link are patched below.
	base := func(id uint16, target uint32, next uint32) []byte {
		b := o.AppendUint16([]byte("II"), 42)
		b = o.AppendUint32(b, 8)
		b = o.AppendUint16(b, 2)
		b = append(b, short(o, 0x0112, 3).bytes(o)...)
		b = o.AppendUint16(b, id)
		b = o.AppendUint16(b, 4)
		b = o.AppendUint32(b, 1)
		b = o.AppendUint32(b, target)
		return o.AppendUint32(b, next)
	}
	for name, data := range map[string][]byte{
		"Exif IFD pointing at IFD0":  base(0x8769, 8, 0),
		"GPS IFD pointing at IFD0":   base(0x8825, 8, 0),
		"IFD1 pointing at IFD0":      base(0x8769, 0, 8),
		"Exif IFD past the end":      base(0x8769, 0xFFFFFFF0, 0),
		"IFD1 past the end":          base(0x8769, 0, 0xFFFFFFF0),
		"Exif IFD inside the header": base(0x8769, 2, 0),
	} {
		x, err := exif.Parse(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		n := 0
		for range x.Tags() {
			n++
		}
		if n != 2 || x.Orientation() != 3 {
			t.Errorf("%s: %d tags, orientation %d", name, n, x.Orientation())
		}
		if _, ok := x.GPS(); ok {
			t.Errorf("%s: GPS", name)
		}
	}
	// A GPS IFD sharing the Exif IFD's offset is visited once.
	shared := buildTIFF(layout{order: o, ifd0: []entry{short(o, 0x0112, 3)}, exif: []entry{ascii(0x9003, "x")}})
	x, _ := exif.Parse(shared)
	exifAt, _ := x.Tag(exif.IFD0, exif.TagExifIFDPointer)
	at, _ := exifAt.Int(0)
	shared = buildTIFF(layout{order: o, ifd0: []entry{short(o, 0x0112, 3), long(o, 0x8825, uint32(at)+12)}, exif: []entry{ascii(0x9003, "x")}})
	x, _ = exif.Parse(shared)
	n := 0
	for range x.Tags() {
		n++
	}
	if n < 3 {
		t.Errorf("shared offsets: %d tags", n)
	}
}

// bytes returns an entry's 12 bytes with its value inline.
func (e entry) bytes(o byteOrder) []byte {
	b := o.AppendUint16(nil, e.id)
	b = o.AppendUint16(b, e.typ)
	b = o.AppendUint32(b, e.count)
	return append(b, append(append([]byte(nil), e.value...), make([]byte, 4-len(e.value))...)...)
}

func TestMalformedInput(t *testing.T) {
	jpg := testJPEG(t, 8, 8)
	cases := map[string]struct {
		data []byte
		err  error
	}{
		"empty":                   {nil, exif.ErrUnknownFormat},
		"one byte":                {[]byte{0xFF}, exif.ErrUnknownFormat},
		"text":                    {[]byte("hello, world"), exif.ErrUnknownFormat},
		"GIF":                     {[]byte("GIF89a\x01\x00\x01\x00"), exif.ErrUnknownFormat},
		"JPEG without EXIF":       {jpg, exif.ErrNoExif},
		"JPEG with XMP only":      {withSegments(jpg, segment(0xE1, []byte("http://ns.adobe.com/xap/1.0/\x00<x/>"))), exif.ErrNoExif},
		"JPEG cut after SOI":      {jpg[:2], exif.ErrNoExif},
		"JPEG with garbage":       {[]byte("\xFF\xD8garbage"), exif.ErrMalformed},
		"JPEG with a short len":   {[]byte("\xFF\xD8\xFF\xE0\x00\x01"), exif.ErrMalformed},
		"JPEG cut in a segment":   {[]byte("\xFF\xD8\xFF\xE0\x00\x10JFIF"), exif.ErrMalformed},
		"JPEG with a second SOI":  {[]byte("\xFF\xD8\xFF\xD8"), exif.ErrMalformed},
		"JPEG EXIF without TIFF":  {withSegments(jpg, exifSegment(nil)), exif.ErrMalformed},
		"BigTIFF":                 {[]byte("II\x2b\x00\x08\x00\x00\x00\x00\x00"), exif.ErrUnknownFormat},
		"TIFF with IFD0 past end": {[]byte("MM\x00*\x7f\xff\xff\xff"), exif.ErrMalformed},
		"TIFF header only":        {[]byte("II*\x00"), exif.ErrMalformed},
		"raw EXIF, bad order":     {[]byte("Exif\x00\x00XX*\x00\x08\x00\x00\x00"), exif.ErrMalformed},
		"PNG without EXIF":        {pngWithTIFF(t, nil), exif.ErrNoExif},
		"PNG signature only":      {[]byte("\x89PNG\r\n\x1a\n"), exif.ErrMalformed},
		"PNG eXIf cut short":      {append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x10\x00eXIf"), "II*\x00"...), exif.ErrMalformed},
		"PNG chunk past the end":  {[]byte("\x89PNG\r\n\x1a\n\x00\x10\x00\x00IDAT\x00"), exif.ErrMalformed},
		"WebP simple":             {append([]byte("RIFF\x1e\x00\x00\x00WEBP"), webpChunk("VP8L", lossless1x1)...), exif.ErrNoExif},
		"WebP extended, no EXIF":  {webpWithTIFF(nil), exif.ErrNoExif},
		"WebP header only":        {[]byte("RIFF\x04\x00\x00\x00WEBP"), exif.ErrMalformed},
		"WebP chunk past the end": {[]byte("RIFF\x14\x00\x00\x00WEBPVP8X\xff\x00\x00\x00\x00\x00\x00\x00"), exif.ErrMalformed},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			x, err := exif.Parse(c.data)
			if !errors.Is(err, c.err) {
				t.Errorf("Parse: %v, want %v", err, c.err)
			}
			if x.Orientation() != 1 {
				t.Error("orientation of a failed parse")
			}
			for _, r := range []io.Reader{bytes.NewReader(c.data), onlyReader{bytes.NewReader(c.data)}} {
				if _, err := exif.Decode(r); !errors.Is(err, c.err) {
					t.Errorf("Decode: %v, want %v", err, c.err)
				}
			}
		})
	}
}

// countingReader counts what Decode reads.
type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func TestDecode_ReadsOnlyTheMetadata(t *testing.T) {
	big := withSegments(testJPEG(t, 512, 512), exifSegment(orientationTIFF(le, 6)))
	c := &countingReader{r: bytes.NewReader(big)}
	x, err := exif.Decode(c)
	if err != nil || x.Orientation() != 6 {
		t.Fatal(err)
	}
	if c.n > 200 {
		t.Errorf("read %d of %d bytes", c.n, len(big))
	}

	// A PNG with its eXIf after a large IDAT: seeking skips the pixels.
	p := pngWithTIFF(t, nil)
	iend := len(p) - 12
	huge := append(append(append([]byte(nil), p[:iend]...), pngChunk("IDAT", make([]byte, 1<<20))...), pngChunk("eXIf", orientationTIFF(be, 8))...)
	huge = append(huge, p[iend:]...)
	sr := &seekCounter{Reader: bytes.NewReader(huge)}
	x, err = exif.Decode(sr)
	if err != nil || x.Orientation() != 8 {
		t.Fatal(err)
	}
	if sr.n > 1000 {
		t.Errorf("read %d bytes of %d", sr.n, len(huge))
	}
}

type seekCounter struct {
	*bytes.Reader
	n int
}

func (s *seekCounter) Read(p []byte) (int, error) {
	n, err := s.Reader.Read(p)
	s.n += n
	return n, err
}

func TestDecode_Limits(t *testing.T) {
	tiff := orientationTIFF(le, 6)
	p := pngWithTIFF(t, tiff)
	if _, err := (exif.Decoder{MaxMetadata: int64(len(tiff) - 1)}).Decode(bytes.NewReader(p)); !errors.Is(err, exif.ErrTooLarge) {
		t.Errorf("PNG past MaxMetadata: %v", err)
	}
	w := webpWithTIFF(tiff)
	if _, err := (exif.Decoder{MaxMetadata: 4}).Decode(bytes.NewReader(w)); !errors.Is(err, exif.ErrTooLarge) {
		t.Errorf("WebP past MaxMetadata: %v", err)
	}
	if _, err := (exif.Decoder{MaxTIFF: 10}).Decode(bytes.NewReader(tiff)); !errors.Is(err, exif.ErrTooLarge) {
		t.Errorf("TIFF past MaxTIFF: %v", err)
	}
	late := pngWithTIFF(t, nil)
	if _, err := (exif.Decoder{MaxScan: 10}).Decode(bytes.NewReader(late)); !errors.Is(err, exif.ErrTooLarge) {
		t.Errorf("past MaxScan: %v", err)
	}
	// An endless stream of empty chunks ends at MaxScan.
	endless := io.MultiReader(bytes.NewReader([]byte(pngSignature)), &repeat{b: []byte("\x00\x00\x00\x00tEXt\x00\x00\x00\x00")})
	if _, err := (exif.Decoder{MaxScan: 1 << 16}).Decode(endless); !errors.Is(err, exif.ErrTooLarge) {
		t.Errorf("endless stream: %v", err)
	}
}

const pngSignature = "\x89PNG\r\n\x1a\n"

type repeat struct {
	b []byte
	i int
}

func (r *repeat) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = r.b[r.i%len(r.b)]
		r.i++
	}
	return len(p), nil
}

func TestAllocations(t *testing.T) {
	o := be
	data := jpegWithTIFF(t, buildTIFF(layout{
		order: o,
		ifd0:  []entry{ascii(0x010F, "Maker"), short(o, 0x0112, 6)},
		exif:  []entry{ascii(0x9003, "2024:07:14 18:30:05"), ascii(0x9011, "+00:00")},
		gps:   gpsEntries(o, "S", [3]uint32{33, 52, 0}, "E", [3]uint32{151, 12, 0}),
	}))
	for name, f := range map[string]func(){
		"Orientation": func() {
			x, _ := exif.Parse(data)
			_ = x.Orientation()
		},
		"GPS": func() {
			x, _ := exif.Parse(data)
			_, _ = x.GPS()
		},
		"DateTimeOriginal": func() {
			x, _ := exif.Parse(data)
			_, _, _ = x.DateTimeOriginal()
		},
	} {
		if n := testing.AllocsPerRun(100, f); n != 0 {
			t.Errorf("%s: %v allocations", name, n)
		}
	}
}
