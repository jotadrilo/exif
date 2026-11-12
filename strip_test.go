package exif_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image/jpeg"
	"io"
	"strings"
	"testing"

	"github.com/jotadrilo/exif"
)

const private = "private-name.png"

func strip(t *testing.T, in []byte, opts ...exif.StripOption) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := exif.Strip(&out, bytes.NewReader(in), opts...); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestStrip_RemovesEveryMetadataSegmentAndKeepsTheImageBytes(t *testing.T) {
	clean := testJPEG(t, 32, 24)
	gps := buildTIFF(layout{order: le, ifd0: []entry{short(le, 0x0112, 6), ascii(0x010F, private)},
		gps: gpsEntries(le, "N", [3]uint32{40, 25, 0}, "W", [3]uint32{3, 42, 0})})
	sent := withSegments(clean,
		segment(0xE0, []byte("JFIF\x00\x01\x02\x00\x00\x01\x00\x01\x00\x00")),
		exifSegment(gps),
		segment(0xE1, []byte("http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta>"+private+"</x:xmpmeta>")),
		segment(0xE2, []byte("ICC_PROFILE\x00\x01\x01"+private)),
		segment(0xED, []byte("Photoshop 3.0\x00"+private)),
		segment(0xFE, []byte(private)),
		segment(0xC8, []byte(private)),
		segment(0xF0, []byte(private)),
		segment(0xFD, []byte(private)),
		segment(0x02, []byte(private)),
		segment(0xBF, []byte(private)),
	)
	sent = append(sent, "after-the-end "+private...)

	got := strip(t, sent)

	if !bytes.Equal(got, clean) {
		t.Fatalf("stripped JPEG differs from the clean one (%d vs %d bytes)", len(got), len(clean))
	}
	if strings.Contains(string(got), private) {
		t.Error("private data kept")
	}
	if _, err := exif.Parse(got); !errors.Is(err, exif.ErrNoExif) {
		t.Errorf("EXIF left: %v", err)
	}
}

func TestStrip_KeepsAdobeAndOptionallyTheICCProfile(t *testing.T) {
	clean := testJPEG(t, 8, 8)
	adobe := segment(0xEE, []byte("Adobe\x00\x64\x00\x00\x00\x00\x01"))
	icc := segment(0xE2, []byte("ICC_PROFILE\x00\x01\x01profile"))
	sent := withSegments(clean, icc, adobe)

	if got := strip(t, sent); !bytes.Equal(got, withSegments(clean, adobe)) {
		t.Error("default: want the Adobe segment alone")
	}
	if got := strip(t, sent, exif.KeepICCProfile()); !bytes.Equal(got, sent) {
		t.Error("KeepICCProfile: want both")
	}
}

func TestStrip_KeepOrientation(t *testing.T) {
	clean := testJPEG(t, 16, 16)
	for name, o := range orders {
		for want := 1; want <= 8; want++ {
			tiff := buildTIFF(layout{order: o, ifd0: []entry{ascii(0x010F, private), short(o, 0x0112, uint16(want))},
				gps: gpsEntries(o, "S", [3]uint32{1, 2, 3}, "E", [3]uint32{4, 5, 6})})
			sent := withSegments(clean, segment(0xE0, []byte("JFIF\x00")), exifSegment(tiff))

			got := strip(t, sent, exif.KeepOrientation())

			x, err := exif.Parse(got)
			if want == 1 {
				if !bytes.Equal(got, clean) {
					t.Errorf("%s: orientation 1 kept a segment", name)
				}
				continue
			}
			if err != nil || x.Orientation() != want {
				t.Errorf("%s: orientation %d (%v), want %d", name, x.Orientation(), err, want)
			}
			if _, ok := x.GPS(); ok || x.Make() != "" {
				t.Errorf("%s: kept more than the orientation", name)
			}
			n := 0
			for range x.Tags() {
				n++
			}
			if n != 1 {
				t.Errorf("%s: %d tags", name, n)
			}
			seg := 4 + int(binary.BigEndian.Uint16(got[4:]))
			if !bytes.Equal(append(got[:2:2], got[seg:]...), clean) {
				t.Errorf("%s: past the orientation the image differs", name)
			}
		}
	}
}

// TestStrip_KeepOrientationAsItIsRead pins unpixel-web's cases: the orientation kept is the one
// Orientation reads.
func TestStrip_KeepOrientationAsItIsRead(t *testing.T) {
	clean := testJPEG(t, 8, 8)
	for name, tiff := range map[string][]byte{
		"a LONG":        buildTIFF(layout{order: le, ifd0: []entry{long(le, 0x0112, 5)}}),
		"a BYTE":        buildTIFF(layout{order: be, ifd0: []entry{byteTag(0x0112, 7)}}),
		"the last":      buildTIFF(layout{order: le, ifd0: []entry{short(le, 0x0112, 3), short(le, 0x0112, 6)}}),
		"past 8":        orientationTIFF(be, 9),
		"nonsense":      []byte(private),
		"cut in an IFD": orientationTIFF(le, 6)[:14],
	} {
		sent := withSegments(clean, exifSegment(tiff))
		before, _ := exif.Parse(sent)
		after, _ := exif.Parse(strip(t, sent, exif.KeepOrientation()))
		if before.Orientation() != after.Orientation() {
			t.Errorf("%s: %d became %d", name, before.Orientation(), after.Orientation())
		}
	}
	// Only the first EXIF segment counts, as readers take it.
	sent := withSegments(clean, exifSegment(orientationTIFF(le, 6)), exifSegment(orientationTIFF(le, 3)))
	if x, _ := exif.Parse(strip(t, sent, exif.KeepOrientation())); x.Orientation() != 6 {
		t.Errorf("first segment: %d", x.Orientation())
	}
}

func TestStrip_ScanWithStuffingRestartsAndFill(t *testing.T) {
	header := segment(0xDB, []byte("quantization"))
	scan := segment(0xDA, []byte("scan-header"))
	data := []byte{0x12, 0xFF, 0x00, 0x34, 0xFF, 0xD0, 0x56, 0xFF, 0xD7, 0x78}
	second := segment(0xC4, []byte("huffman"))
	sent := bytes.Join([][]byte{{0xFF, 0xD8}, segment(0xE0, []byte("JFIF")), header, scan, data,
		{0xFF, 0xFF}, segment(0xFE, []byte(private)), second, scan, data, {0xFF, 0xFF, 0xD9}}, nil)

	got := strip(t, sent)

	want := bytes.Join([][]byte{{0xFF, 0xD8}, header, scan, data, second, scan, data, {0xFF, 0xD9}}, nil)
	if !bytes.Equal(got, want) {
		t.Errorf("got  %x\nwant %x", got, want)
	}
}

func TestStrip_DecodesToTheSamePixels(t *testing.T) {
	clean := testJPEG(t, 300, 200) // larger than the reader's buffer scans span chunks
	sent := withSegments(clean, exifSegment(orientationTIFF(le, 6)), segment(0xFE, []byte(private)))
	got := strip(t, sent)
	a, err := jpeg.Decode(bytes.NewReader(clean))
	if err != nil {
		t.Fatal(err)
	}
	b, err := jpeg.Decode(bytes.NewReader(got))
	if err != nil {
		t.Fatal(err)
	}
	if a.Bounds() != b.Bounds() {
		t.Fatal("bounds")
	}
	if !bytes.Equal(got, clean) {
		t.Error("bytes differ")
	}
}

func TestStrip_Errors(t *testing.T) {
	jpg := testJPEG(t, 8, 8)
	for name, c := range map[string]struct {
		in  []byte
		err error
	}{
		"empty":                 {nil, exif.ErrUnknownFormat},
		"PNG":                   {pngWithTIFF(t, nil), exif.ErrUnknownFormat},
		"cut before the end":    {jpg[:len(jpg)-2], exif.ErrMalformed},
		"cut in a scan":         {jpg[:len(jpg)/2], exif.ErrMalformed},
		"cut in a segment":      {[]byte("\xFF\xD8\xFF\xE0\x00\x10JFIF"), exif.ErrMalformed},
		"a short length":        {[]byte("\xFF\xD8\xFF\xE0\x00\x01"), exif.ErrMalformed},
		"a second SOI":          {[]byte("\xFF\xD8\xFF\xD8"), exif.ErrMalformed},
		"garbage between":       {[]byte("\xFF\xD8garbage"), exif.ErrMalformed},
		"a stuffed zero alone":  {[]byte("\xFF\xD8\xFF\x00"), exif.ErrMalformed},
		"SOI only":              {jpg[:2], exif.ErrMalformed},
		"fill bytes to the end": {[]byte("\xFF\xD8\xFF\xFF\xFF"), exif.ErrMalformed},
	} {
		if err := exif.Strip(io.Discard, bytes.NewReader(c.in)); !errors.Is(err, c.err) {
			t.Errorf("%s: %v, want %v", name, err, c.err)
		}
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestStrip_AWriteErrorReachesTheCaller(t *testing.T) {
	if err := exif.Strip(failingWriter{}, bytes.NewReader(testJPEG(t, 8, 8))); err == nil || err.Error() != "disk full" {
		t.Errorf("got %v", err)
	}
}
