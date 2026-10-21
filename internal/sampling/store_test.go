package sampling

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"deelook/internal/focas"
	"deelook/internal/hub"
)

func TestStoreCSV(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenStore(dir, "")
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
	started := time.Date(2026, 10, 14, 16, 34, 15, 0, time.Local)
	id, err := st.CreateSession("2", "bench 31i/B", cfg, started)
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
	// while sampling it is listed but cannot be downloaded or deleted
	if list, _ := st.Sessions("", 10); len(list) != 1 || list[0].Status != "running" {
		t.Errorf("running: %+v", list)
	}
	if _, err := st.File(id); err == nil {
		t.Error("a running session's file handed out")
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
		t.Fatalf("sessions: %+v %v", list, err)
	}
	// one file, standing alone, named for when and where
	path, err := st.File(id)
	if err != nil {
		t.Fatal(err)
	}
	if base := filepath.Base(path); base != "20261014-163415_bench-31i-B_s1.db" || list[0].File != base || list[0].Size == 0 {
		t.Errorf("file %q, listed %q size %d", base, list[0].File, list[0].Size)
	}
	if _, err := os.Stat(path + "-wal"); err == nil {
		t.Error("a finished session's file still has a write-ahead log")
	}
	if st.Storage().Sessions != 1 || st.Storage().Used == 0 {
		t.Errorf("storage %+v", st.Storage())
	}
	// inside, moments are UTC; only the file name is local time
	db, _ := sql.Open("sqlite", dsn(path, false))
	var startedAt, endedAt, receivedAt string
	_ = db.QueryRow(`SELECT started_at, ended_at, (SELECT MIN(received_at) FROM chunks) FROM session`).Scan(&startedAt, &endedAt, &receivedAt)
	db.Close()
	if startedAt != started.UTC().Format(time.RFC3339Nano) || !strings.HasSuffix(endedAt, "Z") || !strings.HasSuffix(receivedAt, "Z") {
		t.Errorf("times in the file: %q %q %q", startedAt, endedAt, receivedAt)
	}

	// a deelook started later finds it; deleting removes the file
	st2, err := OpenStore(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if list, _ := st2.Sessions("", 10); len(list) != 1 || list[0].ID != id {
		t.Errorf("reopened: %+v", list)
	}
	if err := st2.Delete(id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("deleted session's file still there")
	}
	if list, _ := st2.Sessions("", 10); len(list) != 0 {
		t.Errorf("after delete: %+v", list)
	}
	// the deleted session's number is not given out again, even by a
	// deelook started later with no session file left
	st2.Close()
	st3, err := OpenStore(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer st3.Close()
	next, err := st3.CreateSession("2", "bench", cfg, time.Now())
	if err != nil || next != id+1 {
		t.Errorf("next session %d %v, want %d", next, err, id+1)
	}
	if err := st3.FinishSession(next, "stopped", "", time.Now(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestStoreCSVExtended(t *testing.T) {
	st, err := OpenStore(t.TempDir(), "")
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
	// read while still being written: a connection of its own
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
	if err := st.FinishSession(id, "completed", "", now, []int64{3, 3, 3}); err != nil {
		t.Fatal(err)
	}
}

// TestStoreInterrupted: a session a dead process left running is found
// "interrupted" by the next start, its file standing alone again.
func TestStoreInterrupted(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenStore(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateSession("2", "bench", Config{Mode: "basic", ServoPeriodUs: 1000}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Add(id, []Chunk{{Channel: 0, Data: []uint16{1, 2}, ReceivedAt: time.Now()}}, nil); err != nil {
		t.Fatal(err)
	}
	st.Close() // the process dies: no FinishSession

	st, err = OpenStore(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	info, err := st.Session(id)
	if err != nil || info.Status != "interrupted" || info.EndedAt == nil {
		t.Fatalf("after a crash: %+v %v", info, err)
	}
	path, _ := st.File(id)
	if _, err := os.Stat(path + "-wal"); err == nil {
		t.Error("an interrupted session's file still has a write-ahead log")
	}
	if next, _ := st.CreateSession("2", "bench", Config{Mode: "basic", ServoPeriodUs: 1000}, time.Now()); next != id+1 {
		t.Errorf("next id %d after %d", next, id)
	}
}

// TestStoreLegacy moves the sessions of an older single database (machine
// list included) to files of their own and removes it.
func TestStoreLegacy(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "sampling.db")
	old, err := sql.Open("sqlite", dsn(legacy, true))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE sessions (id INTEGER PRIMARY KEY, machine_id TEXT NOT NULL, machine_name TEXT NOT NULL,
			started_at TEXT NOT NULL, ended_at TEXT, status TEXT NOT NULL, error TEXT NOT NULL DEFAULT '',
			config TEXT NOT NULL, samples TEXT NOT NULL DEFAULT '[]')`,
		`CREATE TABLE chunks (session_id INTEGER NOT NULL, channel INTEGER NOT NULL, first INTEGER NOT NULL,
			count INTEGER NOT NULL, received_at TEXT NOT NULL, data BLOB NOT NULL, PRIMARY KEY (session_id, channel, first)) WITHOUT ROWID`,
		`CREATE TABLE events (session_id INTEGER NOT NULL, time_us INTEGER NOT NULL, bits INTEGER NOT NULL, received_at TEXT NOT NULL)`,
		`CREATE TABLE machines (id TEXT PRIMARY KEY, source TEXT NOT NULL, name TEXT NOT NULL, host TEXT NOT NULL,
			port INTEGER NOT NULL, axes TEXT NOT NULL DEFAULT '[]', spindles TEXT NOT NULL DEFAULT '[]',
			paths INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL)`,
		`INSERT INTO sessions VALUES (3, '2', 'bench', '2026-10-14T16:00:00+09:00', '2026-10-14T16:00:05+09:00', 'completed', '',
			'{"mode":"basic","servoPeriodUs":1000,"channels":[{"kind":"servo","axis":1,"type":3,"label":"X"}]}', '[3]')`,
		`INSERT INTO sessions VALUES (7, 'L1', 'lathe', '2026-10-14T17:00:00+09:00', NULL, 'running', '',
			'{"mode":"basic","servoPeriodUs":1000,"channels":[{"kind":"servo","axis":1,"type":3,"label":"X"}]}', '[]')`,
		`INSERT INTO chunks VALUES (3, 0, 0, 3, '2026-10-14T16:00:01+09:00', X'010002000300')`,
		`INSERT INTO chunks VALUES (7, 0, 0, 1, '2026-10-14T17:00:01+09:00', X'0900')`,
		`INSERT INTO events VALUES (3, 1000, 2, '2026-10-14T16:00:01+09:00')`,
		`INSERT INTO machines VALUES ('2', 'hub', 'bench', '198.51.100.77', 8193, '["X1"]', '["S1"]', 2, '2026-10-14T12:00:00Z')`,
	} {
		if _, err := old.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	old.Close()

	sessions := filepath.Join(dir, "sessions")
	st, err := OpenStore(dir, legacy)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := os.Stat(legacy); err == nil {
		t.Error("the old database is still there")
	}
	list, _ := st.Sessions("", 10)
	if len(list) != 2 || list[0].ID != 7 || list[0].Status != "interrupted" || list[0].EndedAt == nil || list[1].ID != 3 || list[1].Events != 1 {
		t.Fatalf("moved sessions %+v", list)
	}
	var b strings.Builder
	if err := st.WriteCSV(&b, 3); err != nil || b.String() != "time_ms,X,trigger\n0.000,1,\n1.000,2,0x0002\n2.000,3,\n" {
		t.Errorf("moved session 3 CSV %q %v", b.String(), err)
	}
	ms, synced, _ := st.takeOldMachines()
	if len(ms) != 1 || ms[0].Host != "198.51.100.77" || synced.IsZero() {
		t.Errorf("machines %+v %v", ms, synced)
	}
	if next, _ := st.CreateSession("2", "bench", Config{Mode: "basic", ServoPeriodUs: 1000}, time.Now()); next != 8 {
		t.Errorf("next id %d, want 8", next)
	}
	// no session file holds a machine address
	files, _ := filepath.Glob(filepath.Join(sessions, "*"))
	for _, f := range files {
		if b, _ := os.ReadFile(f); bytes.Contains(b, []byte("198.51.100.")) {
			t.Errorf("%s holds a machine address", f)
		}
	}
}

func TestStoreLowDisk(t *testing.T) {
	st, err := OpenStore(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if st.lowDisk() != nil {
		t.Error("minimum 0 stopped sampling")
	}
	st.SetMinFree(1 << 62) // more than any disk
	if err := st.lowDisk(); err == nil || !strings.Contains(err.Error(), "disk almost full") {
		t.Errorf("low disk: %v", err)
	}
	st.SetMinFree(1) // any disk has a byte
	if err := st.lowDisk(); err != nil {
		t.Errorf("1 byte minimum: %v", err)
	}
	if s := st.Storage(); s.Free == 0 || s.Total < s.Free {
		t.Errorf("storage %+v", s)
	}
}

func TestSize(t *testing.T) {
	for text, want := range map[string]uint64{"1GB": 1 << 30, "500 MB": 500 << 20, "1.5gb": 3 << 29, "0": 0, "2048": 2048, "10KB": 10240} {
		if n, err := ParseSize(text); err != nil || n != want {
			t.Errorf("ParseSize(%q) = %d %v, want %d", text, n, err, want)
		}
	}
	for _, bad := range []string{"", "lots", "-1GB", "1XB"} {
		if _, err := ParseSize(bad); err == nil {
			t.Errorf("ParseSize(%q) accepted", bad)
		}
	}
	for n, want := range map[uint64]string{1 << 30: "1GB", 3 << 29: "1.5GB", 500 << 20: "500MB", 0: "0", 100: "100"} {
		if got := FormatSize(n); got != want {
			t.Errorf("FormatSize(%d) = %q, want %q", n, got, want)
		}
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

// TestStoreReconcile: the session list follows the sessions folder, and a
// lost deelook.db is rebuilt from the files.
func TestStoreReconcile(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenStore(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{Mode: "basic", ServoPeriodUs: 1000, Channels: []ChannelConfig{{Kind: "servo", Axis: 1, Type: 3, Label: "X"}}}
	record := func(st *Store, started time.Time) int64 {
		t.Helper()
		id, err := st.CreateSession("2", "bench", cfg, started)
		if err == nil {
			err = st.Add(id, []Chunk{{Channel: 0, Data: []uint16{1, 2}, ReceivedAt: started}}, nil)
		}
		if err == nil {
			err = st.FinishSession(id, "stopped", "", started.Add(time.Second), []int64{2})
		}
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	ids := func(st *Store) []int64 {
		t.Helper()
		list, err := st.Sessions("", 100)
		if err != nil {
			t.Fatal(err)
		}
		var out []int64
		for _, s := range list {
			out = append(out, s.ID)
		}
		return out
	}
	a := record(st, time.Now())
	b := record(st, time.Now().Add(time.Second))

	// a file deleted by hand leaves the list
	pa, _ := st.File(a)
	if err := os.Remove(pa); err != nil {
		t.Fatal(err)
	}
	if got := ids(st); !slices.Equal(got, []int64{b}) {
		t.Errorf("after deleting %d by hand: %v", a, got)
	}

	// session files of another deelook put in by hand: listed unless the
	// number is taken (its first session has a's free number, its second
	// b's)
	other, err := OpenStore(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	o1, o2 := record(other, time.Now()), record(other, time.Now())
	p1, _ := other.File(o1)
	p2, _ := other.File(o2)
	other.Close()
	for _, c := range [][2]string{{p1, "imported_s1.db"}, {p2, "imported_s2.db"}} {
		raw, _ := os.ReadFile(c[0])
		if err := os.WriteFile(filepath.Join(dir, "sessions", c[1]), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := ids(st); !slices.Equal(got, []int64{b, o1}) {
		t.Errorf("after putting files in: %v, want [%d %d]", got, b, o1)
	}
	if info, _ := st.Session(o1); info.File != "imported_s1.db" || info.Samples[0] != 2 {
		t.Errorf("imported session %+v", info)
	}

	// once b is deleted, the file left out for b's number is listed
	if err := st.Delete(b); err != nil {
		t.Fatal(err)
	}
	if got := ids(st); !slices.Equal(got, []int64{o2, o1}) {
		t.Errorf("after deleting %d: %v, want [%d %d]", b, got, o2, o1)
	}

	// deelook.db lost: rebuilt from the files, numbering above them
	st.Close()
	for _, f := range []string{indexName, indexName + "-wal", indexName + "-shm"} {
		os.Remove(filepath.Join(dir, f))
	}
	st, err = OpenStore(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if got := ids(st); !slices.Equal(got, []int64{o2, o1}) {
		t.Errorf("rebuilt list %v", got)
	}
	if next := record(st, time.Now()); next != o2+1 {
		t.Errorf("next session %d after a rebuild, want %d", next, o2+1)
	}
}

// The page's stored settings: kept by key, replaced, removed with null,
// and only sensible keys and JSON taken.
func TestStoreUI(t *testing.T) {
	st, err := OpenStore(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if v, err := st.UI("sampling.form.L1"); err != nil || v != nil {
		t.Fatalf("nothing stored: %s %v", v, err)
	}
	for _, v := range []string{`{"mode":"basic","channels":[{"kind":"servo"}]}`, `{"mode":"extended"}`} {
		if err := st.SetUI("sampling.form.L1", json.RawMessage(v)); err != nil {
			t.Fatal(err)
		}
		if got, _ := st.UI("sampling.form.L1"); string(got) != v {
			t.Errorf("stored %s, read %s", v, got)
		}
	}
	if err := st.SetUI("sampling.form.L1", json.RawMessage("null")); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.UI("sampling.form.L1"); got != nil {
		t.Errorf("null left %s", got)
	}
	for _, bad := range []struct{ key, value string }{
		{"../x", `{}`}, {"", `{}`}, {"a b", `{}`}, {"ok", `{not json`},
		{"ok", `"` + strings.Repeat("x", maxUIValue) + `"`},
	} {
		if err := st.SetUI(bad.key, json.RawMessage(bad.value)); err == nil {
			t.Errorf("took key %q value %.20s", bad.key, bad.value)
		}
	}
}

// Every route registers: the mux panics on two patterns that match the
// same path without one being more specific.
func TestRoutes(t *testing.T) {
	st, err := OpenStore(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ms, err := OpenMachines(st)
	if err != nil {
		t.Fatal(err)
	}
	NewManager(hub.New("http://127.0.0.1:1", time.Second), st, ms, nil, "", nil).Routes(http.NewServeMux())
}
