package sampling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"deelook/internal/focas"
	"deelook/internal/hub"
)

// Machines that can sample come from deemesh-hub's machine list (the
// nc_focas2_fanuc ones), kept so sampling still works while the hub cannot
// be reached, and from machines added in deelook itself (ids "L1", "L2"...),
// for a deelook without a hub. They live in a JSON file next to the
// settings, not in the sampling database: that database is meant to be
// handed to whoever needs the data, and CNC addresses must not go with it.

// machine is one machine that can sample. Host and port never go to
// browsers.
type machine struct {
	ID       string   `json:"id"`
	Source   string   `json:"source"` // hub | local
	Name     string   `json:"name"`
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Axes     []string `json:"axes,omitempty"` // servo axis names, logical order (all paths)
	Spindles []string `json:"spindles,omitempty"`
	Paths    int      `json:"paths,omitempty"` // 0: not known
}

func (mc machine) target() (target, error) {
	if mc.Host == "" || mc.Port <= 0 || mc.Port > 65535 {
		return target{}, fmt.Errorf("machine %s has no host/port", mc.ID)
	}
	return target{ID: mc.ID, Name: mc.Name, Host: mc.Host, Port: uint16(mc.Port)}, nil
}

// Machines is the machine list file.
type Machines struct {
	path string
	mu   sync.Mutex
	data machineData
}

type machineData struct {
	Note        string    `json:"note"`
	HubSyncedAt time.Time `json:"hubSyncedAt"` // when the hub's list was last received
	Machines    []machine `json:"machines"`
}

const machinesNote = "Machines deelook can sample: the deemesh-hub's Fanuc ones as last received, and those added in deelook. Written by deelook."

// OpenMachines reads the machine list file at path (none is an empty
// list). A list still in an older sampling database is moved here, and
// the database compacted so it holds no machine addresses any more.
func OpenMachines(path string, store *Store) (*Machines, error) {
	m := &Machines{path: path}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(raw, &m.data); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	old, at, err := store.takeOldMachines()
	if err != nil {
		return nil, err
	}
	if len(old) > 0 && len(m.data.Machines) == 0 {
		m.data.Machines, m.data.HubSyncedAt = old, at
		if err := m.save(); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// save writes the file whole, through a temporary file so a crash never
// leaves half of it; m.mu is held (or m not shared yet).
func (m *Machines) save() error {
	m.data.Note = machinesNote
	raw, err := json.MarshalIndent(m.data, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}

// list returns the machines, the hub's by number, then those added here.
func (m *Machines) list() []machine {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := slices.Clone(m.data.Machines)
	slices.SortFunc(out, func(a, b machine) int {
		if a.Source != b.Source {
			return strings.Compare(a.Source, b.Source) // hub before local
		}
		ai, _ := strconv.Atoi(strings.TrimPrefix(a.ID, "L"))
		bi, _ := strconv.Atoi(strings.TrimPrefix(b.ID, "L"))
		return ai - bi
	})
	return out
}

// hubSyncedAt is when the hub's list was last received (zero: never).
func (m *Machines) hubSyncedAt() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.data.HubSyncedAt
}

// syncHub makes the hub entries the Fanuc machines of the hub's latest
// list, keeping the axis names already known for them. The file is written
// when the list changed, else at most every ten minutes (for the time).
func (m *Machines) syncHub(ms []hub.Machine, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	known := map[string]machine{}
	var next []machine
	for _, mc := range m.data.Machines {
		if mc.Source == "hub" {
			known[mc.ID] = mc
		} else {
			next = append(next, mc)
		}
	}
	for _, h := range ms {
		if h.Protocol != fanucProtocol {
			continue
		}
		mc := known[strconv.Itoa(h.ID)]
		mc.ID, mc.Source, mc.Name, mc.Host, mc.Port = strconv.Itoa(h.ID), "hub", h.Name, h.Host, h.Port
		next = append(next, mc)
	}
	changed := !slices.EqualFunc(sorted(next), sorted(m.data.Machines), sameMachine)
	if !changed && now.Sub(m.data.HubSyncedAt) < 10*time.Minute {
		return nil
	}
	m.data.Machines, m.data.HubSyncedAt = next, now
	return m.save()
}

func sorted(ms []machine) []machine {
	out := slices.Clone(ms)
	slices.SortFunc(out, func(a, b machine) int { return strings.Compare(a.ID, b.ID) })
	return out
}

func sameMachine(a, b machine) bool {
	return a.ID == b.ID && a.Source == b.Source && a.Name == b.Name && a.Host == b.Host && a.Port == b.Port &&
		slices.Equal(a.Axes, b.Axes) && slices.Equal(a.Spindles, b.Spindles) && a.Paths == b.Paths
}

func (m *Machines) setNames(id string, axes, spindles []string, paths int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, mc := range m.data.Machines {
		if mc.ID == id {
			if slices.Equal(mc.Axes, axes) && slices.Equal(mc.Spindles, spindles) && mc.Paths == paths {
				return nil
			}
			m.data.Machines[i].Axes, m.data.Machines[i].Spindles, m.data.Machines[i].Paths = axes, spindles, paths
			return m.save()
		}
	}
	return nil
}

// addLocal adds a machine with a new id L<n>, n above every local id in
// the file and above taken (the highest one recorded sessions keep, so a
// removed machine's sessions never pass for a new one's).
func (m *Machines) addLocal(name, host string, port int, axes, spindles []string, paths, taken int) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := taken
	for _, mc := range m.data.Machines {
		if k, err := strconv.Atoi(strings.TrimPrefix(mc.ID, "L")); err == nil && mc.Source == "local" {
			n = max(n, k)
		}
	}
	id := "L" + strconv.Itoa(n+1)
	m.data.Machines = append(m.data.Machines, machine{ID: id, Source: "local", Name: name, Host: host, Port: port,
		Axes: axes, Spindles: spindles, Paths: paths})
	return id, m.save()
}

func (m *Machines) deleteLocal(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := slices.IndexFunc(m.data.Machines, func(mc machine) bool { return mc.ID == id && mc.Source == "local" })
	if i < 0 {
		return errors.New("no such machine added in deelook")
	}
	m.data.Machines = slices.Delete(m.data.Machines, i, i+1)
	return m.save()
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
		// a file deelook cannot write only loses the copy for later
		if serr := m.list.syncHub(ms, time.Now()); serr != nil {
			log.Printf("machine list %s: %v", m.list.path, serr)
		}
	}
	return m.list.list(), err
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
	taken, err := m.store.maxLocalSessionID()
	if err != nil {
		return "", err
	}
	return m.list.addLocal(name, host, port, axes, spindles, paths, taken)
}

// DeleteMachine removes a machine added in deelook (not one of the hub's).
func (m *Manager) DeleteMachine(id string) error {
	m.mu.Lock()
	r := m.runs[id]
	m.mu.Unlock()
	if r != nil && r.active() {
		return errors.New("the machine is sampling")
	}
	return m.list.deleteLocal(id)
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
	return m.list.setNames(id, axes, spindles, paths)
}
