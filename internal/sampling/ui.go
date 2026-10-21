package sampling

import (
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

// The sampling page keeps what it was set to in deelook.db: the machine
// list and machine picked, and per machine the whole setup (mode, period,
// length, bookmarks, channels). So the page comes back as it was after
// deelook restarts, after going to another page, and in another browser.
// deelook does not read these values; it stores them for the page.

// uiKey is what a page may store under: short names of letters, digits
// and . _ -, such as sampling.form.L1.
var uiKey = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)

// maxUIValue bounds one stored value (a setup of 8 channels is about 2 KB).
const maxUIValue = 64 << 10

// UI returns the value stored under key, or nil when there is none.
func (s *Store) UI(key string) (json.RawMessage, error) {
	if !uiKey.MatchString(key) {
		return nil, errors.New("bad key")
	}
	var v string
	err := s.index.QueryRow(`SELECT value FROM ui WHERE key = ?`, key).Scan(&v)
	if err != nil {
		return nil, nil // none yet (or unreadable: the page starts from defaults)
	}
	return json.RawMessage(v), nil
}

// SetUI stores value (JSON) under key; JSON null removes it.
func (s *Store) SetUI(key string, value json.RawMessage) error {
	if !uiKey.MatchString(key) {
		return errors.New("bad key: letters, digits and . _ - only")
	}
	if len(value) > maxUIValue || !json.Valid(value) {
		return errors.New("the value must be JSON of at most 64 KB")
	}
	if string(value) == "null" {
		_, err := s.index.Exec(`DELETE FROM ui WHERE key = ?`, key)
		return err
	}
	_, err := s.index.Exec(`INSERT INTO ui (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, string(value), stamp(time.Now()))
	return err
}
