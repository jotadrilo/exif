package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	bep "github.com/bep/imagemeta"
	dsoexif "github.com/dsoprea/go-exif/v3"
	dsocommon "github.com/dsoprea/go-exif/v3/common"
	evan "github.com/evanoberholster/imagemeta"
	"github.com/jotadrilo/exif"
	goexif "github.com/rwcarlsen/goexif/exif"
)

var libs = []lib{
	{"baseline", func([]byte, bool) (result, error) { return result{}, nil }, func(path string, _ bool) (result, error) {
		f, err := os.Open(path)
		if err != nil {
			return result{}, err
		}
		return result{}, f.Close()
	}},
	{"jotadrilo/exif Parse", oursMem, oursFile},
	{"jotadrilo/exif Decode", oursDecodeMem, oursFile},
	{"rwcarlsen/goexif", goexifMem, nil},
	{"dsoprea/go-exif v3", dsopreaMem, nil},
	{"evanoberholster/imagemeta", evanMem, nil},
	{"bep/imagemeta", bepMem, nil},
}

func init() {
	for i := range libs {
		if libs[i].file == nil {
			libs[i].file = readerLibs[libs[i].name]
		}
	}
}

// readerLibs parse an *os.File, as each library's documentation suggests for a file.
var readerLibs = map[string]func(string, bool) (result, error){
	"rwcarlsen/goexif": func(path string, all bool) (result, error) {
		return withFile(path, func(f *os.File) (result, error) { return goexifRead(f, all) })
	},
	"dsoprea/go-exif v3": func(path string, all bool) (result, error) {
		raw, err := dsoexif.SearchFileAndExtractExif(path)
		if err != nil {
			return result{}, err
		}
		return dsopreaRaw(raw, all)
	},
	"evanoberholster/imagemeta": func(path string, all bool) (result, error) {
		return withFile(path, func(f *os.File) (result, error) { return evanRead(f, all) })
	},
	"bep/imagemeta": func(path string, all bool) (result, error) {
		return withFile(path, func(f *os.File) (result, error) { return bepRead(f, filepath.Ext(path), all) })
	},
}

func withFile(path string, f func(*os.File) (result, error)) (result, error) {
	file, err := os.Open(path)
	if err != nil {
		return result{}, err
	}
	defer file.Close()
	return f(file)
}

func taken(x exif.Exif) string {
	t, _, ok := x.DateTimeOriginal()
	if !ok {
		return ""
	}
	return t.Format("2006:01:02 15:04:05")
}

func ours(x exif.Exif, all bool) result {
	r := result{Orientation: x.Orientation()}
	if !all {
		return r
	}
	if g, ok := x.GPS(); ok {
		r.HasGPS, r.Lat, r.Lon, r.HasAlt, r.Alt = true, g.Latitude, g.Longitude, g.HasAltitude, g.Altitude
	}
	r.Taken = taken(x)
	return r
}

// The benchmark reads the date as a time.Time but formats it only for the values check.
func oursMem(data []byte, all bool) (result, error) {
	x, err := exif.Parse(data)
	if err != nil {
		return result{}, err
	}
	return oursCheap(x, all), nil
}

func oursDecodeMem(data []byte, all bool) (result, error) {
	x, err := exif.Decode(bytes.NewReader(data))
	if err != nil {
		return result{}, err
	}
	return oursCheap(x, all), nil
}

func oursFile(path string, all bool) (result, error) {
	return withFile(path, func(f *os.File) (result, error) {
		x, err := exif.Decode(f)
		if err != nil {
			return result{}, err
		}
		return oursCheap(x, all), nil
	})
}

// valuesMode makes the libraries format the date; timed runs leave it a time.Time, as the
// other libraries do.
var valuesMode = len(os.Args) > 2 && os.Args[2] == "values"

func oursCheap(x exif.Exif, all bool) result {
	if valuesMode {
		return ours(x, all)
	}
	r := result{Orientation: x.Orientation()}
	if all {
		if g, ok := x.GPS(); ok {
			r.HasGPS, r.Lat, r.Lon, r.HasAlt, r.Alt = true, g.Latitude, g.Longitude, g.HasAltitude, g.Altitude
		}
		_, _, _ = x.DateTimeOriginal()
	}
	return r
}

func goexifMem(data []byte, all bool) (result, error) { return goexifRead(bytes.NewReader(data), all) }

func goexifRead(rd io.Reader, all bool) (result, error) {
	x, err := goexif.Decode(rd)
	if err != nil {
		return result{}, err
	}
	var r result
	r.Orientation = 1
	if t, err := x.Get(goexif.Orientation); err == nil {
		if o, err := t.Int(0); err == nil && o >= 1 && o <= 8 {
			r.Orientation = o
		}
	}
	if !all {
		return r, nil
	}
	if lat, lon, err := x.LatLong(); err == nil {
		r.HasGPS, r.Lat, r.Lon = true, lat, lon
	}
	if t, err := x.Get(goexif.GPSAltitude); err == nil {
		if n, d, err := t.Rat2(0); err == nil && d != 0 {
			r.HasAlt, r.Alt = true, float64(n)/float64(d)
			if ref, err := x.Get(goexif.GPSAltitudeRef); err == nil {
				if v, err := ref.Int(0); err == nil && v == 1 {
					r.Alt = -r.Alt
				}
			}
		}
	}
	if t, err := x.Get(goexif.DateTimeOriginal); err == nil {
		r.Taken, _ = t.StringVal()
	}
	return r, nil
}

var (
	dsoMapping, _ = dsocommon.NewIfdMappingWithStandard()
	dsoTags       = dsoexif.NewTagIndex()
)

func dsopreaMem(data []byte, all bool) (result, error) {
	raw, err := dsoexif.SearchAndExtractExif(data)
	if err != nil {
		return result{}, err
	}
	return dsopreaRaw(raw, all)
}

func dsopreaRaw(raw []byte, all bool) (result, error) {
	_, index, err := dsoexif.Collect(dsoMapping, dsoTags, raw)
	if err != nil {
		return result{}, err
	}
	r := result{Orientation: 1}
	if es, err := index.RootIfd.FindTagWithName("Orientation"); err == nil && len(es) > 0 {
		if v, err := es[0].Value(); err == nil {
			if s, ok := v.([]uint16); ok && len(s) > 0 && s[0] >= 1 && s[0] <= 8 {
				r.Orientation = int(s[0])
			}
		}
	}
	if !all {
		return r, nil
	}
	if gps, err := index.RootIfd.ChildWithIfdPath(dsocommon.IfdGpsInfoStandardIfdIdentity); err == nil {
		if gi, err := gps.GpsInfo(); err == nil {
			r.HasGPS, r.Lat, r.Lon = true, gi.Latitude.Decimal(), gi.Longitude.Decimal()
			// GpsInfo truncates the altitude to whole meters; report it as the library does.
			r.HasAlt, r.Alt = true, float64(gi.Altitude)
		}
	}
	if ifd, err := index.RootIfd.ChildWithIfdPath(dsocommon.IfdExifStandardIfdIdentity); err == nil {
		if es, err := ifd.FindTagWithName("DateTimeOriginal"); err == nil && len(es) > 0 {
			if v, err := es[0].Value(); err == nil {
				r.Taken, _ = v.(string)
			}
		}
	}
	return r, nil
}

func evanMem(data []byte, all bool) (result, error) { return evanRead(bytes.NewReader(data), all) }

func evanRead(rs io.ReadSeeker, all bool) (result, error) {
	e, err := evan.Decode(rs)
	if err != nil {
		return result{}, err
	}
	r := result{Orientation: int(e.IFD0.Orientation)}
	if r.Orientation < 1 || r.Orientation > 8 {
		r.Orientation = 1
	}
	if !all {
		return r, nil
	}
	if lat, lon := e.GPS.Latitude(), e.GPS.Longitude(); lat != 0 || lon != 0 {
		r.HasGPS, r.Lat, r.Lon = true, lat, lon
		r.HasAlt, r.Alt = true, float64(e.GPS.Altitude())
	}
	if t := e.OriginalDate(); !t.IsZero() {
		r.Taken = t.Format("2006:01:02 15:04:05")
	}
	return r, nil
}

var bepFormats = map[string]bep.ImageFormat{".jpg": bep.JPEG, ".png": bep.PNG, ".webp": bep.WebP, ".tif": bep.TIFF}

// bepCurrentExt is the extension of the file the timed child reads; bep/imagemeta needs the
// format named.
var bepCurrentExt = func() string {
	for i, a := range os.Args {
		if a == "-file" && i+1 < len(os.Args) {
			return filepath.Ext(os.Args[i+1])
		}
	}
	return ".jpg"
}()

func bepMem(data []byte, all bool) (result, error) {
	return bepRead(bytes.NewReader(data), bepCurrentExt, all)
}

func bepRead(rs io.ReadSeeker, ext string, all bool) (result, error) {
	format, ok := bepFormats[ext]
	if !ok {
		return result{}, fmt.Errorf("format %s", ext)
	}
	var tags bep.Tags
	opts := bep.Options{R: rs, ImageFormat: format, Sources: bep.EXIF,
		HandleTag: func(ti bep.TagInfo) error {
			tags.Add(ti)
			return nil
		},
	}
	if !all {
		opts.ShouldHandleTag = func(ti bep.TagInfo) bool { return ti.Tag == "Orientation" }
	}
	if _, err := bep.Decode(opts); err != nil && !errors.Is(err, bep.ErrStopWalking) {
		return result{}, err
	}
	ex := tags.EXIF()
	r := result{Orientation: 1}
	if t, ok := ex["Orientation"]; ok {
		if o := toInt(t.Value); o >= 1 && o <= 8 {
			r.Orientation = o
		}
	}
	if !all {
		return r, nil
	}
	if lat, lon, err := tags.GetLatLong(); err == nil && (lat != 0 || lon != 0) {
		r.HasGPS, r.Lat, r.Lon = true, lat, lon
	}
	if t, ok := ex["GPSAltitude"]; ok {
		if f, ok := toFloat(t.Value); ok {
			r.HasAlt, r.Alt = true, f
			if ref, ok := ex["GPSAltitudeRef"]; ok && toInt(ref.Value) == 1 {
				r.Alt = -f
			}
		}
	}
	if t, ok := ex["DateTimeOriginal"]; ok {
		r.Taken = strings.TrimSpace(fmt.Sprint(t.Value))
	}
	return r, nil
}

func toInt(v any) int {
	switch x := v.(type) {
	case uint8:
		return int(x)
	case uint16:
		return int(x)
	case uint32:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	case []uint8:
		if len(x) > 0 {
			return int(x[0])
		}
	}
	return 0
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case interface{ Float64() float64 }:
		return x.Float64(), true
	}
	return 0, false
}
