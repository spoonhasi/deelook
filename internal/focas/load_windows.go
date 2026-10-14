//go:build windows

package focas

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// FindLibrary is the FOCAS2 library deelook uses from dir when -fwlib is not
// given: Fwlib64.dll (Fwlib32.dll in a 32-bit build).
func FindLibrary(dir string) string {
	if unsafe.Sizeof(uintptr(0)) == 4 {
		return filepath.Join(dir, "Fwlib32.dll")
	}
	return filepath.Join(dir, "Fwlib64.dll")
}

// Load opens the 64-bit FOCAS2 DLL at path. The DLLs it loads in turn by
// name at run time (fwlibe64.dll for Ethernet; a missing one gives EW_NODLL
// on connect) are looked up in the same folder: it is registered with
// SetDllDirectory, since such loads otherwise search the executable's
// folder. Fwlib64.dll 7.3 and fwlibe64.dll alone reach a 0i-F and a 30i-B
// over Ethernet: the per-series DLLs (fwlib30i64.dll...) are not needed.
func Load(path string) (*Lib, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := windows.SetDllDirectory(filepath.Dir(abs)); err != nil {
		return nil, fmt.Errorf("SetDllDirectory %s: %w", filepath.Dir(abs), err)
	}
	mod, err := windows.LoadLibraryEx(abs, 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH)
	if err != nil {
		return nil, fmt.Errorf("FOCAS2 library %s: %w (the 64-bit Fwlib64.dll and fwlibe64.dll must be there)", abs, err)
	}
	l := &Lib{Path: abs, Version: fileVersion(abs), fn: map[string]uintptr{},
		sym: func(name string) (uintptr, error) { return windows.GetProcAddress(mod, name) }}
	for _, name := range functions {
		p, err := windows.GetProcAddress(mod, name)
		if err != nil {
			return nil, missing(abs, name)
		}
		l.fn[name] = p
	}
	for _, name := range optional { // PMC signals, CNC information, triggers; axis names
		if p, err := windows.GetProcAddress(mod, name); err == nil {
			l.fn[name] = p
		}
	}
	return l, nil
}

func syscallN(fn uintptr, args ...uintptr) uintptr {
	r, _, _ := syscall.SyscallN(fn, args...)
	return r
}

// fileVersion reads the DLL's FILEVERSION ("7.3.0.1"): the build decides,
// for instance, whether two CNCs behind one IP get separate connections.
func fileVersion(path string) string {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])) != nil {
		return ""
	}
	var info *windows.VS_FIXEDFILEINFO
	var n uint32
	if windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\`, unsafe.Pointer(&info), &n) != nil || info == nil {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d.%d", info.FileVersionMS>>16, info.FileVersionMS&0xffff,
		info.FileVersionLS>>16, info.FileVersionLS&0xffff)
}
