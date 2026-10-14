//go:build linux

package focas

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/ebitengine/purego"
)

// FindLibrary is the FOCAS2 library deelook uses from dir when -fwlib is not
// given: the libfwlib32.so* there (Fanuc names its Linux builds so whatever
// their bitness).
func FindLibrary(dir string) string {
	if m, _ := filepath.Glob(filepath.Join(dir, "libfwlib32.so*")); len(m) > 0 {
		sort.Strings(m)
		return m[len(m)-1]
	}
	return filepath.Join(dir, "libfwlib32.so")
}

// Load opens the FOCAS2 shared library at path (64-bit, or 32-bit for a
// 386/arm build). The Linux library also needs cnc_startupprocess on the
// thread that connects; Connect does that.
func Load(path string) (*Lib, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	mod, err := purego.Dlopen(abs, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("FOCAS2 library %s: %w", abs, err)
	}
	l := &Lib{Path: abs, Version: filepath.Base(abs), fn: map[string]uintptr{},
		sym: func(name string) (uintptr, error) { return purego.Dlsym(mod, name) }}
	for _, name := range append(functions, "cnc_startupprocess") {
		p, err := purego.Dlsym(mod, name)
		if err != nil {
			return nil, missing(abs, name)
		}
		l.fn[name] = p
	}
	if p, err := purego.Dlsym(mod, "cnc_exitthread"); err == nil {
		l.fn["cnc_exitthread"] = p
	}
	for _, name := range optional { // PMC signals, CNC information, triggers; axis names
		if p, err := purego.Dlsym(mod, name); err == nil {
			l.fn[name] = p
		}
	}
	return l, nil
}

func syscallN(fn uintptr, args ...uintptr) uintptr {
	r, _, _ := purego.SyscallN(fn, args...)
	return r
}
