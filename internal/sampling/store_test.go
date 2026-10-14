package sampling

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"deelook/internal/focas"
)

func TestStoreCSV(t *testing.T) {
	st, err := OpenStore(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := Config{Mode: "basic", ServoPeriodUs: 250, Channels: []ChannelConfig{
		{Kind: "servo", Axis: 1, Type: 3, Label: "X TCMD"},
		{Kind: "spindle", Axis: 1, Type: 1, Label: "S1 SPEED"},
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateSession("2", "bench", cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// servo: 8 samples at 0.25 ms in two chunks (one negative value);
	// spindle: 2 samples at 1 ms
	err = st.Add(id, []Chunk{
		{Channel: 0, First: 0, Data: []uint16{1, 2, 3, 0xffff}, ReceivedAt: now},
		{Channel: 1, First: 0, Data: []uint16{100}, ReceivedAt: now},
		{Channel: 0, First: 4, Data: []uint16{5, 6, 7, 8}, ReceivedAt: now},
		{Channel: 1, First: 1, Data: []uint16{101}, ReceivedAt: now},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishSession(id, "stopped", "", now, []int64{8, 2}); err != nil {
		t.Fatal(err)
	}

	var b strings.Builder
	if err := st.WriteCSV(&b, id); err != nil {
		t.Fatal(err)
	}
	want := `time_ms,X TCMD,S1 SPEED
0.000,1,100
0.250,2,
0.500,3,
0.750,-1,
1.000,5,101
1.250,6,
1.500,7,
1.750,8,
`
	if b.String() != want {
		t.Errorf("CSV:\n%s\nwant:\n%s", b.String(), want)
	}

	list, err := st.Sessions("2", 10)
	if err != nil || len(list) != 1 || list[0].Status != "stopped" || list[0].Samples[0] != 8 {
		t.Errorf("sessions: %+v %v", list, err)
	}
}

func TestStoreCSVExtended(t *testing.T) {
	st, err := OpenStore(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	cfg := Config{Mode: "extended", ServoPeriodUs: 1000, Points: 100, Channels: []ChannelConfig{
		{Kind: "pmcbit", Signals: []string{"Y0.4", "x8.4"}, Label: "sig"},
		{Kind: "cnc", Axis: 1, Type: 2, Label: "N"},
		{Kind: "pmcword", Signals: []string{"R100"}, Label: "R,100"},
	}, Trigger: Trigger{Signal: "Y0.4", Up: true}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateSession("1", "ncguide", cfg, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	err = st.Add(id, []Chunk{
		{Channel: 0, First: 0, Data: []uint16{0, 1, 3}, ReceivedAt: now},
		{Channel: 1, First: 0, Data: []uint16{40000, 40000, 40010}, ReceivedAt: now},
		{Channel: 2, First: 0, Data: []uint16{0xffff, 1, 2}, ReceivedAt: now},
	}, []Event{{TimeUs: 1000, Bits: focas.TrgUp, At: now}})
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	if err := st.WriteCSV(&b, id); err != nil {
		t.Fatal(err)
	}
	want := `time_ms,sig:Y0.4,sig:x8.4,N,"R,100",trigger
0.000,0,0,40000,65535,
1.000,1,0,40000,1,0x0002
2.000,1,1,40010,2,
`
	if b.String() != want {
		t.Errorf("CSV:\n%s\nwant:\n%s", b.String(), want)
	}
	if list, _ := st.Sessions("1", 1); len(list) != 1 || list[0].Events != 1 {
		t.Errorf("sessions: %+v", list)
	}

	// the whole session in 2 buckets: PMC bits as AND/OR, CNC info unsigned
	v, err := st.View(id, 0, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if v.TotalUs != 3000 || len(v.Channels) != 3 || len(v.Events) != 1 {
		t.Fatalf("view %+v", v)
	}
	bits, n := v.Channels[0], v.Channels[1]
	if !slices.Equal(bits.Min, []int32{0, 3}) || !slices.Equal(bits.Max, []int32{1, 3}) || bits.BucketUs != 2000 || bits.EndUs != 4000 {
		t.Errorf("pmc bits %+v", bits)
	}
	if !slices.Equal(n.Min, []int32{40000, 40010}) || !slices.Equal(n.Max, []int32{40000, 40010}) {
		t.Errorf("cnc info %+v", n)
	}
	// a zoomed range
	v, _ = st.View(id, 1000, 2000, 10)
	if len(v.Channels[2].Min) != 1 || v.Channels[2].Min[0] != 1 {
		t.Errorf("zoomed %+v", v.Channels[2])
	}
}

func TestConfigChannels(t *testing.T) {
	cfg := Config{Mode: "basic", ServoPeriodUs: 250, Channels: []ChannelConfig{
		{Kind: "servo", Axis: 2, Type: 3, Shift: 0},
		{Kind: "spindle", Axis: 1, Type: 1, Shift: 3},
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	ch := cfg.focasChannels()
	if ch[0].Axis != 2 || ch[0].Shift != 0x0200 || ch[0].ChNo != 1 {
		t.Errorf("servo channel %+v", ch[0])
	}
	if ch[1].Axis != -1 || ch[1].Shift != 0x0003 || ch[1].ChNo != 2 {
		t.Errorf("spindle channel %+v", ch[1])
	}
	for name, bad := range map[string]Config{
		"500 µs":               {Mode: "basic", ServoPeriodUs: 500, Channels: cfg.Channels},
		"no mode":              {ServoPeriodUs: 1000, Channels: cfg.Channels},
		"0.25 ms mixed, fixed": {Mode: "basic", ServoPeriodUs: 250, Points: 1000, Channels: cfg.Channels},
		"4 points":             {Mode: "basic", ServoPeriodUs: 1000, Points: 4, Channels: cfg.Channels},
		"timer trigger":        {Mode: "extended", ServoPeriodUs: 1000, Channels: cfg.Channels, Trigger: Trigger{Signal: "T10.0", Up: true}},
		"pmc in basic":         {Mode: "basic", ServoPeriodUs: 1000, Channels: []ChannelConfig{{Kind: "pmcbit", Signals: []string{"Y0.4"}}}},
		"two bit channels": {Mode: "extended", ServoPeriodUs: 1000, Points: 100, Channels: []ChannelConfig{
			{Kind: "pmcbit", Signals: []string{"Y0.4"}}, {Kind: "pmcbit", Signals: []string{"Y0.5"}}}},
		"byte with a bit": {Mode: "extended", ServoPeriodUs: 1000, Points: 100, Channels: []ChannelConfig{
			{Kind: "pmcbyte", Signals: []string{"R100.1"}}}},
		"trigger without edge": {Mode: "extended", ServoPeriodUs: 1000, Points: 100, Channels: cfg.Channels,
			Trigger: Trigger{Signal: "Y0.4"}},
		"servo type 47": {Mode: "basic", ServoPeriodUs: 1000, Channels: []ChannelConfig{{Kind: "servo", Axis: 1, Type: 47}}},
	} {
		if bad.Validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	ok := Config{Mode: "basic", ServoPeriodUs: 1000, Points: 2000, StartSeq: 100, Channels: []ChannelConfig{{Kind: "servo", Axis: 1, Type: 46}}}
	if err := ok.Validate(); err != nil {
		t.Errorf("type 46 with points and start block: %v", err)
	}
}

func TestConfigChannels2(t *testing.T) {
	cfg := Config{Mode: "extended", ServoPeriodUs: 250, Points: 5000, Channels: []ChannelConfig{
		{Kind: "servo", Axis: 1, Type: 3, Shift: 2},
		{Kind: "pmcbit", Signals: []string{"Y0.4", "G8.4"}},
		{Kind: "cnc", Axis: 1, Type: 2},
		{Kind: "pmcbyte", Unit: 2, Signals: []string{"R100"}},
	}, Trigger: Trigger{Seq: 10, Signal: "Y0.4", Up: true}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	ch, order, err := cfg.focasChannels2()
	if err != nil {
		t.Fatal(err)
	}
	// the PMC bit channel goes last on the CNC
	if !slices.Equal(order, []int{0, 2, 3, 1}) {
		t.Errorf("order %v", order)
	}
	for j, c := range ch {
		if c.ChNo != int8(j+1) {
			t.Errorf("entry %d has chno %d", j, c.ChNo)
		}
	}
	if ch[0].Axis != 1 || ch[0].Shift != 0x0202 {
		t.Errorf("servo %+v", ch[0])
	}
	y04 := focas.PmcSignal{Unit: 1, Adr: 2, Bit: 4, No: 0}
	if ch[3].Axis != focas.AxisPmcBit || ch[3].IO[0] != y04 || ch[3].IO[1] != (focas.PmcSignal{Unit: 1, Adr: 0, Bit: 4, No: 8}) {
		t.Errorf("pmc bits %+v", ch[3])
	}
	if ch[1].Axis != 101 || ch[1].Type != 2 {
		t.Errorf("cnc info %+v", ch[1])
	}
	if ch[2].Axis != focas.AxisPmcByte || ch[2].IO[0] != (focas.PmcSignal{Unit: 2, Adr: 5, No: 100}) {
		t.Errorf("pmc byte %+v", ch[2])
	}
	cond, trg := cfg.trigger()
	if cond != focas.TrgSeq|focas.TrgUp || trg.SeqNo != 10 || trg.Pmc != y04 {
		t.Errorf("trigger %#x %+v", cond, trg)
	}
}
