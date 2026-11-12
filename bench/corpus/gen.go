//go:build ignore

// gen writes the base images that corpus.sh gives metadata to: a 12-megapixel JPEG, a
// thumbnail, and small JPEG and PNG images. Their pixels are a deterministic pattern with
// noise, so the compressed sizes look like a photo's.
package main

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"log"
	"os"
)

func pattern(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	seed := uint32(1)
	for y := range h {
		for x := range w {
			seed = seed*1664525 + 1013904223
			n := uint8(seed >> 27)
			img.Pix[img.PixOffset(x, y)+0] = uint8(x*255/w) + n
			img.Pix[img.PixOffset(x, y)+1] = uint8(y*255/h) + n
			img.Pix[img.PixOffset(x, y)+2] = uint8((x+y)*128/(w+h)) + n
			img.Pix[img.PixOffset(x, y)+3] = 255
		}
	}
	return img
}

func write(name string, f func(*os.File) error) {
	out, err := os.Create(name)
	if err != nil {
		log.Fatal(err)
	}
	if err := f(out); err != nil {
		log.Fatal(err)
	}
	if err := out.Close(); err != nil {
		log.Fatal(err)
	}
}

func main() {
	dir := os.Args[1]
	for name, size := range map[string][2]int{"base-large.jpg": {4032, 3024}, "base-small.jpg": {64, 48}, "thumb.jpg": {160, 120}} {
		img := pattern(size[0], size[1])
		write(dir+"/"+name, func(f *os.File) error { return jpeg.Encode(f, img, &jpeg.Options{Quality: 88}) })
	}
	for name, size := range map[string][2]int{"base-large.png": {1024, 768}, "base-small.png": {64, 48}} {
		img := pattern(size[0], size[1])
		img.Set(0, 0, color.RGBA{1, 2, 3, 255})
		write(dir+"/"+name, func(f *os.File) error { return png.Encode(f, img) })
	}
}
