//go:build !unix

package soak

func peakRSSKB() int64 { return 0 }
