// Package exif reads EXIF metadata from JPEG, TIFF, PNG and WebP images, and strips metadata
// from JPEG files, in pure Go with no dependencies.
//
// It is built for untrusted uploads and for speed. Parse reads an image held in memory without
// copying or allocating: it locates the EXIF block and returns an Exif that walks, on each
// call, only the directories that call needs. Every read is bounds-checked, sub-directory
// pointers that loop or leave the data are ignored, and every error is a constant of type
// Error.
//
// The common cases:
//
//	x, _ := exif.Parse(data) // a failed parse leaves a zero Exif: orientation 1, no GPS
//	switch x.Orientation() { ... }
//
//	if g, ok := x.GPS(); ok {
//		fmt.Println(g.Latitude, g.Longitude)
//	}
//
// Any tag is reachable through Tag and Tags, with typed accessors on Tag.
package exif
