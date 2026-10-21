package sampling

import (
	"bufio"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	_ "modernc.org/sqlite" // pure Go: keeps the build free of cgo
)

// Store keeps the sampling sessions: each in its own SQLite file in the
// sessions folder, named <start>_<machine>_s<id>.db (a table with the
// session, one with the samples and one with the trigger marks), and a
// list of them in deelook.db beside that folder.
//
// The session files are the record. A finished one is complete on its own
// (no write-ahead log next to it), so it can be copied or downloaded and
// handed to whoever needs the data; deleting a session is deleting its
// file; no file holds machine addresses. deelook.db is deelook's own: the
// session list (a copy of each file's session row, kept in step with the
// folder, so files deleted or put there by hand show up and a lost
// deelook.db is rebuilt from the files), the highest session number given
// out, and the machine list (machines.go).
//
// Samples go in chunks: one row per channel per read from the CNC, the raw
// 16-bit words as a little-endian blob. Sample i of a channel was taken i
// periods after the session started (the CNC samples at a fixed period, so
// this is more exact than the PC clock; received_at records when the chunk
// arrived).
type Store struct {
	dir     string  // the session files
	index   *sql.DB // deelook.db
	minFree uint64  // sampling stops below this much free disk space (0: never)

	mu      sync.Mutex
	open    map[int64]*sql.DB // the sessions being written
	next    int64             // the next session number
	ignored map[string]string // files left out of the list (and said so), by name: their modification time

	oldMachines []machine // the machine list found in an older single database
	oldSynced   time.Time
}

// indexName is deelook's own database in the data folder.
const indexName = "deelook.db"

const indexSchema = `
CREATE TABLE IF NOT EXISTS sessions (    -- a copy of each session file's row, and the file
	id           INTEGER PRIMARY KEY,
	file         TEXT NOT NULL UNIQUE,       -- its name in the sessions folder
	machine_id   TEXT NOT NULL,
	machine_name TEXT NOT NULL,
	started_at   TEXT NOT NULL,
	ended_at     TEXT,
	status       TEXT NOT NULL,
	error        TEXT NOT NULL DEFAULT '',
	config       TEXT NOT NULL,
	samples      TEXT NOT NULL DEFAULT '[]',
	events       INTEGER NOT NULL DEFAULT 0,
	size         INTEGER NOT NULL DEFAULT 0, -- the file as last read: its size and
	modified     TEXT NOT NULL DEFAULT ''    -- modification time, to see it changed
);
CREATE TABLE IF NOT EXISTS machines (    -- machines.go
	id       TEXT PRIMARY KEY,               -- hub machine_id, or L1, L2... for machines added in deelook
	source   TEXT NOT NULL,                  -- hub | local
	name     TEXT NOT NULL,
	host     TEXT NOT NULL,
	port     INTEGER NOT NULL,
	axes     TEXT NOT NULL DEFAULT '[]',     -- servo axis names, logical order (all paths)
	spindles TEXT NOT NULL DEFAULT '[]',
	paths    INTEGER NOT NULL DEFAULT 0      -- 0: not known
);
CREATE TABLE IF NOT EXISTS meta (
	key   TEXT PRIMARY KEY,                  -- last_session_id, last_local_machine, hub_synced_at
	value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS ui (          -- what the sampling page was set to, kept for it (ui.go)
	key        TEXT PRIMARY KEY,             -- sampling.pick, sampling.form.<machine>
	value      TEXT NOT NULL,                -- JSON, as the page sent it
	updated_at TEXT NOT NULL
);
`

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

// OpenStore opens the data folder dataDir: deelook.db and the sessions
// folder, creating them if needed, and brings the session list in step
// with the files. legacy names the single database an older deelook kept
// every session in ("" or a missing file: none): its sessions are moved to
// files of their own and, once each is checked, the old database is removed.
func OpenStore(dataDir, legacy string) (*Store, error) {
	dir := filepath.Join(dataDir, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, indexName)
	index, err := sql.Open("sqlite", dsn(path, true))
	if err == nil {
		_, err = index.Exec(indexSchema)
	}
	if err != nil {
		if index != nil {
			index.Close()
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	s := &Store{dir: dir, index: index, open: map[int64]*sql.DB{}, next: 1, ignored: map[string]string{}}
	fail := func(err error) (*Store, error) {
		index.Close()
		return nil, err
	}
	if v, ok := s.meta("last_session_id"); ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			s.next = n + 1
		}
	}
	// earlier builds kept the highest number in a file in the sessions folder
	if b, err := os.ReadFile(filepath.Join(dir, "last-session-id")); err == nil {
		if n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil {
			s.noteID(n)
		}
		os.Remove(filepath.Join(dir, "last-session-id"))
	}
	if err := s.reconcile(); err != nil {
		return fail(err)
	}
	if legacy != "" {
		if _, err := os.Stat(legacy); err == nil {
			if err := s.migrate(legacy); err != nil {
				return fail(fmt.Errorf("moving the sessions of %s to files of their own: %w", legacy, err))
			}
			if err := s.reconcile(); err != nil {
				return fail(err)
			}
		}
	}
	return s, nil
}

func (s *Store) meta(key string) (string, bool) {
	var v string
	err := s.index.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	return v, err == nil
}

func (s *Store) setMeta(key, value string) error {
	_, err := s.index.Exec(`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// noteID records id as given out, so a deleted session's number never
// comes back; s.mu is held (or s not shared yet).
func (s *Store) noteID(id int64) {
	if id < s.next {
		return
	}
	s.next = id + 1
	if err := s.setMeta("last_session_id", strconv.FormatInt(id, 10)); err != nil {
		log.Printf("%s: %v", indexName, err)
	}
}

// reconcile brings the session list in step with the sessions folder:
// files new or changed since last read are (re)read, entries whose file is
// gone are dropped, and sessions a dead process left running are marked
// interrupted. s.mu is held (or s not shared yet).
func (s *Store) reconcile() error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	type entry struct {
		id       int64
		size     int64
		modified string
	}
	listed := map[string]entry{}
	byID := map[int64]string{}
	rows, err := s.index.Query(`SELECT id, file, size, modified FROM sessions`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var e entry
		var file string
		if rows.Scan(&e.id, &file, &e.size, &e.modified) == nil {
			listed[file], byID[e.id] = e, file
		}
	}
	rows.Close()

	present := map[string]bool{}
	for _, de := range entries {
		name := de.Name()
		if de.IsDir() || !sessionFile.MatchString(name) {
			continue
		}
		present[name] = true
		e, ok := listed[name]
		if ok && s.open[e.id] != nil {
			continue // being written: CreateSession and FinishSession keep its entry
		}
		fi, err := de.Info()
		if err != nil || ok && e.size == fi.Size() && e.modified == stamp(fi.ModTime()) ||
			!ok && s.ignored[name] == stamp(fi.ModTime()) {
			continue
		}
		s.ignored[name] = stamp(fi.ModTime()) // until listed below
		path := filepath.Join(s.dir, name)
		info, err := readSession(path)
		if err == nil && info.Status == "running" { // a dead process left it so
			if err = closeInterrupted(path); err == nil {
				info, err = readSession(path)
			}
		}
		if err == nil {
			fi, err = os.Stat(path)
		}
		if err != nil {
			log.Printf("session file %s: %v (not listed)", path, err)
			continue
		}
		if other, taken := byID[info.ID]; taken && other != name {
			log.Printf("session file %s: number %d belongs to %s already (not listed)", path, info.ID, other)
			continue
		}
		if err := s.list(info, name, fi.Size(), stamp(fi.ModTime())); err != nil {
			return err
		}
		delete(s.ignored, name)
		byID[info.ID] = name
		s.noteID(info.ID)
	}
	for name, e := range listed {
		if !present[name] && s.open[e.id] == nil {
			if _, err := s.index.Exec(`DELETE FROM sessions WHERE file = ?`, name); err != nil {
				return err
			}
			clear(s.ignored) // a file left out for its number may have it free now
		}
	}
	return nil
}

// list writes a session's entry in deelook.db.
func (s *Store) list(info SessionInfo, file string, size int64, modified string) error {
	c, _ := json.Marshal(info.Config)
	n, _ := json.Marshal(info.Samples)
	var ended any
	if info.EndedAt != nil {
		ended = stamp(*info.EndedAt)
	}
	if _, err := s.index.Exec(`DELETE FROM sessions WHERE file = ? AND id <> ?`, file, info.ID); err != nil {
		return err
	}
	_, err := s.index.Exec(`INSERT INTO sessions (id, file, machine_id, machine_name, started_at, ended_at, status, error,
		config, samples, events, size, modified) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET file = excluded.file, machine_id = excluded.machine_id,
		machine_name = excluded.machine_name, started_at = excluded.started_at, ended_at = excluded.ended_at,
		status = excluded.status, error = excluded.error, config = excluded.config, samples = excluded.samples,
		events = excluded.events, size = excluded.size, modified = excluded.modified`,
		info.ID, file, info.Machine, info.MachineName, stamp(info.StartedAt), ended, info.Status, info.Error,
		string(c), string(n), info.Events, size, modified)
	return err
}

// readSession reads a closed session file's row.
func readSession(path string) (SessionInfo, error) {
	db, err := sql.Open("sqlite", dsn(path, false))
	if err != nil {
		return SessionInfo{}, err
	}
	defer db.Close()
	return scanSession(db.QueryRow(`SELECT id, machine_id, machine_name, started_at, ended_at, status, error,
		config, samples, (SELECT COUNT(*) FROM events) FROM session`))
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

// dsn is the sqlite address of a database file, for writing (write-ahead
// log) or reading.
func dsn(path string, write bool) string {
	p := filepath.ToSlash(path)
	p = strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23").Replace(p)
	if write {
		return "file:" + p + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	}
	return "file:" + p + "?mode=ro&_pragma=busy_timeout(5000)"
}

// Close closes the files still being written (deelook stops sampling
// before, so normally none) and deelook.db.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, db := range s.open {
		db.Close()
		delete(s.open, id)
	}
	return s.index.Close()
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
	_ = s.index.QueryRow(`SELECT COUNT(*), COALESCE(SUM(size), 0) FROM sessions`).Scan(&st.Sessions, &st.Used)
	for id := range s.open { // listed with the size they started with
		if path, err := s.path(id); err == nil {
			st.Used += fileSize(path)
		}
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

// stamp is how deelook keeps a moment in its databases: UTC, RFC 3339 to
// 100 ns ("2026-10-14T08:06:01.6065281Z"). Files of earlier builds hold
// local time with its offset, which reads the same. Only file names use
// local time, for people.
func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

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

// CreateSession starts a session's file, lists it, and keeps it open for
// writing.
func (s *Store) CreateSession(machine, name string, cfg Config, started time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.next
	file := fmt.Sprintf("%s_%s_s%d.db", started.Local().Format("20060102-150405"), safeName(name), id)
	path := filepath.Join(s.dir, file)
	db, err := sql.Open("sqlite", dsn(path, true))
	if err == nil {
		db.SetMaxOpenConns(1) // one writer; readers open their own (withDB)
		_, err = db.Exec(sessionSchema)
	}
	if err == nil {
		c, _ := json.Marshal(cfg)
		_, err = db.Exec(`INSERT INTO session (id, machine_id, machine_name, started_at, status, config) VALUES (?, ?, ?, ?, 'running', ?)`,
			id, machine, name, stamp(started), string(c))
	}
	if err == nil {
		err = s.list(SessionInfo{ID: id, Machine: machine, MachineName: name, StartedAt: started, Status: "running", Config: cfg},
			file, 0, "")
	}
	if err != nil {
		if db != nil {
			db.Close()
		}
		os.Remove(path)
		return 0, fmt.Errorf("session file %s: %w", path, err)
	}
	s.open[id] = db
	s.noteID(id)
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
		if _, err := st.Exec(c.Channel, c.First, len(c.Data), stamp(c.ReceivedAt), blob); err != nil {
			return err
		}
	}
	for _, e := range events {
		if _, err := tx.Exec(`INSERT INTO events (time_us, bits, received_at) VALUES (?, ?, ?)`,
			e.TimeUs, e.Bits, stamp(e.At)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// FinishSession records how a session ended, closes its file, folding the
// write-ahead log in so the file stands alone, and lists it as it is now.
func (s *Store) FinishSession(id int64, status, errText string, ended time.Time, samples []int64) error {
	db, err := s.writer(id)
	if err != nil {
		return err
	}
	n, _ := json.Marshal(samples)
	_, err = db.Exec(`UPDATE session SET status = ?, error = ?, ended_at = ?, samples = ?`,
		status, errText, stamp(ended), string(n))
	// back to a plain journal: the file then stands alone. With a reader
	// still open this waits for it (busy_timeout) or fails; SQLite folds
	// the log in anyway when the last connection closes.
	_, _ = db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
	_, _ = db.Exec(`PRAGMA journal_mode = DELETE`)
	cerr := db.Close()
	if err == nil {
		err = cerr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.open, id)
	path, perr := s.path(id)
	if perr == nil {
		info, rerr := readSession(path)
		fi, serr := os.Stat(path)
		if rerr == nil && serr == nil {
			perr = s.list(info, filepath.Base(path), fi.Size(), stamp(fi.ModTime()))
		}
	}
	if err == nil {
		err = perr
	}
	return err
}

// path is a listed session's file; s.mu is held.
func (s *Store) path(id int64) (string, error) {
	var file string
	if err := s.index.QueryRow(`SELECT file FROM sessions WHERE id = ?`, id).Scan(&file); err != nil {
		return "", fmt.Errorf("no session %d", id)
	}
	return filepath.Join(s.dir, file), nil
}

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

const listColumns = `id, machine_id, machine_name, started_at, ended_at, status, error, config, samples, events, file, size`

// scanListed reads a session's entry; s.mu is held.
func (s *Store) scanListed(row interface{ Scan(...any) error }) (SessionInfo, error) {
	var si SessionInfo
	var started, cfg, samples string
	var ended sql.NullString
	if err := row.Scan(&si.ID, &si.Machine, &si.MachineName, &started, &ended, &si.Status, &si.Error, &cfg,
		&samples, &si.Events, &si.File, &si.Size); err != nil {
		return si, err
	}
	si.StartedAt, _ = time.Parse(time.RFC3339Nano, started)
	if ended.Valid {
		t, _ := time.Parse(time.RFC3339Nano, ended.String)
		si.EndedAt = &t
	}
	_ = json.Unmarshal([]byte(cfg), &si.Config)
	_ = json.Unmarshal([]byte(samples), &si.Samples)
	if db := s.open[si.ID]; db != nil { // listed as it started: size and marks as they are now
		si.Size = fileSize(filepath.Join(s.dir, si.File))
		_ = db.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&si.Events)
	}
	return si, nil
}

// Sessions lists the newest sessions first, of one machine or ("") all.
func (s *Store) Sessions(machine string, limit int) ([]SessionInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reconcile(); err != nil {
		return nil, err
	}
	rows, err := s.index.Query(`SELECT `+listColumns+` FROM sessions WHERE ? = '' OR machine_id = ?
		ORDER BY id DESC LIMIT ?`, machine, machine, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionInfo{}
	for rows.Next() {
		si, err := s.scanListed(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, si)
	}
	return out, rows.Err()
}

func (s *Store) Session(id int64) (SessionInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	si, err := s.scanListed(s.index.QueryRow(`SELECT `+listColumns+` FROM sessions WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return si, fmt.Errorf("no session %d", id)
	}
	return si, err
}

// File is a finished session's file, for download.
func (s *Store) File(id int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.open[id] != nil {
		return "", errors.New("the session is still sampling")
	}
	return s.path(id)
}

// Delete removes a finished session's file and its entry.
func (s *Store) Delete(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.open[id] != nil {
		return errors.New("the session is still sampling")
	}
	path, err := s.path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, p := range []string{path + "-wal", path + "-shm", path + "-journal"} {
		os.Remove(p)
	}
	_, err = s.index.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	clear(s.ignored) // a file left out for this number may be listed now
	return err
}

// withDB runs fn on a session's file opened for reading: a connection of
// its own even while the session is being written (the write-ahead log lets
// both go on), so a long CSV export never holds up the writer.
func (s *Store) withDB(id int64, fn func(db *sql.DB) error) error {
	s.mu.Lock()
	path, err := s.path(id)
	s.mu.Unlock()
	if err != nil {
		return err
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
	var n sql.NullInt64
	err := s.index.QueryRow(`SELECT MAX(CAST(SUBSTR(machine_id, 2) AS INTEGER)) FROM sessions WHERE machine_id GLOB 'L[0-9]*'`).Scan(&n)
	return int(n.Int64), err
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
// own (keeping their numbers where free), takes its machine list, and
// removes it once every session's samples and marks are found in the new
// file. The new files are listed by the reconcile after it.
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
	if _, err := s.path(id); err == nil {
		id = s.next // the number is a listed session's already
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
	s.noteID(id)
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
