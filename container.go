package exif

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

const (
	pngSignature = "\x89PNG\r\n\x1a\n"
	jpegAPP1     = 0xE1
	jpegSOS      = 0xDA
	jpegEOI      = 0xD9
	jpegSOI      = 0xD8
)

// Parse finds and reads the EXIF metadata of a JPEG, TIFF, PNG or WebP image, or of a raw
// EXIF block, held in memory. It reads only the metadata: a JPEG is walked up to its EXIF
// segment, a PNG or WebP chunk by chunk without touching the pixel data. It does not
// allocate, and the returned Exif aliases data.
func Parse(data []byte) (Exif, error) {
	tiff, err := locate(data)
	if err != nil {
		return Exif{}, err
	}
	return ParseTIFF(tiff)
}

// locate returns the TIFF structure inside an image.
func locate(b []byte) ([]byte, error) {
	switch {
	case len(b) >= 2 && b[0] == 0xFF && b[1] == jpegSOI:
		return jpegExif(b)
	case len(b) >= 4 && (string(b[:4]) == "II*\x00" || string(b[:4]) == "MM\x00*"):
		return b, nil
	case len(b) >= len(pngSignature) && string(b[:len(pngSignature)]) == pngSignature:
		return pngExif(b)
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return webpExif(b)
	case len(b) >= len(exifHeader) && string(b[:len(exifHeader)]) == exifHeader:
		return b[len(exifHeader):], nil
	}
	return nil, ErrUnknownFormat
}

// isStandalone reports a JPEG marker without a length: TEM and the restart markers.
func isStandalone(m byte) bool { return m == 0x01 || (m >= 0xD0 && m <= 0xD7) }

// jpegExif walks the segments before the first scan and returns the first EXIF APP1's TIFF
// structure. An EXIF segment cut short by the end of the file yields what it holds.
func jpegExif(b []byte) ([]byte, error) {
	i := 2
	for {
		if i >= len(b) {
			return nil, ErrNoExif
		}
		if b[i] != 0xFF {
			return nil, ErrMalformed
		}
		for i < len(b) && b[i] == 0xFF {
			i++
		}
		if i >= len(b) {
			return nil, ErrMalformed
		}
		m := b[i]
		i++
		switch {
		case isStandalone(m):
			continue
		case m == jpegSOS || m == jpegEOI:
			return nil, ErrNoExif
		case m == jpegSOI || m == 0x00:
			return nil, ErrMalformed
		}
		if i+2 > len(b) {
			return nil, ErrMalformed
		}
		n := int(binary.BigEndian.Uint16(b[i:]))
		if n < 2 {
			return nil, ErrMalformed
		}
		seg := b[i+2 : min(i+n, len(b))]
		if m == jpegAPP1 && bytes.HasPrefix(seg, []byte(exifHeader)) {
			return seg[len(exifHeader):], nil
		}
		if i+n > len(b) {
			return nil, ErrMalformed
		}
		i += n
	}
}

// pngExif returns the eXIf chunk's data.
func pngExif(b []byte) ([]byte, error) {
	i := len(pngSignature)
	for {
		if i+8 > len(b) {
			return nil, ErrMalformed
		}
		n := uint64(binary.BigEndian.Uint32(b[i:]))
		kind := string(b[i+4 : i+8])
		end := uint64(i) + 12 + n
		if kind == "eXIf" {
			if uint64(i)+8+n > uint64(len(b)) {
				return nil, ErrMalformed
			}
			return b[i+8 : i+8+int(n)], nil
		}
		if kind == "IEND" {
			return nil, ErrNoExif
		}
		if end > uint64(len(b)) {
			return nil, ErrMalformed
		}
		i = int(end)
	}
}

// webpExif returns the EXIF chunk's data of an extended WebP. A simple WebP, whose first chunk
// is not VP8X, cannot hold metadata.
func webpExif(b []byte) ([]byte, error) {
	end := min(uint64(binary.LittleEndian.Uint32(b[4:]))+8, uint64(len(b)))
	i := uint64(12)
	first := true
	for i+8 <= end {
		kind := string(b[i : i+4])
		n := uint64(binary.LittleEndian.Uint32(b[i+4:]))
		if first && kind != "VP8X" {
			return nil, ErrNoExif
		}
		first = false
		if i+8+n > end {
			return nil, ErrMalformed
		}
		if kind == "EXIF" {
			return b[i+8 : i+8+n], nil
		}
		i += 8 + n + n&1
	}
	if first {
		return nil, ErrMalformed
	}
	return nil, ErrNoExif
}

// Decoder reads EXIF metadata from a stream within limits. Its zero value uses the defaults.
type Decoder struct {
	// MaxMetadata caps the EXIF block of a PNG or WebP, or a raw EXIF stream. A JPEG segment
	// is at most 64 KiB by format. Default 4 MiB.
	MaxMetadata int64
	// MaxTIFF caps a TIFF file, which is read whole since its directories may point
	// anywhere. Default 64 MiB.
	MaxTIFF int64
	// MaxScan caps the bytes skipped while looking for the metadata, which bounds the time
	// spent on a huge or endless stream. Default 1 GiB.
	MaxScan int64
}

const (
	defaultMaxMetadata = 4 << 20
	defaultMaxTIFF     = 64 << 20
	defaultMaxScan     = 1 << 30
)

// Decode reads the EXIF metadata of an image stream with the default limits.
//
// Only the metadata is read: a JPEG up to its EXIF segment, a PNG or WebP chunk by chunk,
// skipping pixel data with Seek when r is an io.Seeker (an *os.File reads a few kilobytes of a
// large photo) or by discarding it otherwise. Prefer Parse when the image is already in
// memory: it does not allocate.
func Decode(r io.Reader) (Exif, error) { return Decoder{}.Decode(r) }

// Decode reads the EXIF metadata of an image stream within d's limits.
func (d Decoder) Decode(r io.Reader) (Exif, error) {
	st := stream{r: r, left: d.MaxScan}
	if st.left <= 0 {
		st.left = defaultMaxScan
	}
	if sk, ok := r.(io.Seeker); ok {
		st.seeker = sk
		st.size = -1
		if cur, err := sk.Seek(0, io.SeekCurrent); err == nil {
			if end, err := sk.Seek(0, io.SeekEnd); err == nil {
				if _, err := sk.Seek(cur, io.SeekStart); err != nil {
					return Exif{}, err
				}
				st.size = end - cur
			}
		}
	}
	maxMeta := d.MaxMetadata
	if maxMeta <= 0 {
		maxMeta = defaultMaxMetadata
	}
	maxTIFF := d.MaxTIFF
	if maxTIFF <= 0 {
		maxTIFF = defaultMaxTIFF
	}

	head, err := st.read(2)
	if err != nil {
		return Exif{}, unknownIfShort(err)
	}
	if head[0] == 0xFF && head[1] == jpegSOI {
		tiff, err := st.jpeg()
		if err != nil {
			return Exif{}, err
		}
		return ParseTIFF(tiff)
	}
	st.unread(head)
	if head, err = st.read(4); err != nil {
		return Exif{}, unknownIfShort(err)
	}
	var tiff []byte
	switch {
	case string(head) == "II*\x00" || string(head) == "MM\x00*":
		tiff, err = st.all(head, maxTIFF)
	case string(head) == pngSignature[:4]:
		tiff, err = st.png(maxMeta)
	case string(head) == "RIFF":
		tiff, err = st.webp(maxMeta)
	case string(head) == exifHeader[:4]:
		tiff, err = st.all(head, maxMeta)
	default:
		return Exif{}, ErrUnknownFormat
	}
	if err != nil {
		return Exif{}, err
	}
	return ParseTIFF(tiff)
}

func unknownIfShort(err error) error {
	if errors.Is(err, ErrMalformed) {
		return ErrUnknownFormat
	}
	return err
}

// stream reads a container sequentially.
type stream struct {
	r      io.Reader
	seeker io.Seeker
	buf    [16]byte
	pre    []byte // bytes read ahead, returned before r's
	left   int64  // bytes that may still be skipped
	pos    int64  // bytes consumed
	size   int64  // the stream's length when a Seeker tells it, else 0 or -1
}

func (s *stream) unread(b []byte) { s.pre = append(s.buf[8:8], b...) }

// read returns the next n bytes (n <= 8), valid until the next call.
func (s *stream) read(n int) ([]byte, error) {
	got := copy(s.buf[:n], s.pre)
	s.pre = s.pre[got:]
	if got < n {
		m, err := io.ReadFull(s.r, s.buf[got:n])
		s.pos += int64(m)
		if err != nil {
			return nil, ioError(err)
		}
	}
	return s.buf[:n], nil
}

func ioError(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return ErrMalformed
	}
	return err
}

func (s *stream) skip(n int64) error {
	if s.size > 0 && n > s.size-s.pos {
		return ErrMalformed
	}
	if n > s.left {
		return ErrTooLarge
	}
	s.left -= n
	s.pos += n
	if s.seeker != nil {
		if _, err := s.seeker.Seek(n, io.SeekCurrent); err != nil {
			return err
		}
		return nil
	}
	m, err := io.CopyN(io.Discard, s.r, n)
	if m < n {
		if err == nil || errors.Is(err, io.EOF) {
			return ErrMalformed
		}
		return err
	}
	return nil
}

// payload reads the next n bytes into a new slice.
func (s *stream) payload(n int64) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(s.r, b); err != nil {
		return nil, ioError(err)
	}
	return b, nil
}

// truncatable reads up to n bytes, accepting fewer when the stream ends first, as Parse does
// for an EXIF segment cut short.
func (s *stream) truncatable(n int64) ([]byte, error) {
	b := make([]byte, n)
	got, err := io.ReadFull(s.r, b)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return b[:got], nil
}

// all reads the rest of the stream after head, up to limit bytes in all.
func (s *stream) all(head []byte, limit int64) ([]byte, error) {
	b := append(make([]byte, 0, 4096), head...)
	for {
		if int64(len(b)) > limit {
			return nil, ErrTooLarge
		}
		if len(b) == cap(b) {
			b = append(b, 0)[:len(b)]
		}
		want := min(int64(cap(b)), limit+1)
		n, err := s.r.Read(b[len(b):want])
		b = b[:len(b)+n]
		if errors.Is(err, io.EOF) {
			if int64(len(b)) > limit {
				return nil, ErrTooLarge
			}
			return b, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func (s *stream) jpeg() ([]byte, error) {
	for {
		b, err := s.read(1)
		if err != nil {
			if errors.Is(err, ErrMalformed) {
				return nil, ErrNoExif
			}
			return nil, err
		}
		if b[0] != 0xFF {
			return nil, ErrMalformed
		}
		for b[0] == 0xFF {
			if b, err = s.read(1); err != nil {
				return nil, err
			}
		}
		m := b[0]
		switch {
		case isStandalone(m):
			continue
		case m == jpegSOS || m == jpegEOI:
			return nil, ErrNoExif
		case m == jpegSOI || m == 0x00:
			return nil, ErrMalformed
		}
		if b, err = s.read(2); err != nil {
			return nil, err
		}
		n := int64(binary.BigEndian.Uint16(b)) - 2
		if n < 0 {
			return nil, ErrMalformed
		}
		if m == jpegAPP1 && n >= int64(len(exifHeader)) {
			if b, err = s.read(len(exifHeader)); err != nil {
				return nil, err
			}
			n -= int64(len(exifHeader))
			if string(b) == exifHeader {
				return s.truncatable(n)
			}
		}
		if err := s.skip(n); err != nil {
			return nil, err
		}
	}
}

func (s *stream) png(limit int64) ([]byte, error) {
	b, err := s.read(len(pngSignature) - 4)
	if err != nil {
		return nil, unknownIfShort(err)
	}
	if string(b) != pngSignature[4:] {
		return nil, ErrUnknownFormat
	}
	for {
		if b, err = s.read(8); err != nil {
			return nil, err
		}
		n := int64(binary.BigEndian.Uint32(b))
		switch string(b[4:8]) {
		case "eXIf":
			if n > limit {
				return nil, ErrTooLarge
			}
			return s.payload(n)
		case "IEND":
			return nil, ErrNoExif
		}
		if err := s.skip(n + 4); err != nil {
			return nil, err
		}
	}
}

func (s *stream) webp(limit int64) ([]byte, error) {
	b, err := s.read(8)
	if err != nil {
		return nil, unknownIfShort(err)
	}
	if string(b[4:8]) != "WEBP" {
		return nil, ErrUnknownFormat
	}
	left := int64(binary.LittleEndian.Uint32(b)) - 4
	first := true
	for left >= 8 {
		if b, err = s.read(8); err != nil {
			return nil, err
		}
		left -= 8
		n := int64(binary.LittleEndian.Uint32(b[4:]))
		kind := string(b[:4])
		if first && kind != "VP8X" {
			return nil, ErrNoExif
		}
		first = false
		if n > left {
			return nil, ErrMalformed
		}
		if kind == "EXIF" {
			if n > limit {
				return nil, ErrTooLarge
			}
			return s.payload(n)
		}
		padded := min(n+n&1, left)
		if err := s.skip(padded); err != nil {
			return nil, err
		}
		left -= padded
	}
	if first {
		return nil, ErrMalformed
	}
	return nil, ErrNoExif
}
