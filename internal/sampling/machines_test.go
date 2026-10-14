package sampling

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"deelook/internal/hub"
)

func TestMachinesFile(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenStore(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ms, err := OpenMachines(st)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	hubList := []hub.Machine{
		{ID: 2, Name: "bench-31i", Protocol: fanucProtocol, Host: "10.0.0.2", Port: 8193},
		{ID: 5, Name: "bench-840d", Protocol: "nc_opcua_siemens", Host: "10.0.0.5", Port: 4840},
		{ID: 1, Name: "sim", Protocol: fanucProtocol, Host: "10.0.0.1", Port: 8193},
	}
	if err := ms.syncHub(hubList, now); err != nil {
		t.Fatal(err)
	}
	if err := ms.setNames("2", []string{"X1", "X2"}, []string{"S1"}, 2); err != nil {
		t.Fatal(err)
	}
	id, err := ms.addLocal("lathe", "10.0.0.9", 8193, []string{"X", "Z"}, []string{"S1"}, 1, 0)
	if err != nil || id != "L1" {
		t.Fatalf("addLocal %q %v", id, err)
	}
	// the hub drops machine 1 and moves machine 2: its names stay, 1 goes
	later := now.Add(time.Minute)
	if err := ms.syncHub([]hub.Machine{{ID: 2, Name: "bench-31i", Protocol: fanucProtocol, Host: "10.0.0.22", Port: 8193}}, later); err != nil {
		t.Fatal(err)
	}

	// a deelook started later reads the same from the file
	again, err := OpenMachines(st)
	if err != nil {
		t.Fatal(err)
	}
	list := again.list()
	if len(list) != 2 || list[0].ID != "2" || list[0].Host != "10.0.0.22" || !slices.Equal(list[0].Axes, []string{"X1", "X2"}) ||
		list[1].ID != "L1" || list[1].Source != "local" || list[1].Paths != 1 {
		t.Fatalf("machines %+v", list)
	}
	if !again.hubSyncedAt().Equal(later) {
		t.Errorf("hub synced at %v, want %v", again.hubSyncedAt(), later)
	}
	if again.deleteLocal("2") == nil {
		t.Error("deleted a hub machine")
	}
	// L1 recorded a session: after it is removed, a new machine is L2
	if _, err := st.CreateSession("L1", "lathe", Config{Mode: "basic"}, now); err != nil {
		t.Fatal(err)
	}
	if err := again.deleteLocal("L1"); err != nil {
		t.Error(err)
	}
	taken, _ := st.maxLocalSessionID()
	if id, _ := again.addLocal("mill", "10.0.0.8", 8193, nil, nil, 0, taken); id != "L2" {
		t.Errorf("next local id %q", id)
	}

	// the session files know nothing of machine addresses
	st.Close()
	files, _ := filepath.Glob(filepath.Join(dir, "sessions", "*"))
	if len(files) == 0 {
		t.Fatal("no session file")
	}
	for _, f := range files {
		if b, err := os.ReadFile(f); err == nil && bytes.Contains(b, []byte("10.0.0.")) {
			t.Errorf("%s holds a machine address", f)
		}
	}
}

// TestMachinesFromOldDatabase: the machine list an older single database
// held ends up in the machine list file, and that database is gone.
func TestMachinesFromOldDatabase(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "sampling.db")
	old, err := sql.Open("sqlite", dsn(legacy, true))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`CREATE TABLE machines (id TEXT PRIMARY KEY, source TEXT NOT NULL, name TEXT NOT NULL,
		host TEXT NOT NULL, port INTEGER NOT NULL, axes TEXT NOT NULL DEFAULT '[]', spindles TEXT NOT NULL DEFAULT '[]',
		paths INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	synced := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	for _, row := range [][]any{
		{"2", "hub", "bench", "198.51.100.77", 8193, `["X1","X2"]`, `["S1"]`, 2, synced.Format(time.RFC3339Nano)},
		{"L1", "local", "lathe", "198.51.100.78", 8193, `[]`, `[]`, 0, synced.Add(-time.Hour).Format(time.RFC3339Nano)},
	} {
		if _, err := old.Exec(`INSERT INTO machines VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, row...); err != nil {
			t.Fatal(err)
		}
	}
	old.Close()

	st, err := OpenStore(dir, legacy)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ms, err := OpenMachines(st)
	if err != nil {
		t.Fatal(err)
	}
	list := ms.list()
	if len(list) != 2 || list[0].Host != "198.51.100.77" || !slices.Equal(list[0].Axes, []string{"X1", "X2"}) || list[1].ID != "L1" {
		t.Fatalf("moved machines %+v", list)
	}
	if !ms.hubSyncedAt().Equal(synced) {
		t.Errorf("hub synced at %v", ms.hubSyncedAt())
	}
	for _, f := range []string{legacy, legacy + "-wal"} {
		if _, err := os.Stat(f); err == nil {
			t.Errorf("%s still there", f)
		}
	}
}

// TestMachinesFromJSON: the machines.json of earlier builds moves into
// deelook.db and is removed.
func TestMachinesFromJSON(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenStore(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	old := filepath.Join(dir, "machines.json")
	const text = `{"hubSyncedAt":"2026-10-14T12:00:00Z","machines":[{"id":"L1","source":"local","name":"lathe","host":"198.51.100.9","port":8193}]}`
	if err := os.WriteFile(old, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	ms, err := OpenMachines(st, old)
	if err != nil {
		t.Fatal(err)
	}
	if list := ms.list(); len(list) != 1 || list[0].Host != "198.51.100.9" {
		t.Fatalf("imported %+v", list)
	}
	if _, err := os.Stat(old); err == nil {
		t.Error("machines.json still there")
	}
	// read back from deelook.db alone
	again, err := OpenMachines(st, old)
	if err != nil {
		t.Fatal(err)
	}
	if list := again.list(); len(list) != 1 || list[0].ID != "L1" || again.hubSyncedAt().IsZero() {
		t.Errorf("from deelook.db %+v %v", list, again.hubSyncedAt())
	}
	// a local machine's number is not given out again after it is removed
	if err := again.deleteLocal("L1"); err != nil {
		t.Fatal(err)
	}
	if id, _ := again.addLocal("mill", "198.51.100.10", 8193, nil, nil, 0, 0); id != "L2" {
		t.Errorf("next local machine %q", id)
	}
}
