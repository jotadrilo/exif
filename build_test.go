package exif_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// byteOrder reads and appends in one byte order.
type byteOrder interface {
	binary.ByteOrder
	binary.AppendByteOrder
}

var (
	le byteOrder = binary.LittleEndian
	be byteOrder = binary.BigEndian
)

// entry is one IFD entry; value is in the TIFF's byte order.
type entry struct {
	id    uint16
	typ   uint16
	count uint32
	value []byte
}

// layout describes a TIFF to build. Pointer tags to the Exif and GPS IFDs and the IFD1 link
// are added when those lists are non-empty.
type layout struct {
	order byteOrder
	ifd0  []entry
	exif  []entry
	gps   []entry
	ifd1  []entry
}

func ifdSize(es []entry) int {
	n := 2 + 12*len(es) + 4
	for _, e := range es {
		if len(e.value) > 4 {
			n += len(e.value) + len(e.value)&1
		}
	}
	return n
}

// buildTIFF lays out header, IFD0, Exif IFD, GPS IFD and IFD1, each followed by its values.
func buildTIFF(l layout) []byte {
	o := l.order
	ifd0 := append([]entry(nil), l.ifd0...)
	// Pointers take one entry each; reserve them to compute offsets.
	if len(l.exif) > 0 {
		ifd0 = append(ifd0, entry{0x8769, 4, 1, o.AppendUint32(nil, 0)})
	}
	if len(l.gps) > 0 {
		ifd0 = append(ifd0, entry{0x8825, 4, 1, o.AppendUint32(nil, 0)})
	}
	at0 := 8
	atExif := at0 + ifdSize(ifd0)
	atGPS := atExif
	if len(l.exif) > 0 {
		atGPS += ifdSize(l.exif)
	}
	at1 := atGPS
	if len(l.gps) > 0 {
		at1 += ifdSize(l.gps)
	}
	for i := range ifd0 {
		switch ifd0[i].id {
		case 0x8769:
			ifd0[i].value = o.AppendUint32(nil, uint32(atExif))
		case 0x8825:
			ifd0[i].value = o.AppendUint32(nil, uint32(atGPS))
		}
	}
	mark := "II"
	if o == byteOrder(binary.BigEndian) {
		mark = "MM"
	}
	b := o.AppendUint16([]byte(mark), 42)
	b = o.AppendUint32(b, uint32(at0))
	next0 := 0
	if len(l.ifd1) > 0 {
		next0 = at1
	}
	b = appendIFD(o, b, ifd0, next0)
	if len(l.exif) > 0 {
		b = appendIFD(o, b, l.exif, 0)
	}
	if len(l.gps) > 0 {
		b = appendIFD(o, b, l.gps, 0)
	}
	if len(l.ifd1) > 0 {
		b = appendIFD(o, b, l.ifd1, 0)
	}
	return b
}

func appendIFD(o byteOrder, b []byte, es []entry, next int) []byte {
	data := len(b) + 2 + 12*len(es) + 4
	b = o.AppendUint16(b, uint16(len(es)))
	var tail []byte
	for _, e := range es {
		b = o.AppendUint16(b, e.id)
		b = o.AppendUint16(b, e.typ)
		b = o.AppendUint32(b, e.count)
		if len(e.value) <= 4 {
			v := append(append([]byte(nil), e.value...), make([]byte, 4-len(e.value))...)
			b = append(b, v...)
			continue
		}
		b = o.AppendUint32(b, uint32(data+len(tail)))
		tail = append(tail, e.value...)
		if len(e.value)&1 == 1 {
			tail = append(tail, 0)
		}
	}
	b = o.AppendUint32(b, uint32(next))
	return append(b, tail...)
}

func ascii(id uint16, s string) entry {
	return entry{id, 2, uint32(len(s) + 1), append([]byte(s), 0)}
}

func short(o byteOrder, id uint16, vs ...uint16) entry {
	var b []byte
	for _, v := range vs {
		b = o.AppendUint16(b, v)
	}
	return entry{id, 3, uint32(len(vs)), b}
}

func long(o byteOrder, id uint16, vs ...uint32) entry {
	var b []byte
	for _, v := range vs {
		b = o.AppendUint32(b, v)
	}
	return entry{id, 4, uint32(len(vs)), b}
}

// rational takes numerator, denominator pairs.
func rational(o byteOrder, id uint16, nd ...uint32) entry {
	var b []byte
	for _, v := range nd {
		b = o.AppendUint32(b, v)
	}
	return entry{id, 5, uint32(len(nd) / 2), b}
}

func byteTag(id uint16, vs ...byte) entry { return entry{id, 1, uint32(len(vs)), vs} }

// gpsEntries returns a GPS IFD for a position in whole degrees, minutes and seconds.
func gpsEntries(o byteOrder, latRef string, lat [3]uint32, lonRef string, lon [3]uint32) []entry {
	return []entry{
		ascii(0x1, latRef),
		rational(o, 0x2, lat[0], 1, lat[1], 1, lat[2], 1),
		ascii(0x3, lonRef),
		rational(o, 0x4, lon[0], 1, lon[1], 1, lon[2], 1),
	}
}

// orientationTIFF is a TIFF with IFD0 holding only an orientation SHORT.
func orientationTIFF(o byteOrder, orientation uint16) []byte {
	return buildTIFF(layout{order: o, ifd0: []entry{short(o, 0x0112, orientation)}})
}

// testJPEG encodes a real w x h JPEG.
func testJPEG(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x * 7), uint8(y * 5), 90, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func segment(marker byte, payload []byte) []byte {
	s := []byte{0xFF, marker}
	s = binary.BigEndian.AppendUint16(s, uint16(len(payload)+2))
	return append(s, payload...)
}

// withSegments inserts segments right after a JPEG's SOI.
func withSegments(jpg []byte, segs ...[]byte) []byte {
	out := append([]byte(nil), jpg[:2]...)
	for _, s := range segs {
		out = append(out, s...)
	}
	return append(out, jpg[2:]...)
}

func exifSegment(tiff []byte) []byte {
	return segment(0xE1, append([]byte("Exif\x00\x00"), tiff...))
}

// jpegWithTIFF is a real JPEG with an EXIF APP1 holding tiff.
func jpegWithTIFF(t testing.TB, tiff []byte) []byte {
	return withSegments(testJPEG(t, 16, 8), exifSegment(tiff))
}

func pngChunk(kind string, data []byte) []byte {
	c := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	c = append(c, kind...)
	c = append(c, data...)
	return binary.BigEndian.AppendUint32(c, crc32.ChecksumIEEE(c[4:]))
}

// pngWithTIFF is a real PNG with an eXIf chunk right after IHDR.
func pngWithTIFF(t testing.TB, tiff []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	ihdrEnd := 8 + 12 + 13
	out := append([]byte(nil), b[:ihdrEnd]...)
	if tiff != nil {
		out = append(out, pngChunk("eXIf", tiff)...)
	}
	return append(out, b[ihdrEnd:]...)
}

func webpChunk(kind string, data []byte) []byte {
	c := append([]byte(kind), binary.LittleEndian.AppendUint32(nil, uint32(len(data)))...)
	c = append(c, data...)
	if len(data)&1 == 1 {
		c = append(c, 0)
	}
	return c
}

// lossless1x1 is the VP8L bitstream of one opaque pixel.
var lossless1x1 = []byte{0x2f, 0x00, 0x00, 0x00, 0x00, 0x07, 0x50, 0xc0, 0x42, 0x14, 0xb8, 0xff, 0x81, 0x88, 0xe8, 0x7f, 0x00}

// webpWithTIFF is an extended WebP: VP8X, the image, then an EXIF chunk.
func webpWithTIFF(tiff []byte) []byte {
	vp8x := []byte{0x08, 0, 0, 0, 0, 0, 0, 0, 0, 0} // EXIF flag, 1x1
	body := append([]byte("WEBP"), webpChunk("VP8X", vp8x)...)
	body = append(body, webpChunk("VP8L", lossless1x1)...)
	if tiff != nil {
		body = append(body, webpChunk("EXIF", tiff)...)
	}
	out := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(len(body)))...)
	return append(out, body...)
}
