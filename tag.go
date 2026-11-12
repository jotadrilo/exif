package exif

import (
	"encoding/binary"
	"math"
)

// Tag is one entry of an IFD. Its value aliases the parsed input.
type Tag struct {
	IFD   IFD
	ID    uint16
	Type  Type
	Count uint32
	value []byte
	big   bool
}

// Valid reports whether the tag's type is known and its value lies inside the structure.
func (t Tag) Valid() bool { return t.value != nil || (t.Type.Size() > 0 && t.Count == 0) }

// Bytes returns the tag's raw value bytes in the file's byte order, or nil when the value is
// not readable. The slice aliases the parsed input.
func (t Tag) Bytes() []byte { return t.value }

func (t Tag) u16(b []byte) uint16 {
	if t.big {
		return binary.BigEndian.Uint16(b)
	}
	return binary.LittleEndian.Uint16(b)
}

func (t Tag) u32(b []byte) uint32 {
	if t.big {
		return binary.BigEndian.Uint32(b)
	}
	return binary.LittleEndian.Uint32(b)
}

func (t Tag) u64(b []byte) uint64 {
	if t.big {
		return binary.BigEndian.Uint64(b)
	}
	return binary.LittleEndian.Uint64(b)
}

// at returns the bytes of value i, checking the index and the data.
func (t Tag) at(i int) ([]byte, error) {
	if t.value == nil {
		if t.Type.Size() == 0 {
			return nil, ErrType
		}
		if t.Count != 0 {
			return nil, ErrMalformed
		}
	}
	if i < 0 || uint64(i) >= uint64(t.Count) {
		return nil, ErrIndex
	}
	s := t.Type.Size()
	return t.value[i*s : i*s+s], nil
}

// Int returns value i of an integer tag (BYTE, SHORT, LONG, their signed forms, IFD and the
// BigTIFF 64-bit integers), sign-extended for the signed types. A LONG8 above MaxInt64 is
// ErrMalformed.
func (t Tag) Int(i int) (int64, error) {
	if !t.Type.isInteger() {
		return 0, ErrType
	}
	b, err := t.at(i)
	if err != nil {
		return 0, err
	}
	switch t.Type {
	case Byte:
		return int64(b[0]), nil
	case SByte:
		return int64(int8(b[0])), nil
	case Short:
		return int64(t.u16(b)), nil
	case SShort:
		return int64(int16(t.u16(b))), nil
	case Long, IFDType:
		return int64(t.u32(b)), nil
	case SLong:
		return int64(int32(t.u32(b))), nil
	case SLong8:
		return int64(t.u64(b)), nil
	default: // Long8, IFD8
		v := t.u64(b)
		if v > math.MaxInt64 {
			return 0, ErrMalformed
		}
		return int64(v), nil
	}
}

// Rat returns value i of a RATIONAL or SRATIONAL tag as its numerator and denominator.
func (t Tag) Rat(i int) (num, den int64, err error) {
	if t.Type != Rational && t.Type != SRational {
		return 0, 0, ErrType
	}
	b, err := t.at(i)
	if err != nil {
		return 0, 0, err
	}
	if t.Type == Rational {
		return int64(t.u32(b)), int64(t.u32(b[4:])), nil
	}
	return int64(int32(t.u32(b))), int64(int32(t.u32(b[4:]))), nil
}

// Float returns value i of any numeric tag as a float64. A rational with a zero denominator
// is ErrZeroDenominator, never an infinity or NaN.
func (t Tag) Float(i int) (float64, error) {
	switch t.Type {
	case Rational, SRational:
		n, d, err := t.Rat(i)
		if err != nil {
			return 0, err
		}
		if d == 0 {
			return 0, ErrZeroDenominator
		}
		return float64(n) / float64(d), nil
	case Float:
		b, err := t.at(i)
		if err != nil {
			return 0, err
		}
		return float64(math.Float32frombits(t.u32(b))), nil
	case Double:
		b, err := t.at(i)
		if err != nil {
			return 0, err
		}
		return math.Float64frombits(t.u64(b)), nil
	}
	v, err := t.Int(i)
	return float64(v), err
}

// Text returns an ASCII tag's value up to its first NUL, without trailing spaces. It is the
// only accessor that allocates.
func (t Tag) Text() (string, error) {
	b, err := t.textBytes()
	return string(b), err
}

// textBytes is Text without the copy into a string.
func (t Tag) textBytes() ([]byte, error) {
	if t.Type != ASCII {
		return nil, ErrType
	}
	if t.value == nil && t.Count != 0 {
		return nil, ErrMalformed
	}
	b := t.value
	for i, c := range b {
		if c == 0 {
			b = b[:i]
			break
		}
	}
	for len(b) > 0 && b[len(b)-1] == ' ' {
		b = b[:len(b)-1]
	}
	return b, nil
}
