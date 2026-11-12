package exif

import (
	"math"
	"time"
)

// Orientation returns the EXIF orientation of the main image, 1 through 8, or 1 (as stored)
// when the tag is absent, unreadable, not an integer, or out of range. When IFD0 repeats the
// tag, the last one wins; when it holds several values, the first one counts.
//
//	1 normal            5 transpose (mirror + 270° clockwise)
//	2 mirror horizontal 6 rotate 90° clockwise
//	3 rotate 180°       7 transverse (mirror + 90° clockwise)
//	4 mirror vertical   8 rotate 270° clockwise
//
// Orientations 5 through 8 swap the width and the height.
func (x Exif) Orientation() int {
	t, ok := x.Tag(IFD0, TagOrientation)
	if !ok {
		return 1
	}
	v, err := t.Int(0)
	if err != nil || v < 1 || v > 8 {
		return 1
	}
	return int(v)
}

// Make returns the camera maker, or "" when absent.
func (x Exif) Make() string { return x.text(IFD0, TagMake) }

// Model returns the camera model, or "" when absent.
func (x Exif) Model() string { return x.text(IFD0, TagModel) }

func (x Exif) text(dir IFD, id uint16) string {
	t, ok := x.Tag(dir, id)
	if !ok {
		return ""
	}
	s, _ := t.Text()
	return s
}

// Dimensions returns the pixel size the metadata states: PixelXDimension and PixelYDimension
// of the Exif IFD, or else ImageWidth and ImageLength of IFD0. It is what the camera wrote,
// as stored (before orientation), and an editor may have left it stale: decode the image
// header for the authoritative size.
func (x Exif) Dimensions() (width, height int, ok bool) {
	if off, found := x.offset(ExifIFD); found {
		w, okW := x.positive(ExifIFD, off, TagPixelXDimension)
		h, okH := x.positive(ExifIFD, off, TagPixelYDimension)
		if okW && okH {
			return w, h, true
		}
	}
	if x.tiff == nil {
		return 0, 0, false
	}
	w, okW := x.positive(IFD0, x.ifd0, TagImageWidth)
	h, okH := x.positive(IFD0, x.ifd0, TagImageLength)
	if okW && okH {
		return w, h, true
	}
	return 0, 0, false
}

func (x Exif) positive(dir IFD, off uint32, id uint16) (int, bool) {
	t, ok := x.find(dir, off, id)
	if !ok {
		return 0, false
	}
	v, err := t.Int(0)
	if err != nil || v <= 0 || v > math.MaxInt32 {
		return 0, false
	}
	return int(v), true
}

// GPS is a position in decimal degrees, south and west negative.
type GPS struct {
	Latitude  float64
	Longitude float64
	// Altitude is meters above sea level, negative below it, when HasAltitude is true.
	Altitude    float64
	HasAltitude bool
}

// GPS returns the position the GPS IFD records. It reports false unless the latitude, the
// longitude and both references (N/S, E/W) are present and readable, and also for any
// position that cannot be a real fix: a zero denominator, a negative component, a value out
// of range, a GPSStatus of "V" (measurement void), or exactly (0, 0), the "Null Island"
// placeholder devices write without a fix. An unreadable altitude only clears HasAltitude.
func (x Exif) GPS() (GPS, bool) {
	off, ok := x.offset(GPSIFD)
	if !ok {
		return GPS{}, false
	}
	var latRef, lonRef, altRef, status, lat, lon, alt Tag
	var have uint8
	raw, _ := x.entries(off)
	for i := 0; i+ifdEntrySize <= len(raw); i += ifdEntrySize {
		e := raw[i : i+ifdEntrySize]
		switch x.u16(e) {
		case TagGPSLatitudeRef:
			latRef, have = x.tag(GPSIFD, e), have|1
		case TagGPSLatitude:
			lat, have = x.tag(GPSIFD, e), have|2
		case TagGPSLongitudeRef:
			lonRef, have = x.tag(GPSIFD, e), have|4
		case TagGPSLongitude:
			lon, have = x.tag(GPSIFD, e), have|8
		case TagGPSAltitudeRef:
			altRef, have = x.tag(GPSIFD, e), have|16
		case TagGPSAltitude:
			alt, have = x.tag(GPSIFD, e), have|32
		case TagGPSStatus:
			status, have = x.tag(GPSIFD, e), have|64
		}
	}
	if have&15 != 15 {
		return GPS{}, false
	}
	if have&64 != 0 {
		if s := firstLetter(status); s == 'V' {
			return GPS{}, false
		}
	}
	var g GPS
	g.Latitude, ok = degrees(lat, latRef, 'N', 'S', 90)
	if !ok {
		return GPS{}, false
	}
	g.Longitude, ok = degrees(lon, lonRef, 'E', 'W', 180)
	if !ok || (g.Latitude == 0 && g.Longitude == 0) {
		return GPS{}, false
	}
	if have&32 != 0 {
		if a, err := alt.Float(0); err == nil && a >= 0 && !math.IsInf(a, 0) {
			g.Altitude, g.HasAltitude = a, true
			if have&16 != 0 {
				if r, err := altRef.Int(0); err == nil && r == 1 {
					g.Altitude = -a
				}
			}
		}
	}
	return g, true
}

// firstLetter returns the upper-cased first byte of an ASCII tag, or 0.
func firstLetter(t Tag) byte {
	b, err := t.textBytes()
	if err != nil || len(b) == 0 {
		return 0
	}
	c := b[0]
	if c >= 'a' && c <= 'z' {
		c -= 'a' - 'A'
	}
	return c
}

// degrees turns a degrees, minutes, seconds triple (or fewer values) and its reference into
// signed decimal degrees no larger than limit.
func degrees(v, ref Tag, pos, neg byte, limit float64) (float64, bool) {
	sign := 0.0
	switch firstLetter(ref) {
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
	d := 0.0
	scale := 1.0
	for i := 0; i < 3 && uint64(i) < uint64(v.Count); i++ {
		f, err := v.Float(i)
		if err != nil || f < 0 || math.IsInf(f, 0) || math.IsNaN(f) {
			return 0, false
		}
		d += f / scale
		scale *= 60
	}
	if d > limit {
		return 0, false
	}
	return sign * d, true
}

// DateTimeOriginal returns when the photo was taken, from DateTimeOriginal and
// SubSecTimeOriginal of the Exif IFD. With an OffsetTimeOriginal the time carries that zone
// and hasOffset is true; without one, the camera's wall-clock reading is returned in UTC and
// hasOffset is false, so a caller can place it in the zone it knows. ok is false when the tag
// is absent, blank or not a valid date.
func (x Exif) DateTimeOriginal() (t time.Time, hasOffset, ok bool) {
	off, found := x.offset(ExifIFD)
	if !found {
		return time.Time{}, false, false
	}
	var dt, sub, zone Tag
	var have uint8
	raw, _ := x.entries(off)
	for i := 0; i+ifdEntrySize <= len(raw); i += ifdEntrySize {
		e := raw[i : i+ifdEntrySize]
		switch x.u16(e) {
		case TagDateTimeOriginal:
			dt, have = x.tag(ExifIFD, e), have|1
		case TagSubSecTimeOriginal:
			sub, have = x.tag(ExifIFD, e), have|2
		case TagOffsetTimeOriginal:
			zone, have = x.tag(ExifIFD, e), have|4
		}
	}
	if have&1 == 0 {
		return time.Time{}, false, false
	}
	s, err := dt.textBytes()
	if err != nil {
		return time.Time{}, false, false
	}
	var nsec int
	if have&2 != 0 {
		nsec = subSeconds(sub)
	}
	loc := time.UTC
	if have&4 != 0 {
		if secs, ok := zoneOffset(zone); ok {
			hasOffset = true
			if secs != 0 {
				loc = time.FixedZone("", secs)
			}
		}
	}
	t, ok = parseDateTime(s, nsec, loc)
	if !ok {
		return time.Time{}, false, false
	}
	return t, hasOffset, true
}

// parseDateTime reads an EXIF date and time, "YYYY:MM:DD HH:MM:SS" (any single separator is
// accepted), adding nsec nanoseconds in loc. It reports false for a blank, zeroed or invalid
// date.
func parseDateTime(s []byte, nsec int, loc *time.Location) (time.Time, bool) {
	if len(s) < 19 {
		return time.Time{}, false
	}
	num := func(at, n int) (int, bool) {
		v := 0
		for _, c := range s[at : at+n] {
			if c < '0' || c > '9' {
				return 0, false
			}
			v = v*10 + int(c-'0')
		}
		return v, true
	}
	year, ok1 := num(0, 4)
	month, ok2 := num(5, 2)
	day, ok3 := num(8, 2)
	hour, ok4 := num(11, 2)
	minute, ok5 := num(14, 2)
	sec, ok6 := num(17, 2)
	if !(ok1 && ok2 && ok3 && ok4 && ok5 && ok6) {
		return time.Time{}, false
	}
	if year == 0 || month < 1 || month > 12 || day < 1 || hour > 23 || minute > 59 || sec > 60 {
		return time.Time{}, false
	}
	t := time.Date(year, time.Month(month), day, hour, minute, sec, nsec, loc)
	if t.Day() != day && sec != 60 {
		return time.Time{}, false // February 30th and the like
	}
	return t, true
}

// subSeconds reads SubSecTime digits as a fraction of a second.
func subSeconds(t Tag) int {
	b, err := t.textBytes()
	if err != nil {
		return 0
	}
	ns, scale := 0, 100_000_000
	for _, c := range b {
		if c < '0' || c > '9' {
			break
		}
		if scale > 0 {
			ns += int(c-'0') * scale
			scale /= 10
		}
	}
	return ns
}

// zoneOffset reads "+HH:MM" or "-HH:MM" as seconds east of UTC.
func zoneOffset(t Tag) (int, bool) {
	b, err := t.textBytes()
	if err != nil || len(b) < 6 || (b[0] != '+' && b[0] != '-') || b[3] != ':' {
		return 0, false
	}
	d := func(c byte) (int, bool) { return int(c - '0'), c >= '0' && c <= '9' }
	h1, a := d(b[1])
	h2, b2 := d(b[2])
	m1, c := d(b[4])
	m2, e := d(b[5])
	if !(a && b2 && c && e) {
		return 0, false
	}
	h, m := h1*10+h2, m1*10+m2
	if h > 14 || m > 59 {
		return 0, false
	}
	secs := (h*60 + m) * 60
	if b[0] == '-' {
		secs = -secs
	}
	return secs, true
}
