package sampling

import (
	"bufio"
	"cmp"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	_ "modernc.org/sqlite" // pure Go: keeps the build free of cgo
)

// Store keeps each sampling session in its own SQLite file in one folder,
// named <start>_<machine>_s<id>.db: a table with the session, one with the
// samples and one with the trigger marks. A finished session's file is
// complete on its own (no write-ahead log next to it), so it can be copied
// or downloaded and handed to whoever needs the data; deleting a session is
// deleting its file. No file holds machine addresses (machines.go).
//
// Samples go in chunks: one row per channel per read from the CNC, the raw
// 16-bit words as a little-endian blob. Sample i of a channel was taken i
// periods after the session started (the CNC samples at a fixed period, so
// this is more exact than the PC clock; received_at records when the chunk
// arrived).
type Store struct {
	dir     string // the session files
	minFree uint64 // sampling stops below this much free disk space (0: never)

	mu    sync.Mutex
	paths map[int64]string     // every session's file
	open  map[int64]*sql.DB    // the sessions being written
	cache map[string]cachedRow // session rows of files read, while they stay the same
	next  int64                // the next session id

	oldMachines []machine // the machine list found in an older single database
	oldSynced   time.Time
}

type cachedRow struct {
	mod  time.Time
	size int64
	info SessionInfo
}

const sessionSchema = `
CREATE TABLE IF NOT EXISTS session (    -- one row
	id           INTEGER PRIMARY KEY,
	machine_id   TEXT NOT NULL,
	machine_name TEXT NOT NULL,
	started_at   TEXT NOT NULL,             -- RFC 3339, server clock, when sampling started
	ended_at     TEXT,                      -- NULL while running
	status       TEXT NOT NULL,             -- running | stopped | completed | failed | interrupted
	error        TEXT NOT NULL DEFAULT '',
	config       TEXT NOT NULL,             -- Config as JSON
	samples      TEXT NOT NULL DEFAULT '[]' -- samples per channel, JSON, set when the session ends
);
CREATE TABLE IF NOT EXISTS chunks (
	channel     INTEGER NOT NULL,           -- index into config.channels
	first       INTEGER NOT NULL,           -- index of the chunk's first sample
	count       INTEGER NOT NULL,
	received_at TEXT NOT NULL,
	data        BLOB NOT NULL,              -- count raw 16-bit words, little endian
	PRIMARY KEY (channel, first)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS events (
	time_us     INTEGER NOT NULL,           -- from the first sample
	bits        INTEGER NOT NULL,           -- cnc_sdtreadsmpl2 trigger bits
	received_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS events_time ON events(time_us);
`

var sessionFile = regexp.MustCompile(`_s(\d+)\.db$`)

// OpenStore opens the folder of session files dir, creating it if needed.
// Sessions left "running" by a process that died are marked "interrupted".
// legacy names the single database an older deelook kept every session in
// ("" or a missing file: none): its sessions are moved to files of their
// own and, once each is checked, the old database is removed.
func OpenStore(dir, legacy string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, paths: map[int64]string{}, open: map[int64]*sql.DB{}, cache: map[string]cachedRow{}, next: 1}
	if err := s.scan(); err != nil {
		return nil, err
	}
	if legacy != "" {
		if _, err := os.Stat(legacy); err == nil {
			if err := s.migrate(legacy); err != nil {
				return nil, fmt.Errorf("moving the sessions of %s to files of their own: %w", legacy, err)
			}
		}
	}
	return s, nil
}

// scan finds the session files and marks the ones a dead process left
// running.
func (s *Store) scan() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		m := sessionFile.FindStringSubmatch(e.Name())
		if m == nil || e.IsDir() {
			continue
		}
		id, _ := strconv.ParseInt(m[1], 10, 64)
		path := filepath.Join(s.dir, e.Name())
		s.paths[id] = path
		s.next = max(s.next, id+1)
		info, err := s.read(path)
		if err != nil {
			continue // not a session file deelook can read: listed nowhere
		}
		if info.Status == "running" {
			if err := closeInterrupted(path); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			delete(s.cache, path)
		}
	}
	return nil
}

// closeInterrupted marks a session a dead process left running and makes
// its file complete on its own again.
func closeInterrupted(path string) error {
	db, err := sql.Open("sqlite", dsn(path, true))
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`UPDATE session SET status = 'interrupted', ended_at = COALESCE(ended_at,
		(SELECT MAX(received_at) FROM chunks), started_at) WHERE status = 'running'`); err != nil {
		return err
	}
	_, err = db.Exec(`PRAGMA journal_mode = DELETE`)
	return err
}

// dsn is the sqlite address of a session file, for writing (write-ahead
// log while sampling) or reading.
func dsn(path string, write bool) string {
	p := filepath.ToSlash(path)
	p = strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23").Replace(p)
	if write {
		return "file:" + p + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	}
	return "file:" + p + "?mode=ro&_pragma=busy_timeout(5000)"
}

// Close closes the files still being written (deelook stops sampling
// before, so normally none).
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, db := range s.open {
		db.Close()
		delete(s.open, id)
	}
	return nil
}

// SetMinFree makes sampling stop when the session files' disk has less free
// space than n bytes (0: never).
func (s *Store) SetMinFree(n uint64) { s.minFree = n }

// Storage is the session folder's disk use for the browser.
type Storage struct {
	Dir      string `json:"dir"`
	Free     uint64 `json:"free"`
	Total    uint64 `json:"total"`
	Used     int64  `json:"used"` // the session files together
	Sessions int    `json:"sessions"`
	MinFree  uint64 `json:"minFree"`
}

func (s *Store) Storage() Storage {
	st := Storage{Dir: s.dir, MinFree: s.minFree}
	st.Free, st.Total, _ = diskSpace(s.dir)
	s.mu.Lock()
	defer s.mu.Unlock()
	st.Sessions = len(s.paths)
	for _, p := range s.paths {
		st.Used += fileSize(p)
	}
	return st
}

// lowDisk says why sampling must not go on: less free space than the
// minimum on the session files' disk.
func (s *Store) lowDisk() error {
	if s.minFree == 0 {
		return nil
	}
	free, _, err := diskSpace(s.dir)
	if err != nil || free >= s.minFree {
		return nil
	}
	return fmt.Errorf("disk almost full: %s free where the sessions are kept, minimum %s (minfree in %s)",
		formatSize(free), formatSize(s.minFree), "deelook.toml")
}

// fileSize is a session file's size with its write-ahead log.
func fileSize(path string) int64 {
	var n int64
	for _, p := range []string{path, path + "-wal"} {
		if fi, err := os.Stat(p); err == nil {
			n += fi.Size()
		}
	}
	return n
}

// safeName keeps a machine name usable in a file name.
func safeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
		if b.Len() >= 40 {
			break
		}
	}
	if b.Len() == 0 {
		return "machine"
	}
	return b.String()
}

// Chunk is data read from the CNC for one channel.
type Chunk struct {
	Channel    int
	First      int64
	Data       []uint16
	ReceivedAt time.Time
}

// SessionInfo is a session as the browser lists it.
type SessionInfo struct {
	ID          int64      `json:"id"`
	Machine     string     `json:"machine"`
	MachineName string     `json:"machineName"`
	StartedAt   time.Time  `json:"startedAt"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
	Status      string     `json:"status"`
	Error       string     `json:"error,omitempty"`
	Config      Config     `json:"config"`
	Samples     []int64    `json:"samples"`
	Events      int        `json:"events"` // trigger marks recorded
	File        string     `json:"file"`   // the session's file name
	Size        int64      `json:"size"`   // its size in bytes
}

// CreateSession starts a session's file and keeps it open for writing.
func (s *Store) CreateSession(machine, name string, cfg Config, started time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.next
	path := filepath.Join(s.dir, fmt.Sprintf("%s_%s_s%d.db", started.Local().Format("20060102-150405"), safeName(name), id))
	db, err := sql.Open("sqlite", dsn(path, true))
	if err == nil {
		db.SetMaxOpenConns(1) // one writer; readers open their own (withDB)
		_, err = db.Exec(sessionSchema)
	}
	if err == nil {
		c, _ := json.Marshal(cfg)
		_, err = db.Exec(`INSERT INTO session (id, machine_id, machine_name, started_at, status, config) VALUES (?, ?, ?, ?, 'running', ?)`,
			id, machine, name, started.Format(time.RFC3339Nano), string(c))
	}
	if err != nil {
		if db != nil {
			db.Close()
		}
		os.Remove(path)
		return 0, fmt.Errorf("session file %s: %w", path, err)
	}
	s.next++
	s.paths[id] = path
	s.open[id] = db
	return id, nil
}

func (s *Store) writer(id int64) (*sql.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if db := s.open[id]; db != nil {
		return db, nil
	}
	return nil, fmt.Errorf("session %d is not being written", id)
}

// Add writes chunks and trigger marks in one transaction.
func (s *Store) Add(id int64, chunks []Chunk, events []Event) error {
	db, err := s.writer(id)
	if err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.Prepare(`INSERT INTO chunks (channel, first, count, received_at, data) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, c := range chunks {
		blob := make([]byte, 2*len(c.Data))
		for i, w := range c.Data {
			binary.LittleEndian.PutUint16(blob[2*i:], w)
		}
		if _, err := st.Exec(c.Channel, c.First, len(c.Data), c.ReceivedAt.Format(time.RFC3339Nano), blob); err != nil {
			return err
		}
	}
	for _, e := range events {
		if _, err := tx.Exec(`INSERT INTO events (time_us, bits, received_at) VALUES (?, ?, ?)`,
			e.TimeUs, e.Bits, e.At.Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// FinishSession records how a session ended and closes its file, folding
// the write-ahead log in so the file stands alone.
func (s *Store) FinishSession(id int64, status, errText string, ended time.Time, samples []int64) error {
	db, err := s.writer(id)
	if err != nil {
		return err
	}
	n, _ := json.Marshal(samples)
	_, err = db.Exec(`UPDATE session SET status = ?, error = ?, ended_at = ?, samples = ?`,
		status, errText, ended.Format(time.RFC3339Nano), string(n))
	// back to a plain journal: the file then stands alone. With a reader
	// still open this waits for it (busy_timeout) or fails; SQLite folds
	// the log in anyway when the last connection closes.
	_, _ = db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	_, _ = db.Exec(`PRAGMA journal_mode = DELETE`)
	cerr := db.Close()
	s.mu.Lock()
	delete(s.open, id)
	delete(s.cache, s.paths[id])
	s.mu.Unlock()
	if err == nil {
		err = cerr
	}
	return err
}

const sessionColumns = `id, machine_id, machine_name, started_at, ended_at, status, error, config, samples,
	(SELECT COUNT(*) FROM events)`

func scanSession(row interface{ Scan(...any) error }) (SessionInfo, error) {
	var si SessionInfo
	var started, cfg, samples string
	var ended sql.NullString
	if err := row.Scan(&si.ID, &si.Machine, &si.MachineName, &started, &ended, &si.Status, &si.Error, &cfg, &samples, &si.Events); err != nil {
		return si, err
	}
	si.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
	if ended.Valid {
		t, _ := time.Parse(time.RFC3339Nano, ended.String)
		si.EndedAt = &t
	}
	_ = json.Unmarshal([]byte(cfg), &si.Config)
	_ = json.Unmarshal([]byte(samples), &si.Samples)
	return si, nil
}

// read reads a closed session file's row, through the cache; s.mu is held
// (or s not shared yet).
func (s *Store) read(path string) (SessionInfo, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return SessionInfo{}, err
	}
	if c, ok := s.cache[path]; ok && c.mod.Equal(fi.ModTime()) && c.size == fi.Size() {
		return c.info, nil
	}
	db, err := sql.Open("sqlite", dsn(path, false))
	if err != nil {
		return SessionInfo{}, err
	}
	defer db.Close()
	info, err := scanSession(db.QueryRow(`SELECT ` + sessionColumns + ` FROM session`))
	if err != nil {
		return info, err
	}
	info.File, info.Size = filepath.Base(path), fileSize(path)
	s.cache[path] = cachedRow{mod: fi.ModTime(), size: fi.Size(), info: info}
	return info, nil
}

// info is a session's row: live for one being written; s.mu is held.
func (s *Store) info(id int64) (SessionInfo, error) {
	path, ok := s.paths[id]
	if !ok {
		return SessionInfo{}, fmt.Errorf("no session %d", id)
	}
	if db := s.open[id]; db != nil {
		info, err := scanSession(db.QueryRow(`SELECT ` + sessionColumns + ` FROM session`))
		info.File, info.Size = filepath.Base(path), fileSize(path)
		return info, err
	}
	return s.read(path)
}

// Sessions lists the newest sessions first, of one machine or ("") all.
func (s *Store) Sessions(machine string, limit int) ([]SessionInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]int64, 0, len(s.paths))
	for id := range s.paths {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b int64) int { return cmp.Compare(b, a) })
	out := []SessionInfo{}
	for _, id := range ids {
		info, err := s.info(id)
		if err != nil || (machine != "" && info.Machine != machine) {
			continue
		}
		out = append(out, info)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (s *Store) Session(id int64) (SessionInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.info(id)
}

// File is a finished session's file, for download.
func (s *Store) File(id int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.open[id] != nil {
		return "", errors.New("the session is still sampling")
	}
	path, ok := s.paths[id]
	if !ok {
		return "", fmt.Errorf("no session %d", id)
	}
	return path, nil
}

// Delete removes a finished session's file.
func (s *Store) Delete(id int64) error {
	path, err := s.File(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, p := range []string{path + "-wal", path + "-shm", path + "-journal"} {
		os.Remove(p)
	}
	delete(s.paths, id)
	delete(s.cache, path)
	return nil
}

// withDB runs fn on a session's file opened for reading: a connection of
// its own even while the session is being written (the write-ahead log lets
// both go on), so a long CSV export never holds up the writer.
func (s *Store) withDB(id int64, fn func(db *sql.DB) error) error {
	s.mu.Lock()
	path, ok := s.paths[id]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("no session %d", id)
	}
	db, err := sql.Open("sqlite", dsn(path, false))
	if err != nil {
		return err
	}
	defer db.Close()
	return fn(db)
}

// maxLocalSessionID is the highest n of the machine ids L<n> (machines
// added in deelook) that recorded sessions keep, 0 if none.
func (s *Store) maxLocalSessionID() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for id := range s.paths {
		if info, err := s.info(id); err == nil && strings.HasPrefix(info.Machine, "L") {
			if k, err := strconv.Atoi(info.Machine[1:]); err == nil {
				n = max(n, k)
			}
		}
	}
	return n, nil
}

// takeOldMachines hands over the machine list an older single database
// held (machines.go keeps it now), once.
func (s *Store) takeOldMachines() ([]machine, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ms, at := s.oldMachines, s.oldSynced
	s.oldMachines = nil
	return ms, at, nil
}

// migrate moves the sessions of an older single database to files of their
// own (keeping their ids where free), takes its machine list, and removes
// it once every session's samples and marks are found in the new file.
func (s *Store) migrate(legacy string) error {
	old, err := sql.Open("sqlite", dsn(legacy, true))
	if err != nil {
		return err
	}
	defer old.Close()
	has := func(table string) bool {
		var n int
		_ = old.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&n)
		return n > 0
	}
	if has("machines") {
		rows, err := old.Query(`SELECT id, source, name, host, port, axes, spindles, paths, updated_at FROM machines`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var mc machine
			var axes, spindles, at string
			if err := rows.Scan(&mc.ID, &mc.Source, &mc.Name, &mc.Host, &mc.Port, &axes, &spindles, &mc.Paths, &at); err != nil {
				rows.Close()
				return err
			}
			_ = json.Unmarshal([]byte(axes), &mc.Axes)
			_ = json.Unmarshal([]byte(spindles), &mc.Spindles)
			if t, _ := time.Parse(time.RFC3339Nano, at); mc.Source == "hub" && t.After(s.oldSynced) {
				s.oldSynced = t
			}
			s.oldMachines = append(s.oldMachines, mc)
		}
		rows.Close()
	}
	if has("sessions") {
		var ids []int64
		rows, err := old.Query(`SELECT id FROM sessions ORDER BY id`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
		for _, oldID := range ids {
			if err := s.migrateSession(legacy, old, oldID); err != nil {
				return fmt.Errorf("session %d: %w", oldID, err)
			}
		}
	}
	old.Close()
	for _, p := range []string{legacy, legacy + "-wal", legacy + "-shm"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (s *Store) migrateSession(legacy string, old *sql.DB, oldID int64) error {
	var name, started string
	if err := old.QueryRow(`SELECT machine_name, started_at FROM sessions WHERE id = ?`, oldID).Scan(&name, &started); err != nil {
		return err
	}
	at, _ := time.Parse(time.RFC3339Nano, started)
	id := oldID
	if _, taken := s.paths[id]; taken {
		id = s.next
	}
	path := filepath.Join(s.dir, fmt.Sprintf("%s_%s_s%d.db", at.Local().Format("20060102-150405"), safeName(name), id))
	db, err := sql.Open("sqlite", strings.Replace(dsn(path, true), "journal_mode(WAL)", "journal_mode(DELETE)", 1))
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1) // ATTACH holds for one connection
	steps := []struct {
		q    string
		args []any
	}{
		{sessionSchema, nil},
		{`ATTACH DATABASE ? AS old`, []any{legacy}},
		{`INSERT INTO session SELECT ?, machine_id, machine_name, started_at, ended_at,
			CASE status WHEN 'running' THEN 'interrupted' ELSE status END, error, config, samples
			FROM old.sessions WHERE id = ?`, []any{id, oldID}},
		{`INSERT INTO chunks SELECT channel, first, count, received_at, data FROM old.chunks WHERE session_id = ?`, []any{oldID}},
		{`INSERT INTO events SELECT time_us, bits, received_at FROM old.events WHERE session_id = ?`, []any{oldID}},
		{`UPDATE session SET ended_at = COALESCE(ended_at, (SELECT MAX(received_at) FROM chunks), started_at)`, nil},
	}
	for _, st := range steps {
		if _, err := db.Exec(st.q, st.args...); err != nil {
			db.Close()
			os.Remove(path)
			return err
		}
	}
	// the copy must hold all of it before the old database goes
	var want, got, wantE, gotE int64
	_ = old.QueryRow(`SELECT COALESCE(SUM(count), 0) FROM chunks WHERE session_id = ?`, oldID).Scan(&want)
	_ = old.QueryRow(`SELECT COUNT(*) FROM events WHERE session_id = ?`, oldID).Scan(&wantE)
	_ = db.QueryRow(`SELECT COALESCE(SUM(count), 0) FROM chunks`).Scan(&got)
	_ = db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&gotE)
	if want != got || wantE != gotE {
		db.Close()
		os.Remove(path)
		return fmt.Errorf("copied %d of %d samples, %d of %d marks", got, want, gotE, wantE)
	}
	if _, err := db.Exec(`DETACH DATABASE old`); err != nil {
		return err
	}
	s.paths[id] = path
	s.next = max(s.next, id+1)
	return nil
}

// channelReader walks one channel's samples in order across its chunks.
type channelReader struct {
	rows *sql.Rows
	buf  []byte
	next int64 // index of buf's first sample
}

func (r *channelReader) close() { r.rows.Close() }

// at returns sample idx (idx only grows between calls), or ok=false when
// the channel has no sample there.
func (r *channelReader) at(idx int64) (v uint16, ok bool) {
	for {
		if n := int64(len(r.buf) / 2); idx < r.next+n {
			if idx < r.next {
				return 0, false // a gap before this chunk
			}
			return binary.LittleEndian.Uint16(r.buf[2*(idx-r.next):]), true
		}
		if !r.rows.Next() {
			return 0, false
		}
		if r.rows.Scan(&r.next, &r.buf) != nil {
			return 0, false
		}
	}
}

// WriteCSV writes a session as CSV: time in ms from the first sample, then
// the channels: servo and spindle data as signed 16-bit integers, PMC bytes
// and words and CNC information unsigned, and PMC bit channels as one 0/1
// column per signal; last the trigger bits the CNC marked at that time.
// Rows follow the shortest period; a slower channel fills only its own rows.
func (s *Store) WriteCSV(w io.Writer, id int64) error {
	si, err := s.Session(id)
	if err != nil {
		return err
	}
	return s.withDB(id, func(db *sql.DB) error { return writeCSV(db, si.Config, w) })
}

func writeCSV(db *sql.DB, cfg Config, w io.Writer) error {
	n := len(cfg.Channels)
	if n == 0 {
		return errors.New("session has no channels")
	}
	readers := make([]*channelReader, n)
	base := 0
	for i := range n {
		rows, err := db.Query(`SELECT first, data FROM chunks WHERE channel = ? ORDER BY first`, i)
		if err != nil {
			return err
		}
		readers[i] = &channelReader{rows: rows}
		defer readers[i].close()
		if p := cfg.PeriodUs(i); base == 0 || p < base {
			base = p
		}
	}
	marks := map[int64]uint16{}
	rows, err := db.Query(`SELECT time_us, bits FROM events`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var t int64
		var b uint16
		if rows.Scan(&t, &b) == nil {
			marks[t/int64(base)*int64(base)] |= b
		}
	}
	rows.Close()

	bw := bufio.NewWriterSize(w, 1<<16)
	bw.WriteString("time_ms")
	for _, ch := range cfg.Channels {
		if ch.Kind == "pmcbit" {
			for _, sig := range ch.Signals {
				bw.WriteString("," + csvCell(ch.Label+":"+sig))
			}
			continue
		}
		bw.WriteString("," + csvCell(ch.Label))
	}
	if len(marks) > 0 {
		bw.WriteString(",trigger")
	}
	bw.WriteString("\n")
	for k := int64(0); ; k++ {
		us := k * int64(base)
		has := false
		line := strconv.FormatFloat(float64(us)/1000, 'f', 3, 64)
		for i, ch := range cfg.Channels {
			cells := 1
			if ch.Kind == "pmcbit" {
				cells = len(ch.Signals)
			}
			p := int64(cfg.PeriodUs(i))
			v, ok := uint16(0), false
			if us%p == 0 {
				v, ok = readers[i].at(us / p)
			}
			for b := range cells {
				line += ","
				switch {
				case !ok:
				case ch.Kind == "pmcbit":
					line += strconv.Itoa(int(v >> b & 1))
				case cfg.Signed(i):
					line += strconv.Itoa(int(int16(v)))
				default:
					line += strconv.Itoa(int(v))
				}
			}
			has = has || ok
		}
		if !has {
			break
		}
		if len(marks) > 0 {
			line += ","
			if b, ok := marks[us]; ok {
				line += fmt.Sprintf("0x%04x", b)
			}
		}
		bw.WriteString(line)
		bw.WriteString("\n")
	}
	return bw.Flush()
}

// csvCell quotes a header cell when it needs it.
func csvCell(s string) string {
	if strings.ContainsAny(s, ",\"\r\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}
