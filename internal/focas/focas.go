// Package focas calls the few FANUC FOCAS2 functions deelook uses itself:
// servo/spindle data sampling (cnc_sdt*). Everything else deelook shows
// comes through deemesh-hub.
//
// The library is loaded at run time, without cgo: Fwlib64.dll on Windows,
// libfwlib64.so on Linux (both 64-bit; FOCAS's LONG is 32 bits on both).
//
// A library handle belongs to the OS thread that allocated it (FOCAS2 spec,
// "Library handle"), so a Handle must only be used from the goroutine that
// called Connect, and that goroutine must hold runtime.LockOSThread.
package focas

import (
	"fmt"
	"runtime"
	"strings"
	"time"
	"unsafe"
)

// Return codes of the FOCAS2 functions (Fwlib64.h).
const (
	EW_OK       = 0
	EW_FUNC     = 1
	EW_LENGTH   = 2
	EW_NUMBER   = 3
	EW_ATTRIB   = 4
	EW_DATA     = 5
	EW_NOOPT    = 6
	EW_PROT     = 7
	EW_OVRFLOW  = 8
	EW_PARAM    = 9
	EW_BUFFER   = 10
	EW_PATH     = 11
	EW_MODE     = 12
	EW_REJECT   = 13
	EW_ALARM    = 15
	EW_STOP     = 16
	EW_PASSWD   = 17
	EW_PROTOCOL = -17
	EW_SOCKET   = -16
	EW_NODLL    = -15
	EW_HANDLE   = -8
	EW_VERSION  = -7
	EW_UNEXP    = -6
	EW_SYSTEM   = -5
	EW_RESET    = -2
	EW_BUSY     = -1
)

var codeNames = map[int16]string{
	EW_OK: "EW_OK", EW_FUNC: "EW_FUNC", EW_LENGTH: "EW_LENGTH", EW_NUMBER: "EW_NUMBER",
	EW_ATTRIB: "EW_ATTRIB", EW_DATA: "EW_DATA", EW_NOOPT: "EW_NOOPT", EW_PROT: "EW_PROT",
	EW_OVRFLOW: "EW_OVRFLOW", EW_PARAM: "EW_PARAM", EW_BUFFER: "EW_BUFFER", EW_PATH: "EW_PATH",
	EW_MODE: "EW_MODE", EW_REJECT: "EW_REJECT", EW_ALARM: "EW_ALARM", EW_STOP: "EW_STOP",
	EW_PASSWD: "EW_PASSWD", EW_PROTOCOL: "EW_PROTOCOL", EW_SOCKET: "EW_SOCKET", EW_NODLL: "EW_NODLL",
	EW_HANDLE: "EW_HANDLE", EW_VERSION: "EW_VERSION", EW_UNEXP: "EW_UNEXP", EW_SYSTEM: "EW_SYSTEM",
	EW_RESET: "EW_RESET", EW_BUSY: "EW_BUSY",
}

// Error is a FOCAS2 function that did not return EW_OK. Detail is the
// err_no of cnc_getdtailerr where the function documents one (EW_REJECT,
// EW_DATA), else 0.
type Error struct {
	Func   string
	Code   int16
	Detail int16
}

func (e *Error) Error() string {
	name := codeNames[e.Code]
	if name == "" {
		name = "EW_?"
	}
	s := fmt.Sprintf("%s: %s(%d)", e.Func, name, e.Code)
	if e.Detail != 0 {
		s += fmt.Sprintf(" detail %d", e.Detail)
	}
	return s
}

// Is reports whether err is a FOCAS2 error with the given code.
func Is(err error, code int16) bool {
	e, ok := err.(*Error)
	return ok && e.Code == code
}

// Link reports whether err means the connection to the CNC is gone.
func Link(err error) bool {
	e, ok := err.(*Error)
	return ok && (e.Code == EW_SOCKET || e.Code == EW_HANDLE || e.Code == EW_PROTOCOL)
}

// Channel is one sampling channel, laid out as IDBSDTCHAN (6 bytes).
type Channel struct {
	Type  int16  // data type, see the cnc_sdtsetchnl tables
	ChNo  int8   // 1..8
	Axis  int8   // servo axis 1..32, spindle -1..-8 (logical, across paths)
	Shift uint16 // upper byte: period (0 = 1 ms, 2 = 0.25 ms servo), lower: right shift 0..15
}

// PmcSignal is PMC_DATA (6 bytes): one PMC address for a cnc_sdtsetchnl2
// channel or a cnc_sdtstartsmpl2 trigger.
type PmcSignal struct {
	Unit int16  // PMC path: 1..5, 9 = dual check safety; 0 = unused slot
	Adr  int8   // address kind, index into PmcKinds
	Bit  uint8  // bit 0..7 (0 for byte and word signals)
	No   uint16 // address number
}

// PmcKinds are the address letters in PmcSignal.Adr order (cnc_sdtsetchnl2).
const PmcKinds = "GFYXARTKCDMNEZ"

// Channel2 is IDBSDTCHAN2 (102 bytes): a Channel that may also carry PMC
// signals (Axis -125 word, -126 byte, -127 up to 16 bits) or CNC
// information (Axis 101..115 = path 1..15, Type 1 O number, 2 N number).
type Channel2 struct {
	Type  int16
	ChNo  int8
	Axis  int8
	Shift uint16
	IO    [16]PmcSignal
}

// Axis numbers of the cnc_sdtsetchnl2 channels that are not servo/spindle.
const (
	AxisPmcWord = -125 // 30i-B or later
	AxisPmcByte = -126 // 30i-B or later
	AxisPmcBit  = -127
	AxisCncInfo = 100 // + path 1..15
)

// TrgData is TRG_DATA (12 bytes): what cnc_sdtstartsmpl2 marks as trigger.
type TrgData struct {
	SeqNo int32
	Pmc   PmcSignal
}

// Trigger condition bits of cnc_sdtstartsmpl2 and cnc_sdtreadsmpl2.
const (
	TrgSeq   = 1 << 0  // the block with sequence number SeqNo ran
	TrgUp    = 1 << 1  // the PMC signal rose
	TrgDown  = 1 << 2  // the PMC signal fell
	TrgEnded = 1 << 15 // cnc_sdtreadsmpl2 only: sampling ended (all points taken)
)

// odbsd is ODBSD: where cnc_sdtreadsmpl puts one channel's data.
type odbsd struct {
	chadata *uint16
	count   *int32
}

type odberr struct {
	errNo   int16
	errDtno int16
}

// functions every deelook needs; extended are the cnc_sdt*2 calls (PMC
// signals, CNC information, triggers) that not every build has: the J555
// 32-bit Linux library lacks them. naming reads axis and spindle names, for
// machines deelook knows without deemesh.
var (
	functions = []string{
		"cnc_allclibhndl3", "cnc_freelibhndl", "cnc_getdtailerr",
		"cnc_sdtsetchnl", "cnc_sdtclrchnl", "cnc_sdtstartsmpl", "cnc_sdtcancelsmpl",
		"cnc_sdtreadsmpl", "cnc_sdtendsmpl", "cnc_sdtread1shot",
	}
	extended = []string{"cnc_sdtsetchnl2", "cnc_sdtstartsmpl2", "cnc_sdtreadsmpl2", "cnc_sdtendsmpl2"}
	naming   = []string{"cnc_getpath", "cnc_setpath", "cnc_rdaxisname", "cnc_rdspdlname"}
	optional = append(append([]string{}, extended...), naming...)
)

// missing explains a function the library does not export. Builds differ:
// the 64-bit Linux builds at hand (libfwlib64 1.0.5, libfocas 1.0.0) have
// no cnc_sdt* at all, although Fanuc's Linux manual lists them.
func missing(path, name string) error {
	if strings.HasPrefix(name, "cnc_sdt") {
		return fmt.Errorf("FOCAS2 library %s has no %s: this build has no servo/spindle data sampling (cnc_sdt*); use a build that has it", path, name)
	}
	return fmt.Errorf("FOCAS2 library %s has no %s", path, name)
}

// Lib is a loaded FOCAS2 library.
type Lib struct {
	Path    string
	Version string // file version on Windows, from the file name on Linux
	fn      map[string]uintptr
	sym     func(name string) (uintptr, error) // looks up any exported function
}

func (l *Lib) call(name string, args ...uintptr) int16 {
	r := syscallN(l.fn[name], args...)
	return int16(uint16(r))
}

// Handle is a library handle for one CNC, usable only from the goroutine
// (locked OS thread) that called Connect.
type Handle struct {
	lib *Lib
	h   uint16
}

// Connect allocates a library handle and connects to the CNC
// (cnc_allclibhndl3). The calling goroutine must have locked its OS thread.
//
// On Linux the library must be prepared with cnc_startupprocess on this
// same thread first: prepared on another thread, the handle comes back but
// every call on it answers EW_HANDLE (measured with J555 libfwlib32 1.0.2),
// and without it the library aborts the process. deemesh calls it before
// every connect too.
func (l *Lib) Connect(host string, port uint16, timeout time.Duration) (*Handle, error) {
	if p, ok := l.fn["cnc_startupprocess"]; ok {
		if rc := int16(uint16(syscallN(p, 0, 0))); rc != EW_OK { // log level 0, no log file
			return nil, &Error{Func: "cnc_startupprocess", Code: rc}
		}
	}
	ip := append([]byte(host), 0)
	var h uint16
	rc := l.call("cnc_allclibhndl3", uintptr(unsafe.Pointer(&ip[0])), uintptr(port),
		uintptr(int32(timeout/time.Second)), uintptr(unsafe.Pointer(&h)))
	runtime.KeepAlive(ip)
	if rc != EW_OK {
		return nil, &Error{Func: "cnc_allclibhndl3", Code: rc}
	}
	return &Handle{lib: l, h: h}, nil
}

// Free releases the handle (cnc_freelibhndl), and on Linux the thread's
// library state (cnc_exitthread).
func (h *Handle) Free() error {
	err := h.check("cnc_freelibhndl", h.lib.call("cnc_freelibhndl", uintptr(h.h)), false)
	if p, ok := h.lib.fn["cnc_exitthread"]; ok {
		syscallN(p)
	}
	return err
}

// check turns a return code into an error, asking cnc_getdtailerr for the
// detail number where the function documents one.
func (h *Handle) check(fn string, rc int16, detail bool) error {
	if rc == EW_OK {
		return nil
	}
	e := &Error{Func: fn, Code: rc}
	if detail && (rc == EW_REJECT || rc == EW_DATA) {
		var d odberr
		if h.lib.call("cnc_getdtailerr", uintptr(h.h), uintptr(unsafe.Pointer(&d))) == EW_OK {
			e.Detail = d.errNo
		}
	}
	return e
}

// Unlimited is the datanum of cnc_sdtsetchnl that reads until Cancel; a
// number >= 8 instead makes the CNC stop after that many points per channel
// (then End).
const Unlimited = -1

// SetChannels sets the sampling channels (cnc_sdtsetchnl).
func (h *Handle) SetChannels(chs []Channel, points int32) error {
	rc := h.lib.call("cnc_sdtsetchnl", uintptr(h.h), uintptr(int16(len(chs))), uintptr(points),
		uintptr(unsafe.Pointer(&chs[0])))
	runtime.KeepAlive(chs)
	return h.check("cnc_sdtsetchnl", rc, true)
}

// ClearChannels clears the channel setting (cnc_sdtclrchnl).
func (h *Handle) ClearChannels() error {
	return h.check("cnc_sdtclrchnl", h.lib.call("cnc_sdtclrchnl", uintptr(h.h)), true)
}

// Start starts sampling on path (cnc_sdtstartsmpl): at once with seq 0,
// else when the block with sequence number seq runs (Read reports -1 until
// then).
func (h *Handle) Start(path int16, seq int32) error {
	return h.check("cnc_sdtstartsmpl", h.lib.call("cnc_sdtstartsmpl", uintptr(h.h), uintptr(path), uintptr(seq)), true)
}

// Cancel stops sampling (cnc_sdtcancelsmpl); usable while waiting or sampling.
func (h *Handle) Cancel() error {
	return h.check("cnc_sdtcancelsmpl", h.lib.call("cnc_sdtcancelsmpl", uintptr(h.h)), true)
}

// End closes a sampling the CNC finished (cnc_sdtendsmpl), after Read
// reported stat 0.
func (h *Handle) End() error {
	return h.check("cnc_sdtendsmpl", h.lib.call("cnc_sdtendsmpl", uintptr(h.h)), true)
}

// ReadOnce reads the present value of each set channel (cnc_sdtread1shot)
// without sampling: SetChannels first.
func (h *Handle) ReadOnce() ([8]uint16, error) {
	var data [8]uint16
	rc := h.lib.call("cnc_sdtread1shot", uintptr(h.h), uintptr(unsafe.Pointer(&data[0])))
	return data, h.check("cnc_sdtread1shot", rc, true)
}

// Names reads the axis and spindle names of every path, in the order
// cnc_sdtsetchnl numbers axes and spindles: path 1's, then path 2's
// (cnc_getpath, cnc_setpath, cnc_rdaxisname, cnc_rdspdlname). It leaves the
// handle on path 1.
func (h *Handle) Names() (axes, spindles []string, paths int, err error) {
	for _, n := range naming {
		if _, ok := h.lib.fn[n]; !ok {
			return nil, nil, 0, &Error{Func: n, Code: EW_FUNC}
		}
	}
	var cur, maxPath int16
	if err := h.check("cnc_getpath", h.lib.call("cnc_getpath", uintptr(h.h), uintptr(unsafe.Pointer(&cur)), uintptr(unsafe.Pointer(&maxPath))), false); err != nil {
		return nil, nil, 0, err
	}
	paths = max(int(maxPath), 1)
	defer h.lib.call("cnc_setpath", uintptr(h.h), 1)
	for p := 1; p <= paths; p++ {
		if paths > 1 {
			if err := h.check("cnc_setpath", h.lib.call("cnc_setpath", uintptr(h.h), uintptr(p)), false); err != nil {
				return nil, nil, 0, err
			}
		}
		var ax [32]struct{ name, suff byte } // ODBAXISNAME
		n := int16(len(ax))
		if err := h.check("cnc_rdaxisname", h.lib.call("cnc_rdaxisname", uintptr(h.h), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&ax[0]))), false); err != nil {
			return nil, nil, 0, err
		}
		for _, a := range ax[:max(0, min(int(n), len(ax)))] {
			axes = append(axes, nameOf(a.name, a.suff))
		}
		var sp [8]struct{ name, suff1, suff2, suff3 byte } // ODBSPDLNAME
		n = int16(len(sp))
		if err := h.check("cnc_rdspdlname", h.lib.call("cnc_rdspdlname", uintptr(h.h), uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&sp[0]))), false); err != nil {
			return nil, nil, 0, err
		}
		for _, s := range sp[:max(0, min(int(n), len(sp)))] {
			spindles = append(spindles, nameOf(s.name, s.suff1, s.suff2, s.suff3))
		}
	}
	return axes, spindles, paths, nil
}

// nameOf joins an axis or spindle name and its suffix characters.
func nameOf(chars ...byte) string {
	var b []byte
	for _, c := range chars {
		if c > ' ' && c < 0x7f {
			b = append(b, c)
		}
	}
	return string(b)
}

// Extended reports whether the library has the cnc_sdt*2 calls.
func (l *Lib) Extended() bool {
	for _, n := range extended {
		if _, ok := l.fn[n]; !ok {
			return false
		}
	}
	return true
}

// SetChannels2 sets channels that may carry PMC signals or CNC
// information (cnc_sdtsetchnl2). Read them with Reader.Read2.
func (h *Handle) SetChannels2(chs []Channel2, points int32) error {
	rc := h.lib.call("cnc_sdtsetchnl2", uintptr(h.h), uintptr(int16(len(chs))), uintptr(points),
		uintptr(unsafe.Pointer(&chs[0])))
	runtime.KeepAlive(chs)
	return h.check("cnc_sdtsetchnl2", rc, true)
}

// Start2 starts sampling at once and has the CNC mark the moment the
// conditions (TrgSeq, TrgUp, TrgDown bits) come true (cnc_sdtstartsmpl2).
// The header passes TRG_DATA by pointer, whatever the reference page says.
func (h *Handle) Start2(path int16, condition uint16, trg TrgData) error {
	rc := h.lib.call("cnc_sdtstartsmpl2", uintptr(h.h), uintptr(path), uintptr(condition), uintptr(unsafe.Pointer(&trg)))
	runtime.KeepAlive(&trg)
	return h.check("cnc_sdtstartsmpl2", rc, true)
}

// End2 closes a cnc_sdtstartsmpl2 sampling (cnc_sdtendsmpl2).
func (h *Handle) End2() error {
	return h.check("cnc_sdtendsmpl2", h.lib.call("cnc_sdtendsmpl2", uintptr(h.h)), true)
}

// Reader reads sampled data into buffers it owns. The buffers stay pinned
// while the Reader lives, since FOCAS writes into them through ODBSD.
type Reader struct {
	h      *Handle
	Data   [][]uint16 // per channel; valid up to Counts[i] after Read
	Counts []int32
	desc   []odbsd
	pin    runtime.Pinner
}

// NewReader prepares buffers of size points for each of n channels.
// size should be a multiple of 8 (FOCAS reads in 8-word units).
func (h *Handle) NewReader(n, size int) *Reader {
	r := &Reader{h: h, Data: make([][]uint16, n), Counts: make([]int32, n), desc: make([]odbsd, n)}
	for i := range n {
		r.Data[i] = make([]uint16, size)
		r.desc[i] = odbsd{chadata: &r.Data[i][0], count: &r.Counts[i]}
		r.pin.Pin(&r.Data[i][0])
	}
	r.pin.Pin(&r.Counts[0])
	r.pin.Pin(&r.desc[0])
	return r
}

// Read fetches what the CNC has sampled since the last call
// (cnc_sdtreadsmpl). stat: -1 waiting to start, 1 sampling, 0 ended.
func (r *Reader) Read() (stat int16, err error) {
	rc := r.h.lib.call("cnc_sdtreadsmpl", uintptr(r.h.h), uintptr(unsafe.Pointer(&stat)),
		uintptr(int32(len(r.Data[0]))), uintptr(unsafe.Pointer(&r.desc[0])))
	return stat, r.h.check("cnc_sdtreadsmpl", rc, true)
}

// Read2 is Read for a cnc_sdtstartsmpl2 sampling (cnc_sdtreadsmpl2). trg
// holds the trigger bits that came true, the last point read being the
// moment; TrgEnded once all points were taken.
func (r *Reader) Read2() (trg uint16, err error) {
	rc := r.h.lib.call("cnc_sdtreadsmpl2", uintptr(r.h.h), uintptr(unsafe.Pointer(&trg)),
		uintptr(int32(len(r.Data[0]))), uintptr(unsafe.Pointer(&r.desc[0])))
	return trg, r.h.check("cnc_sdtreadsmpl2", rc, true)
}

// Close unpins the buffers.
func (r *Reader) Close() { r.pin.Unpin() }
