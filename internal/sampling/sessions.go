package sampling

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Recorded sessions keep piling up, so the page asks for them a part at a
// time: a summary per day, the sessions of a day when it is opened, and
// deletes or downloads many at once.

// SessionFilter narrows a listing. Times are RFC 3339; a session counts by
// when it started.
type SessionFilter struct {
	Machine string   // "" for all machines
	Since   string   // started at or after ("": any)
	Until   string   // started before ("": any)
	Status  []string // any of these (none: all)
	Before  int64    // numbers below this, for the next part (0: from the newest)
	Limit   int      // at most this many (0: all)
}

// Check puts the times into the form the index compares (UTC), and refuses
// what is not a time or a status.
func (f *SessionFilter) Check() error {
	for _, t := range []*string{&f.Since, &f.Until} {
		if *t == "" {
			continue
		}
		v, err := time.Parse(time.RFC3339Nano, *t)
		if err != nil {
			return fmt.Errorf("bad time %q: want RFC 3339", *t)
		}
		*t = stamp(v)
	}
	for _, st := range f.Status {
		switch st {
		case "running", "stopped", "completed", "failed", "interrupted":
		default:
			return fmt.Errorf("bad status %q", st)
		}
	}
	return nil
}

func (f SessionFilter) where() (string, []any) {
	conds := []string{"1"}
	var args []any
	if f.Machine != "" {
		conds, args = append(conds, "machine_id = ?"), append(args, f.Machine)
	}
	// julianday reads the stored times whatever their offset or fraction
	if f.Since != "" {
		conds, args = append(conds, "julianday(started_at) >= julianday(?)"), append(args, f.Since)
	}
	if f.Until != "" {
		conds, args = append(conds, "julianday(started_at) < julianday(?)"), append(args, f.Until)
	}
	if len(f.Status) > 0 {
		conds = append(conds, "status IN (?"+strings.Repeat(", ?", len(f.Status)-1)+")")
		for _, st := range f.Status {
			args = append(args, st)
		}
	}
	if f.Before > 0 {
		conds, args = append(conds, "id < ?"), append(args, f.Before)
	}
	return strings.Join(conds, " AND "), args
}

// SessionsWhere lists the sessions f selects, newest first.
func (s *Store) SessionsWhere(f SessionFilter) ([]SessionInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.freshen(); err != nil {
		return nil, err
	}
	where, args := f.where()
	q := `SELECT ` + listColumns + ` FROM sessions WHERE ` + where + ` ORDER BY id DESC`
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
	}
	rows, err := s.index.Query(q, args...)
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

// Day is how many sessions started on a day, and their size.
type Day struct {
	Day   string `json:"day"` // YYYY-MM-DD where the page is
	Count int    `json:"count"`
	Size  int64  `json:"size"`
}

// Days sums the sessions f selects per day, newest first, in the time zone
// tzMinutes east of UTC (the page's).
func (s *Store) Days(f SessionFilter, tzMinutes int) ([]Day, error) {
	if tzMinutes < -14*60 || tzMinutes > 14*60 {
		return nil, errors.New("bad time zone offset")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.freshen(); err != nil {
		return nil, err
	}
	where, args := f.where()
	rows, err := s.index.Query(`SELECT date(started_at, ?) AS day, COUNT(*), COALESCE(SUM(size), 0) FROM sessions
		WHERE `+where+` GROUP BY day ORDER BY day DESC`, append([]any{fmt.Sprintf("%+d minutes", tzMinutes)}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Day{}
	for rows.Next() {
		var d Day
		if err := rows.Scan(&d.Day, &d.Count, &d.Size); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Finished lists the numbers and total size of the finished sessions f
// selects (those still sampling are left out): what a delete would take.
func (s *Store) Finished(f SessionFilter) (ids []int64, size int64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.freshen(); err != nil {
		return nil, 0, err
	}
	where, args := f.where()
	rows, err := s.index.Query(`SELECT id, size FROM sessions WHERE `+where+` AND status != 'running' ORDER BY id`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, 0, err
		}
		if s.open[id] == nil {
			ids, size = append(ids, id), size+n
		}
	}
	return ids, size, rows.Err()
}

// Deleted is what a delete of many sessions did.
type Deleted struct {
	Count  int              `json:"count"`
	Size   int64            `json:"size"`             // bytes freed
	Failed map[int64]string `json:"failed,omitempty"` // numbers it could not delete, and why
}

// DeleteMany deletes these sessions' files, as Delete does one.
func (s *Store) DeleteMany(ids []int64) Deleted {
	out := Deleted{Failed: map[int64]string{}}
	for _, id := range ids {
		si, err := s.Session(id)
		if err == nil {
			err = s.Delete(id)
		}
		if err != nil {
			out.Failed[id] = err.Error()
			continue
		}
		out.Count++
		out.Size += si.Size
	}
	return out
}
