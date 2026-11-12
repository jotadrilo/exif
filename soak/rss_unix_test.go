//go:build unix

package soak

import (
	"runtime"
	"syscall"
)

func peakRSSKB() int64 {
	var ru syscall.Rusage
	if syscall.Getrusage(syscall.RUSAGE_SELF, &ru) != nil {
		return 0
	}
	if runtime.GOOS == "darwin" {
		return ru.Maxrss / 1024
	}
	return ru.Maxrss
}
