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
	st, err := OpenStore(filepath.Join(dir, "data", "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	path := filepath.Join(dir, "machines.json")
	ms, err := OpenMachines(path, st)
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
	again, err := OpenMachines(path, st)
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

	// the sampling database knows nothing of machine addresses
	st.Close()
	for _, f := range []string{"s.db", "s.db-wal"} {
		if b, err := os.ReadFile(filepath.Join(dir, "data", f)); err == nil && bytes.Contains(b, []byte("10.0.0.")) {
			t.Errorf("%s holds a machine address", f)
		}
	}
}

// TestMachinesFromOldDatabase opens a database where an older deelook kept
// the machine list: the list moves to the file and no address stays in the
// database file.
func TestMachinesFromOldDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "s.db")
	st, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`CREATE TABLE machines (id TEXT PRIMARY KEY, source TEXT NOT NULL, name TEXT NOT NULL,
		host TEXT NOT NULL, port INTEGER NOT NULL, axes TEXT NOT NULL DEFAULT '[]', spindles TEXT NOT NULL DEFAULT '[]',
		paths INTEGER NOT NULL DEFAULT 0, updated_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	synced := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	for _, row := range [][]any{
		{"2", "hub", "bench", "198.51.100.77", 8193, `["X1","X2"]`, `["S1"]`, 2, synced.Format(time.RFC3339Nano)},
		{"L1", "local", "lathe", "198.51.100.78", 8193, `[]`, `[]`, 0, synced.Add(-time.Hour).Format(time.RFC3339Nano)},
	} {
		if _, err := st.db.Exec(`INSERT INTO machines VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, row...); err != nil {
			t.Fatal(err)
		}
	}
	st.Close()

	st, err = OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := OpenMachines(filepath.Join(dir, "machines.json"), st)
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
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'machines'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("machines table still there (%d, %v)", n, err)
	}
	st.Close()
	db, _ := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath))
	db.Close()
	for _, f := range []string{"s.db", "s.db-wal"} {
		if b, err := os.ReadFile(filepath.Join(dir, f)); err == nil && bytes.Contains(b, []byte("198.51.100.")) {
			t.Errorf("%s still holds a machine address", f)
		}
	}
}
