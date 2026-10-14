package focas

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"
)

// TestSampling talks to a real CNC and is skipped unless FOCAS_LIB,
// FOCAS_HOST and FOCAS_PORT are set. It logs every call's result, then
// samples servo axis 1's torque command for a second.
func TestSampling(t *testing.T) {
	path, host := os.Getenv("FOCAS_LIB"), os.Getenv("FOCAS_HOST")
	port, _ := strconv.Atoi(os.Getenv("FOCAS_PORT"))
	if path == "" || host == "" || port == 0 {
		t.Skip("set FOCAS_LIB, FOCAS_HOST and FOCAS_PORT to run against a CNC")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	lib, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("library %s %s, pointer %d bytes", lib.Path, lib.Version, unsafe.Sizeof(uintptr(0)))
	h, err := lib.Connect(host, uint16(port), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Free()
	t.Logf("handle %d", h.h)

	// a plain read first: tells a bad handle from a refused sampler
	if p, err := lib.sym("cnc_statinfo"); err == nil {
		var buf [64]byte
		rc := int16(uint16(syscallN(p, uintptr(h.h), uintptr(unsafe.Pointer(&buf[0])))))
		t.Logf("cnc_statinfo rc=%d", rc)
	}
	t.Logf("cancel: %v", h.Cancel())
	t.Logf("clear: %v", h.ClearChannels())
	if err := h.SetChannels([]Channel{{Type: 3, ChNo: 1, Axis: 1}}, Unlimited); err != nil {
		t.Fatal(err)
	}
	if err := h.Start(1, 0); err != nil {
		t.Fatal(err)
	}
	r := h.NewReader(1, 4096)
	defer r.Close()
	total := 0
	for range 10 {
		time.Sleep(100 * time.Millisecond)
		stat, err := r.Read()
		if err != nil {
			t.Fatal(err)
		}
		total += int(r.Counts[0])
		t.Logf("read stat=%d count=%d", stat, r.Counts[0])
	}
	t.Logf("cancel: %v, %d samples", h.Cancel(), total)
	if total == 0 {
		t.Error("no samples")
	}
}

// TestSamplingSetups isolates what the server does differently from
// TestSampling: FOCAS_CASE=thread loads the library on another OS thread,
// FOCAS_CASE=mixed sets 0.25 ms servo + 1 ms spindle channels.
func TestSamplingSetups(t *testing.T) {
	path, host := os.Getenv("FOCAS_LIB"), os.Getenv("FOCAS_HOST")
	port, _ := strconv.Atoi(os.Getenv("FOCAS_PORT"))
	which := os.Getenv("FOCAS_CASE")
	if path == "" || host == "" || port == 0 || which == "" {
		t.Skip("set FOCAS_LIB, FOCAS_HOST, FOCAS_PORT and FOCAS_CASE")
	}
	var lib *Lib
	var err error
	if which == "thread" {
		done := make(chan struct{})
		go func() {
			runtime.LockOSThread() // keep this thread from being reused below
			lib, err = Load(path)
			close(done)
			select {} // hold the thread
		}()
		<-done
	} else {
		lib, err = Load(path)
	}
	if err != nil {
		t.Fatal(err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	h, err := lib.Connect(host, uint16(port), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Free()
	t.Logf("cancel: %v / clear: %v", h.Cancel(), h.ClearChannels())
	chs := []Channel{{Type: 3, ChNo: 1, Axis: 1}}
	if which == "mixed" {
		chs = []Channel{
			{Type: 3, ChNo: 1, Axis: 1, Shift: 2 << 8},
			{Type: 0, ChNo: 2, Axis: 1, Shift: 2 << 8},
			{Type: 1, ChNo: 3, Axis: -1},
		}
	}
	if err := h.SetChannels(chs, Unlimited); err != nil {
		t.Fatal(err)
	}
	if err := h.Start(1, 0); err != nil {
		t.Fatal(err)
	}
	r := h.NewReader(len(chs), 8192)
	defer r.Close()
	time.Sleep(500 * time.Millisecond)
	stat, err := r.Read()
	t.Logf("read stat=%d err=%v counts=%v cancel=%v", stat, err, r.Counts, h.Cancel())
	if err != nil {
		t.Fatal(err)
	}
}

// TestFeatures tries one cnc_sdt* feature against a real CNC, picked with
// FOCAS_CASE: names (axis and spindle names), fixed (a set number of points, then End), oneshot
// (cnc_sdtread1shot), seq (wait for a block number, then cancel), ext
// (cnc_sdtsetchnl2 with the channels FOCAS_EXT lists in order: servo with
// type FOCAS_TYPE, pmc = the FOCAS_PMC bit like Y0.4, cnc = path 1 N
// number, byte/word = FOCAS_BYTE like F1; cnc_sdtstartsmpl2 marking the
// PMC bit's rise and fall; FOCAS_DUMP=1 logs each channel's raw words).
// ext found that a PMC bit channel must be the last one.
func TestFeatures(t *testing.T) {
	path, host := os.Getenv("FOCAS_LIB"), os.Getenv("FOCAS_HOST")
	port, _ := strconv.Atoi(os.Getenv("FOCAS_PORT"))
	which := os.Getenv("FOCAS_CASE")
	if path == "" || host == "" || port == 0 || which == "" {
		t.Skip("set FOCAS_LIB, FOCAS_HOST, FOCAS_PORT and FOCAS_CASE")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	lib, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("extended calls: %v", lib.Extended())
	h, err := lib.Connect(host, uint16(port), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Free()
	_, _ = h.Cancel(), h.ClearChannels()
	tcmd := []Channel{{Type: 3, ChNo: 1, Axis: 1}, {Type: 1, ChNo: 2, Axis: -1}}

	switch which {
	case "rate":
		// basic sampling read as fast as the link allows, to see how many
		// points one cnc_sdtreadsmpl call carries (FOCAS_PERIOD 250 or 1000)
		period := uint16(0)
		if os.Getenv("FOCAS_PERIOD") == "250" {
			period = 2 << 8
		}
		chs := []Channel{{Type: 3, ChNo: 1, Axis: 1, Shift: period}, {Type: 0, ChNo: 2, Axis: 1, Shift: period}, {Type: 1, ChNo: 3, Axis: -1}}
		if err := h.SetChannels(chs, Unlimited); err != nil {
			t.Fatal(err)
		}
		if err := h.Start(1, 0); err != nil {
			t.Fatal(err)
		}
		defer func() { t.Logf("cancel %v", h.Cancel()) }()
		r := h.NewReader(3, 8192)
		defer r.Close()
		began := time.Now()
		var total [3]int
		for i := 0; time.Since(began) < 5*time.Second; i++ {
			t0 := time.Now()
			_, err := r.Read()
			if err != nil {
				t.Fatal(err)
			}
			for c := range 3 {
				total[c] += int(r.Counts[c])
			}
			if i < 6 {
				t.Logf("read %d took %v counts=%v", i, time.Since(t0).Round(time.Millisecond), r.Counts)
			}
		}
		el := time.Since(began).Seconds()
		t.Logf("5 s: %v points, %.0f/s per channel", total, float64(total[0])/el)
	case "names":
		axes, spindles, paths, err := h.Names()
		t.Logf("paths %d axes %q spindles %q err %v", paths, axes, spindles, err)
	case "recover":
		// after a failed cnc_sdtreadsmpl2 the CNC wants cnc_sdtendsmpl2
		t.Logf("end2: %v / end: %v / cancel: %v / clear: %v", h.End2(), h.End(), h.Cancel(), h.ClearChannels())
	case "fixed":
		if err := h.SetChannels(tcmd, 2000); err != nil {
			t.Fatal(err)
		}
		if err := h.Start(1, 0); err != nil {
			t.Fatal(err)
		}
		r := h.NewReader(2, 4096)
		defer r.Close()
		total := [2]int{}
		for i := range 60 {
			time.Sleep(100 * time.Millisecond)
			stat, err := r.Read()
			if err != nil {
				t.Fatal(err)
			}
			total[0] += int(r.Counts[0])
			total[1] += int(r.Counts[1])
			if stat == 0 {
				t.Logf("ended after %d reads: %v points; End: %v", i+1, total, h.End())
				return
			}
		}
		t.Errorf("never ended: %v", total)
	case "oneshot":
		if err := h.SetChannels(tcmd, Unlimited); err != nil {
			t.Fatal(err)
		}
		for range 3 {
			d, err := h.ReadOnce()
			t.Logf("read1shot %v %v", d, err)
			time.Sleep(200 * time.Millisecond)
		}
		t.Logf("clear: %v", h.ClearChannels())
	case "seq":
		if err := h.SetChannels(tcmd, Unlimited); err != nil {
			t.Fatal(err)
		}
		if err := h.Start(1, 9999); err != nil {
			t.Fatal(err)
		}
		r := h.NewReader(2, 4096)
		defer r.Close()
		for range 5 {
			time.Sleep(200 * time.Millisecond)
			stat, err := r.Read()
			t.Logf("stat=%d counts=%v err=%v", stat, r.Counts, err)
		}
		t.Logf("cancel: %v", h.Cancel())
	case "ext":
		if !lib.Extended() {
			t.Skip("no cnc_sdt*2 in this library")
		}
		sig := os.Getenv("FOCAS_PMC") // like Y0.4
		var pmc PmcSignal
		if len(sig) >= 4 {
			k := strings.IndexByte(PmcKinds, sig[0])
			dot := strings.IndexByte(sig, '.')
			no, _ := strconv.Atoi(sig[1:dot])
			bit, _ := strconv.Atoi(sig[dot+1:])
			pmc = PmcSignal{Unit: 1, Adr: int8(k), Bit: uint8(bit), No: uint16(no)}
		}
		// FOCAS_EXT picks the channels in order: servo (X TCMD), pmc (the
		// FOCAS_PMC bit), cnc (path 1 N number); default servo,pmc,cnc
		kinds := strings.Split(os.Getenv("FOCAS_EXT"), ",")
		if kinds[0] == "" {
			kinds = []string{"servo", "pmc", "cnc"}
		}
		var chs []Channel2
		for i, k := range kinds {
			c := Channel2{ChNo: int8(i + 1)}
			switch k {
			case "servo":
				c.Type, c.Axis = int16(envInt("FOCAS_TYPE", 3)), 1
			case "pmc":
				c.Axis = AxisPmcBit
				c.IO[0] = pmc
			case "cnc":
				c.Type, c.Axis = 2, AxisCncInfo+1
			case "byte", "word": // FOCAS_BYTE like F1 (30i-B or later)
				a := os.Getenv("FOCAS_BYTE")
				no, _ := strconv.Atoi(a[1:])
				c.Axis = map[string]int8{"byte": AxisPmcByte, "word": AxisPmcWord}[k]
				c.IO[0] = PmcSignal{Unit: 1, Adr: int8(strings.IndexByte(PmcKinds, a[0])), No: uint16(no)}
			}
			chs = append(chs, c)
		}
		points, _ := strconv.Atoi(os.Getenv("FOCAS_POINTS"))
		if points == 0 {
			points = 10000 // cnc_sdtsetchnl2 wants 8 or more; -1 is not documented for it
		}
		if err := h.SetChannels2(chs, int32(points)); err != nil {
			t.Fatal(err)
		}
		if err := h.Start2(1, TrgUp|TrgDown, TrgData{Pmc: pmc}); err != nil {
			t.Fatal(err)
		}
		// a failed cnc_sdtreadsmpl2 needs cnc_sdtendsmpl2, or the CNC's
		// sampler stays stuck (NC Guide then refused every new connection)
		defer func() { t.Logf("cleanup: end2 %v / cancel %v", h.End2(), h.Cancel()) }()
		size, _ := strconv.Atoi(os.Getenv("FOCAS_N"))
		if size == 0 {
			size = 8192
		}
		every, _ := strconv.Atoi(os.Getenv("FOCAS_MS"))
		if every == 0 {
			every = 250
		}
		r := h.NewReader(len(chs), size)
		defer r.Close()
		began := time.Now()
		for i := range 10000 / every {
			time.Sleep(time.Duration(every) * time.Millisecond)
			t0 := time.Now()
			trg, err := r.Read2()
			took := time.Since(t0)
			if err != nil {
				t.Errorf("read %d at %v: %v", i, time.Since(began).Round(time.Millisecond), err)
				return
			}
			last := func(c int) uint16 {
				if c >= len(chs) || r.Counts[c] == 0 {
					return 0
				}
				return r.Data[c][r.Counts[c]-1]
			}
			if trg != 0 || i%(2000/every) == 0 || os.Getenv("FOCAS_VERBOSE") != "" {
				t.Logf("read %d at %v (took %v) trg=%#04x counts=%v last pmc=%04x N=%d", i, time.Since(began).Round(time.Millisecond), took.Round(time.Millisecond), trg, r.Counts, last(1), last(2))
			}
			if i < 4 && os.Getenv("FOCAS_DUMP") != "" {
				// the raw words of each channel, to see how the library lays them out
				for c := range 3 {
					t.Logf("  ch%d: %v", c+1, r.Data[c][:min(int(r.Counts[c]), 64)])
				}
			}
		}
	}
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return v
	}
	return def
}
