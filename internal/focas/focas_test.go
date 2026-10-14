package focas

import (
	"testing"
	"unsafe"
)

// The structs passed to FOCAS must match Fwlib64.h (#pragma pack(4)).
func TestLayout(t *testing.T) {
	if s := unsafe.Sizeof(Channel{}); s != 6 {
		t.Errorf("IDBSDTCHAN size %d, want 6", s)
	}
	if o := unsafe.Offsetof(Channel{}.Shift); o != 4 {
		t.Errorf("IDBSDTCHAN.shift at %d, want 4", o)
	}
	if s, p := unsafe.Sizeof(odbsd{}), unsafe.Sizeof(uintptr(0)); s != 2*p {
		t.Errorf("ODBSD size %d, want two pointers (%d)", s, 2*p)
	}
	if s := unsafe.Sizeof(odberr{}); s != 4 {
		t.Errorf("ODBERR size %d, want 4", s)
	}
}

func TestErrorText(t *testing.T) {
	e := &Error{Func: "cnc_sdtstartsmpl", Code: EW_REJECT, Detail: 1}
	if got, want := e.Error(), "cnc_sdtstartsmpl: EW_REJECT(13) detail 1"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLayout2(t *testing.T) {
	if s := unsafe.Sizeof(PmcSignal{}); s != 6 {
		t.Errorf("PMC_DATA size %d, want 6", s)
	}
	if s := unsafe.Sizeof(Channel2{}); s != 102 {
		t.Errorf("IDBSDTCHAN2 size %d, want 102", s)
	}
	if o := unsafe.Offsetof(Channel2{}.IO); o != 6 {
		t.Errorf("IDBSDTCHAN2.io at %d, want 6", o)
	}
	if s := unsafe.Sizeof(TrgData{}); s != 12 {
		t.Errorf("TRG_DATA size %d, want 12", s)
	}
	if o := unsafe.Offsetof(TrgData{}.Pmc); o != 4 {
		t.Errorf("TRG_DATA.pmc_trg at %d, want 4", o)
	}
}
