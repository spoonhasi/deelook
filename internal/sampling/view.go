package sampling

import (
	"database/sql"
	"encoding/binary"
	"time"
)

// View is a stretch of a recorded session reduced for drawing, like Live
// but read from the database: the whole session, or a zoomed range.
type View struct {
	FromUs   int64         `json:"fromUs"`
	ToUs     int64         `json:"toUs"`
	TotalUs  int64         `json:"totalUs"` // time the longest channel covers
	Channels []LiveChannel `json:"channels"`
	Events   []Event       `json:"events"`
}

// View reduces session id between fromUs and toUs (0, 0: all of it) to at
// most buckets buckets per channel: min and max, or for PMC bit channels
// AND and OR, as Live does.
func (s *Store) View(id int64, fromUs, toUs int64, buckets int) (View, error) {
	si, err := s.Session(id)
	if err != nil {
		return View{}, err
	}
	var v View
	err = s.withDB(id, func(db *sql.DB) (err error) {
		v, err = view(db, si.Config, fromUs, toUs, buckets)
		return err
	})
	return v, err
}

func view(db *sql.DB, cfg Config, fromUs, toUs int64, buckets int) (View, error) {
	v := View{Channels: []LiveChannel{}, Events: []Event{}}

	// how far each channel got: from the chunks, so running and interrupted
	// sessions work too
	got := make([]int64, len(cfg.Channels))
	rows, err := db.Query(`SELECT channel, MAX(first + count) FROM chunks GROUP BY channel`)
	if err != nil {
		return v, err
	}
	for rows.Next() {
		var ch int
		var n int64
		if rows.Scan(&ch, &n) == nil && ch >= 0 && ch < len(got) {
			got[ch] = n
		}
	}
	rows.Close()
	for i := range cfg.Channels {
		v.TotalUs = max(v.TotalUs, got[i]*int64(cfg.PeriodUs(i)))
	}
	if toUs <= 0 || toUs > v.TotalUs {
		toUs = v.TotalUs
	}
	fromUs = max(0, min(fromUs, toUs-1000))
	v.FromUs, v.ToUs = fromUs, toUs

	for i, ch := range cfg.Channels {
		p := int64(cfg.PeriodUs(i))
		first, last := fromUs/p, min((toUs+p-1)/p, got[i]) // sample range, last exclusive
		n := max(last-first, 0)
		per := max(1, (n+int64(buckets)-1)/int64(buckets))
		nb := (n + per - 1) / per
		lc := LiveChannel{Label: ch.Label, Kind: ch.Kind, Signals: ch.Signals, PeriodUs: int(p),
			BucketUs: int(per * p), EndUs: (first + nb*per) * p, Min: make([]int32, nb), Max: make([]int32, nb)}
		seen := make([]bool, nb)
		signed, bits := cfg.Signed(i), ch.Kind == "pmcbit"
		rows, err := db.Query(`SELECT first, data FROM chunks WHERE channel = ?
			AND first < ? AND first + count > ? ORDER BY first`, i, last, first)
		if err != nil {
			return v, err
		}
		for rows.Next() {
			var at int64
			var blob []byte
			if rows.Scan(&at, &blob) != nil {
				continue
			}
			for k := max(at, first); k < min(at+int64(len(blob)/2), last); k++ {
				w := binary.LittleEndian.Uint16(blob[2*(k-at):])
				val := int32(w)
				if signed {
					val = int32(int16(w))
				}
				b := (k - first) / per
				switch {
				case !seen[b]:
					lc.Min[b], lc.Max[b], seen[b] = val, val, true
				case bits:
					lc.Min[b], lc.Max[b] = lc.Min[b]&val, lc.Max[b]|val
				default:
					lc.Min[b], lc.Max[b] = min(lc.Min[b], val), max(lc.Max[b], val)
				}
			}
		}
		rows.Close()
		// a bucket without data (a gap) repeats the one before
		for b := int64(1); b < nb; b++ {
			if !seen[b] {
				lc.Min[b], lc.Max[b] = lc.Min[b-1], lc.Max[b-1]
			}
		}
		v.Channels = append(v.Channels, lc)
	}

	rows, err = db.Query(`SELECT time_us, bits, received_at FROM events WHERE time_us BETWEEN ? AND ?
		ORDER BY time_us`, fromUs, toUs)
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		var e Event
		var at string
		if rows.Scan(&e.TimeUs, &e.Bits, &at) == nil {
			e.At, _ = time.Parse(time.RFC3339Nano, at)
			v.Events = append(v.Events, e)
		}
	}
	return v, rows.Err()
}
