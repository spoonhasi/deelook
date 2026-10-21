package sampling

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"deelook/internal/hub"
)

// Five sessions over three days (in UTC+9, where the page is): listed by
// filters, summed per day, deleted many at once, downloaded as one zip.
func TestSessionsByDay(t *testing.T) {
	st, err := OpenStore(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg := Config{Mode: "basic", ServoPeriodUs: 1000, Channels: []ChannelConfig{{Kind: "servo", Axis: 1, Type: 3, Label: "X"}}}
	kst := time.FixedZone("KST", 9*3600)
	record := func(machine string, started time.Time, status string) int64 {
		t.Helper()
		id, err := st.CreateSession(machine, "m"+machine, cfg, started)
		if err == nil {
			err = st.Add(id, []Chunk{{Channel: 0, Data: []uint16{1, 2}, ReceivedAt: started}}, nil)
		}
		if err == nil {
			err = st.FinishSession(id, status, "", started.Add(time.Second), []int64{2})
		}
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	// 10-20 23:30 and 10-21 00:30 KST are the same UTC day (10-20): days
	// must be counted where the page is
	a := record("1", time.Date(2026, 10, 20, 23, 30, 0, 0, kst), "stopped")
	b := record("1", time.Date(2026, 10, 21, 0, 30, 0, 0, kst), "completed")
	c := record("2", time.Date(2026, 10, 21, 9, 0, 0, 0, kst), "failed")
	d := record("1", time.Date(2026, 10, 22, 8, 0, 0, 0, kst), "stopped")
	e := record("2", time.Date(2026, 10, 22, 16, 0, 0, 0, kst), "stopped")

	days, err := st.Days(SessionFilter{}, 9*60)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, x := range days {
		got = append(got, x.Day+":"+string(rune('0'+x.Count)))
	}
	if want := []string{"2026-10-22:2", "2026-10-21:2", "2026-10-20:1"}; !slices.Equal(got, want) {
		t.Errorf("days in UTC+9: %v, want %v", got, want)
	}
	if days, _ := st.Days(SessionFilter{}, 0); days[len(days)-1].Day != "2026-10-20" || days[len(days)-1].Count != 2 {
		t.Errorf("days in UTC: %+v", days)
	}

	ids := func(f SessionFilter) []int64 {
		t.Helper()
		if err := f.Check(); err != nil {
			t.Fatal(err)
		}
		list, err := st.SessionsWhere(f)
		if err != nil {
			t.Fatal(err)
		}
		var out []int64
		for _, s := range list {
			out = append(out, s.ID)
		}
		return out
	}
	day21 := SessionFilter{Since: "2026-10-21T00:00:00+09:00", Until: "2026-10-22T00:00:00+09:00"}
	if got := ids(day21); !slices.Equal(got, []int64{c, b}) {
		t.Errorf("10-21: %v, want [%d %d]", got, c, b)
	}
	if got := ids(SessionFilter{Machine: "1", Status: []string{"stopped"}}); !slices.Equal(got, []int64{d, a}) {
		t.Errorf("machine 1 stopped: %v, want [%d %d]", got, d, a)
	}
	if got := ids(SessionFilter{Before: d, Limit: 2}); !slices.Equal(got, []int64{c, b}) {
		t.Errorf("the part after %d: %v", d, got)
	}
	if err := (&SessionFilter{Since: "yesterday"}).Check(); err == nil {
		t.Error("took a time that is not RFC 3339")
	}
	if err := (&SessionFilter{Status: []string{"gone"}}).Check(); err == nil {
		t.Error("took an unknown status")
	}

	// the zip holds the files as they are
	m := NewManager(hub.New("", time.Second), st, nil, nil, "", nil)
	w := httptest.NewRecorder()
	m.handleZip(w, httptest.NewRequest("GET", "/api/sampling/sessions/zip?ids="+itoa(a)+","+itoa(e), nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Disposition"), ".zip") {
		t.Fatalf("zip: %d %s", w.Code, w.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	pa, _ := st.File(a)
	if len(zr.File) != 2 || !strings.HasSuffix(pa, zr.File[0].Name) {
		t.Errorf("zip holds %d files, first %q", len(zr.File), zr.File[0].Name)
	}
	w = httptest.NewRecorder()
	m.handleZip(w, httptest.NewRequest("GET", "/api/sampling/sessions/zip?ids=", nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("an empty zip: %d", w.Code)
	}

	// older than 10-22 on machine 1: a and b; what a delete would take, then the delete
	old := SessionFilter{Machine: "1", Until: "2026-10-22T00:00:00+09:00"}
	if err := old.Check(); err != nil {
		t.Fatal(err)
	}
	gone, size, err := st.Finished(old)
	if err != nil || !slices.Equal(gone, []int64{a, b}) || size <= 0 {
		t.Fatalf("finished before 10-22: %v %d %v", gone, size, err)
	}
	res := st.DeleteMany(append(gone, 999))
	if res.Count != 2 || res.Size != size || res.Failed[999] == "" {
		t.Errorf("delete: %+v", res)
	}
	if got := ids(SessionFilter{}); !slices.Equal(got, []int64{e, d, c}) {
		t.Errorf("left: %v", got)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
