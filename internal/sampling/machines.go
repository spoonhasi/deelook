package sampling

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"deelook/internal/focas"
	"deelook/internal/hub"
)

// Machines that can sample come from deemesh-hub's machine list (the
// nc_focas2_fanuc ones), kept in the database so sampling still works while
// the hub cannot be reached, and from machines added in deelook itself
// (ids "L1", "L2"...), for a deelook without a hub.

const machineSchema = `
CREATE TABLE IF NOT EXISTS machines (
	id         TEXT PRIMARY KEY,        -- hub machine_id, or L1, L2... for machines added in deelook
	source     TEXT NOT NULL,           -- hub | local
	name       TEXT NOT NULL,
	host       TEXT NOT NULL,
	port       INTEGER NOT NULL,
	axes       TEXT NOT NULL DEFAULT '[]', -- servo axis names, logical order (all paths)
	spindles   TEXT NOT NULL DEFAULT '[]',
	paths      INTEGER NOT NULL DEFAULT 0, -- 0: not known
	updated_at TEXT NOT NULL            -- hub rows: last time the hub listed it
);
`

// machine is one machine that can sample. Host and port never go to
// browsers.
type machine struct {
	ID, Source, Name, Host string
	Port                   int
	Axes, Spindles         []string
	Paths                  int
	UpdatedAt              time.Time
}

func (mc machine) target() (target, error) {
	if mc.Host == "" || mc.Port <= 0 || mc.Port > 65535 {
		return target{}, fmt.Errorf("machine %s has no host/port", mc.ID)
	}
	return target{ID: mc.ID, Name: mc.Name, Host: mc.Host, Port: uint16(mc.Port)}, nil
}

// syncHub makes the hub rows the Fanuc machines of the hub's latest list,
// keeping the axis names already known for them.
func (s *Store) syncHub(ms []hub.Machine, now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	keep := []any{}
	for _, mc := range ms {
		if mc.Protocol != fanucProtocol {
			continue
		}
		id := strconv.Itoa(mc.ID)
		keep = append(keep, id)
		if _, err := tx.Exec(`INSERT INTO machines (id, source, name, host, port, updated_at) VALUES (?, 'hub', ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET source = 'hub', name = excluded.name, host = excluded.host, port = excluded.port,
			updated_at = excluded.updated_at`, id, mc.Name, mc.Host, mc.Port, now.Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	q := `DELETE FROM machines WHERE source = 'hub'`
	if len(keep) > 0 {
		q += ` AND id NOT IN (?` + strings.Repeat(`, ?`, len(keep)-1) + `)`
	}
	if _, err := tx.Exec(q, keep...); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) setNames(id string, axes, spindles []string, paths int) error {
	a, _ := json.Marshal(axes)
	sp, _ := json.Marshal(spindles)
	_, err := s.db.Exec(`UPDATE machines SET axes = ?, spindles = ?, paths = ? WHERE id = ?`, string(a), string(sp), paths, id)
	return err
}

// machines lists hub machines by number, then those added in deelook.
func (s *Store) machines() ([]machine, error) {
	rows, err := s.db.Query(`SELECT id, source, name, host, port, axes, spindles, paths, updated_at FROM machines`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []machine
	for rows.Next() {
		var mc machine
		var axes, spindles, at string
		if err := rows.Scan(&mc.ID, &mc.Source, &mc.Name, &mc.Host, &mc.Port, &axes, &spindles, &mc.Paths, &at); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(axes), &mc.Axes)
		_ = json.Unmarshal([]byte(spindles), &mc.Spindles)
		mc.UpdatedAt, _ = time.Parse(time.RFC3339Nano, at)
		out = append(out, mc)
	}
	slices.SortFunc(out, func(a, b machine) int {
		if a.Source != b.Source {
			return strings.Compare(a.Source, b.Source) // hub before local
		}
		ai, _ := strconv.Atoi(strings.TrimPrefix(a.ID, "L"))
		bi, _ := strconv.Atoi(strings.TrimPrefix(b.ID, "L"))
		return ai - bi
	})
	return out, rows.Err()
}

func (s *Store) addLocal(name, host string, port int, axes, spindles []string, paths int) (string, error) {
	// a new id, also unlike the ids of removed machines that sessions keep
	var n sql.NullInt64
	if err := s.db.QueryRow(`SELECT MAX(n) FROM (
		SELECT CAST(SUBSTR(id, 2) AS INTEGER) AS n FROM machines WHERE source = 'local'
		UNION ALL SELECT CAST(SUBSTR(machine_id, 2) AS INTEGER) FROM sessions WHERE machine_id GLOB 'L[0-9]*')`).Scan(&n); err != nil {
		return "", err
	}
	id := "L" + strconv.FormatInt(n.Int64+1, 10)
	a, _ := json.Marshal(axes)
	sp, _ := json.Marshal(spindles)
	_, err := s.db.Exec(`INSERT INTO machines (id, source, name, host, port, axes, spindles, paths, updated_at) VALUES (?, 'local', ?, ?, ?, ?, ?, ?, ?)`,
		id, name, host, port, string(a), string(sp), paths, time.Now().Format(time.RFC3339Nano))
	return id, err
}

func (s *Store) deleteLocal(id string) error {
	r, err := s.db.Exec(`DELETE FROM machines WHERE id = ? AND source = 'local'`, id)
	if err != nil {
		return err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return errors.New("no such machine added in deelook")
	}
	return nil
}

// hubWait bounds asking the hub for its machine list, so an unreachable
// hub delays sampling only briefly before the stored list is used.
const hubWait = 3 * time.Second

// machines returns every machine that can sample: the hub's (refreshed
// from the hub when it answers, else as last stored) and those added in
// deelook. hubErr says why the hub's list could not be refreshed.
func (m *Manager) machines(ctx context.Context) (list []machine, hubErr error) {
	hctx, cancel := context.WithTimeout(ctx, hubWait)
	ms, err := m.hub.Machines(hctx)
	cancel()
	if err == nil {
		err = m.store.syncHub(ms, time.Now())
	}
	list, lerr := m.store.machines()
	if lerr != nil {
		return nil, lerr
	}
	return list, err
}

func (m *Manager) target(ctx context.Context, id string) (target, error) {
	list, hubErr := m.machines(ctx)
	for _, mc := range list {
		if mc.ID == id {
			return mc.target()
		}
	}
	if hubErr != nil {
		return target{}, fmt.Errorf("machine %s is not known (and the hub cannot be reached: %v)", id, hubErr)
	}
	return target{}, fmt.Errorf("machine %s is not a Fanuc (nc_focas2_fanuc) machine on the hub nor one added in deelook", id)
}

// onHost runs fn on its own locked OS thread (a FOCAS2 handle belongs to
// its thread), while nothing else in deelook talks to the host's sampler.
func (m *Manager) onHost(id, host string, fn func()) error {
	m.mu.Lock()
	if err := m.free(id, host); err != nil {
		m.mu.Unlock()
		return err
	}
	m.busy[host] = true
	m.mu.Unlock()
	done := make(chan struct{})
	go func() {
		// the host stays busy until fn is through, even if the caller
		// gave up waiting
		defer func() {
			m.mu.Lock()
			delete(m.busy, host)
			m.mu.Unlock()
			close(done)
		}()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		fn()
	}()
	select {
	case <-done:
		return nil
	case <-time.After(30 * time.Second):
		return errors.New("the CNC did not answer in 30 s")
	}
}

// readNames reads a machine's axis and spindle names over FOCAS2.
func (m *Manager) readNames(t target) (axes, spindles []string, paths int, err error) {
	if err := m.needLib(); err != nil {
		return nil, nil, 0, err
	}
	ferr := m.onHost(t.ID, t.Host, func() {
		var h *focas.Handle
		h, err = m.lib.Connect(t.Host, t.Port, 10*time.Second)
		if err != nil {
			return
		}
		defer h.Free()
		axes, spindles, paths, err = h.Names()
	})
	if ferr != nil {
		return nil, nil, 0, ferr
	}
	return axes, spindles, paths, err
}

// AddMachine adds a Fanuc machine by address, after reading its axis names
// over FOCAS2 (which also proves the address answers).
func (m *Manager) AddMachine(name, host string, port int) (string, error) {
	name, host = strings.TrimSpace(name), strings.TrimSpace(host)
	if host == "" || port <= 0 || port > 65535 {
		return "", errors.New("give the CNC's IP address (or host name) and FOCAS2 port")
	}
	if name == "" {
		name = host
	}
	axes, spindles, paths, err := m.readNames(target{ID: "new", Name: name, Host: host, Port: uint16(port)})
	if err != nil {
		return "", fmt.Errorf("the address does not answer as a FOCAS2 CNC: %w", err)
	}
	return m.store.addLocal(name, host, port, axes, spindles, paths)
}

// DeleteMachine removes a machine added in deelook (not one of the hub's).
func (m *Manager) DeleteMachine(id string) error {
	m.mu.Lock()
	r := m.runs[id]
	m.mu.Unlock()
	if r != nil && r.active() {
		return errors.New("the machine is sampling")
	}
	return m.store.deleteLocal(id)
}

// RefreshNames reads a machine's axis names over FOCAS2 and stores them.
func (m *Manager) RefreshNames(ctx context.Context, id string) error {
	t, err := m.target(ctx, id)
	if err != nil {
		return err
	}
	axes, spindles, paths, err := m.readNames(t)
	if err != nil {
		return err
	}
	return m.store.setNames(id, axes, spindles, paths)
}
