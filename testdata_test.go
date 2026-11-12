package exif_test

import (
	"bytes"
	"errors"
	"math"
	"os"
	"testing"
	"time"

	"github.com/jotadrilo/exif"
)

// The files in testdata were written by bench/corpus/corpus.sh with exiftool, an independent
// implementation; the values below are what `exiftool -n` reads from them.
func TestTestdata_AgreesWithExiftool(t *testing.T) {
	sydney := exif.GPS{Latitude: -33.8688, Longitude: 151.2093, Altitude: -12.5, HasAltitude: true}
	camera := struct {
		orientation int
		gps         exif.GPS
		taken       time.Time
		maker       string
		model       string
	}{6, sydney, time.Date(2024, 7, 14, 18, 30, 5, 42_000_000, time.FixedZone("", 10*3600)), "Canon", "Canon EOS R5"}
	cases := map[string]struct {
		orientation  int
		gps          exif.GPS
		taken        time.Time
		maker, model string
	}{
		"camera.jpg":  camera,
		"camera.png":  camera,
		"camera.webp": camera,
		"camera.tif":  camera,
		"phone.jpg": {8, exif.GPS{Latitude: 40.416775, Longitude: -3.70379, Altitude: 657, HasAltitude: true},
			time.Date(2023, 12, 31, 23, 59, 59, 0, time.FixedZone("", -5*3600)), "Apple", "iPhone 15 Pro"},
	}
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-6 }
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile("testdata/" + name)
			if err != nil {
				t.Fatal(err)
			}
			f, err := os.Open("testdata/" + name)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			fromFile, err := exif.Decode(f)
			if err != nil {
				t.Fatal(err)
			}
			x, err := both(t, data)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(x.Raw(), fromFile.Raw()) {
				t.Error("Decode of the file differs")
			}
			if x.Orientation() != c.orientation {
				t.Errorf("orientation %d", x.Orientation())
			}
			g, ok := x.GPS()
			if !ok || !near(g.Latitude, c.gps.Latitude) || !near(g.Longitude, c.gps.Longitude) || !near(g.Altitude, c.gps.Altitude) || !g.HasAltitude {
				t.Errorf("GPS %+v %v", g, ok)
			}
			taken, hasOffset, ok := x.DateTimeOriginal()
			_, off := taken.Zone()
			_, wantOff := c.taken.Zone()
			if !ok || !hasOffset || !taken.Equal(c.taken) || off != wantOff {
				t.Errorf("taken %v %v %v", taken, hasOffset, ok)
			}
			if x.Make() != c.maker || x.Model() != c.model {
				t.Errorf("make %q model %q", x.Make(), x.Model())
			}
			if w, h, ok := x.Dimensions(); !ok || w != 4032 || h != 3024 {
				t.Errorf("dimensions %d x %d", w, h)
			}
		})
	}
	plain, _ := os.ReadFile("testdata/plain.jpg")
	if _, err := exif.Parse(plain); !errors.Is(err, exif.ErrNoExif) {
		t.Errorf("plain.jpg: %v", err)
	}
}

func TestTestdata_StripCameraJPEG(t *testing.T) {
	data, err := os.ReadFile("testdata/camera.jpg")
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := os.ReadFile("testdata/plain.jpg")
	got := strip(t, data)
	// plain.jpg is the same image before exiftool wrote to it.
	if !bytes.Equal(got, plain) {
		t.Errorf("stripped %d bytes, plain has %d", len(got), len(plain))
	}
	x, err := exif.Parse(strip(t, data, exif.KeepOrientation()))
	if err != nil || x.Orientation() != 6 {
		t.Errorf("kept orientation %d %v", x.Orientation(), err)
	}
	if bytes.Contains(got, []byte("Test Author")) {
		t.Error("XMP kept")
	}
}
