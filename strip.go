package exif

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"sync"
)

// StripOption changes what Strip keeps.
type StripOption func(*stripConfig)

type stripConfig struct {
	orientation bool
	icc         bool
}

// KeepOrientation makes Strip replace the first EXIF segment with a minimal one holding its
// orientation alone, so that viewers still show the photo upright. Without it a rotated
// camera photo loses its orientation and shows sideways, unless its pixels are turned first.
// An orientation of 1, or none, writes no segment.
func KeepOrientation() StripOption { return func(c *stripConfig) { c.orientation = true } }

// KeepICCProfile makes Strip keep the APP2 ICC color profile, which wide-gamut photos (such
// as Display P3) need to show their colors. A profile can name the device or software that
// made it.
func KeepICCProfile() StripOption { return func(c *stripConfig) { c.icc = true } }

// readerPool and writerPool reuse Strip's buffers. The reader holds a whole segment (at most
// 64 KiB plus its marker) so that segments are read in place.
var (
	readerPool = sync.Pool{New: func() any { return bufio.NewReaderSize(nil, 1<<17) }}
	writerPool = sync.Pool{New: func() any { return bufio.NewWriterSize(nil, 1<<16) }}
)

// Strip copies a JPEG from src to dst without its metadata: every APP segment (JFIF, EXIF,
// XMP, ICC profiles, Photoshop IRB, MPF, maker data), comments, JPGn and reserved segments,
// and anything after the end of the image. The frames, tables and scans are copied byte for
// byte, never decoded or encoded again, so the image is unchanged. The Adobe APP14 segment
// stays, since it decides how a decoder turns the stored components into colors.
//
// EXIF orientation is metadata too: pass KeepOrientation to keep it, or turn the pixels
// upright before encoding the image again. ICC profiles go unless KeepICCProfile is passed.
//
// Strip streams with bounded memory (about 192 KiB of pooled buffers) in time linear in the
// input. Input that is not a JPEG is ErrUnknownFormat; a JPEG that breaks the format before
// its end is ErrMalformed. On error, dst may hold part of the output.
func Strip(dst io.Writer, src io.Reader, opts ...StripOption) error {
	var c stripConfig
	for _, o := range opts {
		o(&c)
	}
	br := readerPool.Get().(*bufio.Reader)
	bw := writerPool.Get().(*bufio.Writer)
	br.Reset(src)
	bw.Reset(dst)
	defer func() {
		br.Reset(nil)
		bw.Reset(nil)
		readerPool.Put(br)
		writerPool.Put(bw)
	}()
	if err := strip(bw, br, c); err != nil {
		return err
	}
	return bw.Flush()
}

func strip(bw *bufio.Writer, br *bufio.Reader, c stripConfig) error {
	head, err := br.Peek(2)
	if err != nil || head[0] != 0xFF || head[1] != jpegSOI {
		return ErrUnknownFormat
	}
	_, _ = br.Discard(2)
	_, _ = bw.Write([]byte{0xFF, jpegSOI})
	exifSeen := false
	var m byte
	for {
		if m == 0 {
			if m, err = nextMarker(br); err != nil {
				return err
			}
		}
		switch {
		case m == jpegEOI:
			_, err := bw.Write([]byte{0xFF, jpegEOI})
			return err
		case isStandalone(m):
			_, _ = bw.Write([]byte{0xFF, m})
			m = 0
			continue
		case m == jpegSOI || m == 0x00:
			return ErrMalformed
		}
		l, err := br.Peek(2)
		if err != nil {
			return ErrMalformed
		}
		n := int(binary.BigEndian.Uint16(l))
		if n < 2 {
			return ErrMalformed
		}
		seg, err := br.Peek(n)
		if err != nil {
			return ErrMalformed
		}
		payload := seg[2:]
		if m == jpegAPP1 && !exifSeen && bytes.HasPrefix(payload, []byte(exifHeader)) {
			exifSeen = true
			if c.orientation {
				x, _ := ParseTIFF(payload[len(exifHeader):])
				if o := x.Orientation(); o != 1 {
					_, _ = bw.Write(orientationSegment(o))
				}
			}
		}
		if keepSegment(m, payload, c) {
			_, _ = bw.Write([]byte{0xFF, m})
			if _, err := bw.Write(seg); err != nil {
				return err
			}
		}
		_, _ = br.Discard(n)
		if m == jpegSOS {
			if m, err = copyScan(bw, br); err != nil {
				return err
			}
			continue
		}
		m = 0
	}
}

// nextMarker reads a marker, past any 0xFF fill bytes.
func nextMarker(br *bufio.Reader) (byte, error) {
	b, err := br.ReadByte()
	if err != nil || b != 0xFF {
		return 0, ErrMalformed
	}
	for b == 0xFF {
		if b, err = br.ReadByte(); err != nil {
			return 0, ErrMalformed
		}
	}
	return b, nil
}

// copyScan copies a scan's entropy-coded data, where 0xFF appears only stuffed (0xFF00) or as
// a restart marker, and returns the marker that ends it.
func copyScan(bw *bufio.Writer, br *bufio.Reader) (byte, error) {
	for {
		chunk, err := br.ReadSlice(0xFF)
		if errors.Is(err, bufio.ErrBufferFull) {
			if _, err := bw.Write(chunk); err != nil {
				return 0, err
			}
			continue
		}
		if err != nil {
			return 0, ErrMalformed
		}
		if _, err := bw.Write(chunk[:len(chunk)-1]); err != nil {
			return 0, err
		}
		for {
			b, err := br.ReadByte()
			if err != nil {
				return 0, ErrMalformed
			}
			if b == 0xFF {
				continue // fill byte before a marker
			}
			if b == 0x00 || (b >= 0xD0 && b <= 0xD7) {
				_, _ = bw.Write([]byte{0xFF, b})
				break
			}
			return b, nil
		}
	}
}

// keepSegment reports whether a segment shows the image: frames, tables, scans and the Adobe
// color transform, plus the ICC profile when asked.
func keepSegment(m byte, payload []byte, c stripConfig) bool {
	switch {
	case m >= 0xC0 && m <= 0xCF:
		return m != 0xC8 // JPG, reserved for extensions
	case m >= 0xDA && m <= 0xDF:
		return true // SOS, DQT, DNL, DRI, DHP, EXP
	case m == 0xEE:
		return bytes.HasPrefix(payload, []byte("Adobe"))
	case m == 0xE2:
		return c.icc && bytes.HasPrefix(payload, []byte("ICC_PROFILE\x00"))
	}
	return false
}

// orientationSegment is an APP1 whose EXIF holds the orientation and nothing else.
func orientationSegment(o int) []byte {
	seg := []byte("\xFF\xE1\x00\x22Exif\x00\x00MM\x00\x2a\x00\x00\x00\x08" +
		"\x00\x01\x01\x12\x00\x03\x00\x00\x00\x01\x00\x00\x00\x00\x00\x00\x00\x00")
	seg[len(seg)-7] = byte(o)
	return seg
}
