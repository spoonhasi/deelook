package sampling

import (
	"bufio"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure Go: keeps the build free of cgo
)

// Store keeps sessions and their samples in SQLite. Samples go in chunks:
// one row per channel per read from the CNC, the raw 16-bit words as a
// little-endian blob. Sample i of a channel was taken i periods after the
// session started (the CNC samples at a fixed period, so this is more exact
// than the PC clock; received_at records when the chunk arrived).
type Store struct {
	db *sql.DB
}

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
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
	session_id  INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
	channel     INTEGER NOT NULL,           -- index into config.channels
	first       INTEGER NOT NULL,           -- index of the chunk's first sample
	count       INTEGER NOT NULL,
	received_at TEXT NOT NULL,
	data        BLOB NOT NULL,              -- count raw 16-bit words, little endian
	PRIMARY KEY (session_id, channel, first)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS events (
	session_id  INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
	time_us     INTEGER NOT NULL,           -- from the first sample
	bits        INTEGER NOT NULL,           -- cnc_sdtreadsmpl2 trigger bits
	received_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS events_session ON events(session_id, time_us);
`

// OpenStore opens (creating if needed) the database at path. Sessions left
// "running" by a process that died are marked "interrupted".
func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(path) +
		"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("sampling database %s: %w", path, err)
	}
	if _, err := db.Exec(`UPDATE sessions SET status = 'interrupted', ended_at = COALESCE(ended_at, started_at)
		WHERE status = 'running'`); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// takeOldMachines moves out the machine list an older deelook kept in this
// database (with CNC addresses): it returns the rows and when the hub's list
// was last received, drops the table and compacts the file so none of it
// stays in free pages or the write-ahead log.
func (s *Store) takeOldMachines() ([]machine, time.Time, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'machines'`).Scan(&n); err != nil || n == 0 {
		return nil, time.Time{}, err
	}
	rows, err := s.db.Query(`SELECT id, source, name, host, port, axes, spindles, paths, updated_at FROM machines`)
	if err != nil {
		return nil, time.Time{}, err
	}
	var out []machine
	var synced time.Time
	for rows.Next() {
		var mc machine
		var axes, spindles, at string
		if err := rows.Scan(&mc.ID, &mc.Source, &mc.Name, &mc.Host, &mc.Port, &axes, &spindles, &mc.Paths, &at); err != nil {
			rows.Close()
			return nil, time.Time{}, err
		}
		_ = json.Unmarshal([]byte(axes), &mc.Axes)
		_ = json.Unmarshal([]byte(spindles), &mc.Spindles)
		if t, _ := time.Parse(time.RFC3339Nano, at); mc.Source == "hub" && t.After(synced) {
			synced = t
		}
		out = append(out, mc)
	}
	rows.Close()
	for _, q := range []string{`DROP TABLE machines`, `VACUUM`, `PRAGMA wal_checkpoint(TRUNCATE)`} {
		if _, err := s.db.Exec(q); err != nil {
			return nil, time.Time{}, fmt.Errorf("moving the machine list out of the database: %s: %w", q, err)
		}
	}
	return out, synced, nil
}

// maxLocalSessionID is the highest n of the machine ids L<n> (machines
// added in deelook) that recorded sessions keep, 0 if none.
func (s *Store) maxLocalSessionID() (int, error) {
	var n sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(CAST(SUBSTR(machine_id, 2) AS INTEGER)) FROM sessions WHERE machine_id GLOB 'L[0-9]*'`).Scan(&n)
	return int(n.Int64), err
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
}

func (s *Store) CreateSession(machine, name string, cfg Config, started time.Time) (int64, error) {
	c, _ := json.Marshal(cfg)
	r, err := s.db.Exec(`INSERT INTO sessions (machine_id, machine_name, started_at, status, config)
		VALUES (?, ?, ?, 'running', ?)`, machine, name, started.Format(time.RFC3339Nano), string(c))
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

// Add writes chunks and trigger marks in one transaction.
func (s *Store) Add(session int64, chunks []Chunk, events []Event) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	st, err := tx.Prepare(`INSERT INTO chunks (session_id, channel, first, count, received_at, data) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer st.Close()
	for _, c := range chunks {
		blob := make([]byte, 2*len(c.Data))
		for i, w := range c.Data {
			binary.LittleEndian.PutUint16(blob[2*i:], w)
		}
		if _, err := st.Exec(session, c.Channel, c.First, len(c.Data), c.ReceivedAt.Format(time.RFC3339Nano), blob); err != nil {
			return err
		}
	}
	for _, e := range events {
		if _, err := tx.Exec(`INSERT INTO events (session_id, time_us, bits, received_at) VALUES (?, ?, ?, ?)`,
			session, e.TimeUs, e.Bits, e.At.Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) FinishSession(id int64, status, errText string, ended time.Time, samples []int64) error {
	n, _ := json.Marshal(samples)
	_, err := s.db.Exec(`UPDATE sessions SET status = ?, error = ?, ended_at = ?, samples = ? WHERE id = ?`,
		status, errText, ended.Format(time.RFC3339Nano), string(n), id)
	return err
}

const sessionColumns = `id, machine_id, machine_name, started_at, ended_at, status, error, config, samples,
	(SELECT COUNT(*) FROM events WHERE session_id = sessions.id)`

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

// Sessions lists the newest sessions first, of one machine or ("") all.
func (s *Store) Sessions(machine string, limit int) ([]SessionInfo, error) {
	rows, err := s.db.Query(`SELECT `+sessionColumns+` FROM sessions WHERE ? = '' OR machine_id = ?
		ORDER BY id DESC LIMIT ?`, machine, machine, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionInfo{}
	for rows.Next() {
		si, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, si)
	}
	return out, rows.Err()
}

func (s *Store) Session(id int64) (SessionInfo, error) {
	return scanSession(s.db.QueryRow(`SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id))
}

// channelReader walks one channel's samples in order across its chunks.
type channelReader struct {
	rows *sql.Rows
	buf  []byte
	next int64 // index of buf's first sample
	pos  int   // samples of buf already used
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
	cfg := si.Config
	n := len(cfg.Channels)
	if n == 0 {
		return errors.New("session has no channels")
	}
	readers := make([]*channelReader, n)
	base := 0
	for i := range n {
		rows, err := s.db.Query(`SELECT first, data FROM chunks WHERE session_id = ? AND channel = ? ORDER BY first`, id, i)
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
	rows, err := s.db.Query(`SELECT time_us, bits FROM events WHERE session_id = ?`, id)
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
