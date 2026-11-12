package exif_test

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/jotadrilo/exif"
)

func benchFile(b *testing.B, name string) []byte {
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		b.Fatal(err)
	}
	return data
}

func BenchmarkParse(b *testing.B) {
	for _, name := range []string{"camera.jpg", "camera.png", "camera.webp", "camera.tif"} {
		data := benchFile(b, name)
		b.Run(name+"/Orientation", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				x, _ := exif.Parse(data)
				_ = x.Orientation()
			}
		})
		b.Run(name+"/GPS", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				x, _ := exif.Parse(data)
				_, _ = x.GPS()
			}
		})
		b.Run(name+"/DateTimeOriginal", func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				x, _ := exif.Parse(data)
				_, _, _ = x.DateTimeOriginal()
			}
		})
	}
}

func BenchmarkDecode(b *testing.B) {
	data := benchFile(b, "camera.jpg")
	b.ReportAllocs()
	for range b.N {
		x, _ := exif.Decode(bytes.NewReader(data))
		_ = x.Orientation()
	}
}

func BenchmarkStrip(b *testing.B) {
	data := benchFile(b, "camera.jpg")
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for range b.N {
		_ = exif.Strip(io.Discard, bytes.NewReader(data), exif.KeepOrientation())
	}
}
