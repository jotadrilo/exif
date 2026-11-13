package exif

import (
	"encoding/binary"
	"iter"
)

const (
	tiffHeaderSize = 8
	ifdEntrySize   = 12
	exifHeader     = "Exif\x00\x00"
)

// Exif is the EXIF metadata of one image: a TIFF structure read in place, without copying it.
//
// Nothing is decoded up front. Each accessor walks only the directories it needs, so reading
// the orientation touches IFD0 alone and reading GPS touches IFD0 and the GPS IFD. An Exif
// aliases the bytes it was parsed from: they must not change while it is in use.
//
// The zero Exif holds no tags; its Orientation is 1 and its other accessors report absence,
// so the result of a failed Parse or Decode can be used without a check.
type Exif struct {
	tiff []byte
	big  bool
	ifd0 uint32
}

// ParseTIFF reads a TIFF structure ("II*\x00" or "MM\x00*"), optionally preceded by the
// "Exif\x00\x00" header of a JPEG APP1 segment. The returned Exif aliases b.
func ParseTIFF(b []byte) (Exif, error) {
	if len(b) >= len(exifHeader) && string(b[:len(exifHeader)]) == exifHeader {
		b = b[len(exifHeader):]
	}
	if len(b) < tiffHeaderSize {
		return Exif{}, ErrMalformed
	}
	var x Exif
	switch {
	case b[0] == 'I' && b[1] == 'I':
	case b[0] == 'M' && b[1] == 'M':
		x.big = true
	default:
		return Exif{}, ErrMalformed
	}
	x.tiff = b
	if x.u16(b[2:]) != 42 {
		return Exif{}, ErrMalformed
	}
	off := x.u32(b[4:])
	if !x.validIFD(off) {
		return Exif{}, ErrMalformed
	}
	x.ifd0 = off
	return x, nil
}

// ByteOrder returns the byte order of the TIFF structure.
func (x Exif) ByteOrder() binary.ByteOrder {
	if x.big {
		return binary.BigEndian
	}
	return binary.LittleEndian
}

// Raw returns the TIFF structure the Exif reads, which aliases the parsed input.
func (x Exif) Raw() []byte { return x.tiff }

func (x Exif) u16(b []byte) uint16 {
	if x.big {
		return binary.BigEndian.Uint16(b)
	}
	return binary.LittleEndian.Uint16(b)
}

func (x Exif) u32(b []byte) uint32 {
	if x.big {
		return binary.BigEndian.Uint32(b)
	}
	return binary.LittleEndian.Uint32(b)
}

// validIFD reports whether an IFD's entry count lies inside the structure, past the header.
func (x Exif) validIFD(off uint32) bool {
	return off >= tiffHeaderSize && uint64(off)+2 <= uint64(len(x.tiff))
}

// entries returns the complete entries of the IFD at off. A directory cut short yields the
// entries that fit, so a truncated file still gives what it holds.
func (x Exif) entries(off uint32) []byte {
	if !x.validIFD(off) {
		return nil
	}
	start := int(off) + 2
	n := min(int(x.u16(x.tiff[off:])), (len(x.tiff)-start)/ifdEntrySize)
	return x.tiff[start : start+n*ifdEntrySize]
}

// find returns the tag id of the IFD at off. When an IFD repeats a tag, the last one wins.
func (x Exif) find(dir IFD, off uint32, id uint16) (Tag, bool) {
	raw := x.entries(off)
	at := -1
	for i := 0; i+ifdEntrySize <= len(raw); i += ifdEntrySize {
		if x.u16(raw[i:]) == id {
			at = i
		}
	}
	if at < 0 {
		return Tag{}, false
	}
	return x.tag(dir, raw[at:at+ifdEntrySize]), true
}

// tag decodes one 12-byte entry. A value outside the structure leaves the tag without data:
// its accessors return ErrMalformed.
func (x Exif) tag(dir IFD, e []byte) Tag {
	t := Tag{IFD: dir, ID: x.u16(e), Type: Type(x.u16(e[2:])), Count: x.u32(e[4:]), big: x.big}
	size := uint64(t.Type.Size()) * uint64(t.Count)
	switch {
	case t.Type.Size() == 0:
	case size <= 4:
		t.value = e[8 : 8+size]
	default:
		off := uint64(x.u32(e[8:]))
		if off+size <= uint64(len(x.tiff)) {
			t.value = x.tiff[off : off+size]
		}
	}
	return t
}

// pointer reads a sub-IFD offset from a tag, refusing one that leaves the structure or points
// back at the directory holding it.
func (x Exif) pointer(off uint32, id uint16) (uint32, bool) {
	raw := x.entries(off)
	at := -1
	for i := 0; i+ifdEntrySize <= len(raw); i += ifdEntrySize {
		if x.u16(raw[i:]) == id {
			at = i
		}
	}
	if at < 0 {
		return 0, false
	}
	// Read in place: a pointer is one LONG or IFD (or, rarely, a SHORT), held in the entry. Any
	// other count is malformed.
	e := raw[at : at+ifdEntrySize]
	if x.u32(e[4:]) != 1 {
		return 0, false
	}
	var p uint32
	switch Type(x.u16(e[2:])) {
	case Long, IFDType:
		p = x.u32(e[8:])
	case Short:
		p = uint32(x.u16(e[8:]))
	default:
		return 0, false
	}
	if p == off || p == x.ifd0 || !x.validIFD(p) {
		return 0, false
	}
	return p, true
}

// next returns the offset of the IFD chained after the one at off.
func (x Exif) next(off uint32) (uint32, bool) {
	if !x.validIFD(off) {
		return 0, false
	}
	at := uint64(off) + 2 + uint64(x.u16(x.tiff[off:]))*ifdEntrySize
	if at+4 > uint64(len(x.tiff)) {
		return 0, false
	}
	p := x.u32(x.tiff[at:])
	if p == 0 || p == off || p == x.ifd0 || !x.validIFD(p) {
		return 0, false
	}
	return p, true
}

// offset returns where an IFD starts.
func (x Exif) offset(dir IFD) (uint32, bool) {
	if x.tiff == nil {
		return 0, false
	}
	switch dir {
	case IFD0:
		return x.ifd0, true
	case ExifIFD:
		return x.pointer(x.ifd0, TagExifIFDPointer)
	case GPSIFD:
		return x.pointer(x.ifd0, TagGPSIFDPointer)
	case InteropIFD:
		e, ok := x.pointer(x.ifd0, TagExifIFDPointer)
		if !ok {
			return 0, false
		}
		return x.pointer(e, TagInteropIFDPointer)
	case IFD1:
		return x.next(x.ifd0)
	}
	return 0, false
}

// Tag returns the tag id of a directory, and false when the directory or the tag is absent.
func (x Exif) Tag(dir IFD, id uint16) (Tag, bool) {
	off, ok := x.offset(dir)
	if !ok {
		return Tag{}, false
	}
	return x.find(dir, off, id)
}

// Tags yields every tag of IFD0, the Exif IFD, the GPS IFD, the Interop IFD and IFD1, in that
// order and as stored. A directory reached twice, through a loop or a shared offset, is
// yielded once.
func (x Exif) Tags() iter.Seq[Tag] {
	return func(yield func(Tag) bool) {
		var seen [5]uint32
		n := 0
		for _, dir := range [...]IFD{IFD0, ExifIFD, GPSIFD, InteropIFD, IFD1} {
			off, ok := x.offset(dir)
			if !ok {
				continue
			}
			dup := false
			for _, s := range seen[:n] {
				dup = dup || s == off
			}
			if dup {
				continue
			}
			seen[n] = off
			n++
			raw := x.entries(off)
			for i := 0; i+ifdEntrySize <= len(raw); i += ifdEntrySize {
				if !yield(x.tag(dir, raw[i:i+ifdEntrySize])) {
					return
				}
			}
		}
	}
}
