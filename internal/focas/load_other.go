//go:build !windows && !linux

package focas

import "errors"

func FindLibrary(dir string) string { return "" }

func Load(path string) (*Lib, error) {
	return nil, errors.New("FOCAS2 sampling needs Windows or Linux")
}

func syscallN(fn uintptr, args ...uintptr) uintptr { return 0 }
