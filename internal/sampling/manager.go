package sampling

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"deelook/internal/focas"
	"deelook/internal/hub"
)

// Manager runs at most one sampling session per machine: any Fanuc machine
// on the hub's list, or added in deelook (machines.go).
type Manager struct {
	hub     *hub.Client
	store   *Store
	list    *Machines  // the machines that can sample (machines.go)
	lib     *focas.Lib // nil: no FOCAS2 library, no sampling
	libPath string     // where the library was looked for
	libErr  error      // why there is none

	mu   sync.Mutex
	runs map[string]*run // latest run per machine, finished ones included
	busy map[string]bool // hosts a read1shot is talking to
}

// NewManager takes the FOCAS2 library already loaded: deelook does not run
// without one that has cnc_sdt*.
// fanucProtocol is the hub protocol of the machines that can sample.
const fanucProtocol = "nc_focas2_fanuc"

// lib is nil when deelook found no usable FOCAS2 library (libErr says why,
// libPath is where it looked): then the machine list, recorded sessions and
// the reference still work, and every call that needs the CNC says so.
func NewManager(c *hub.Client, store *Store, list *Machines, lib *focas.Lib, libPath string, libErr error) *Manager {
	return &Manager{hub: c, store: store, list: list, lib: lib, libPath: libPath, libErr: libErr,
		runs: map[string]*run{}, busy: map[string]bool{}}
}

// needLib is the error of a call that needs the FOCAS2 library when there
// is none.
func (m *Manager) needLib() error {
	if m.lib == nil {
		return fmt.Errorf("no FOCAS2 library, so no sampling: %v", m.libErr)
	}
	return nil
}

// Start starts a session and returns once sampling runs on the CNC (or
// failed to).
func (m *Manager) Start(ctx context.Context, id string, cfg Config) (Status, error) {
	if err := cfg.Validate(); err != nil {
		return Status{}, err
	}
	if err := m.needLib(); err != nil {
		return Status{}, err
	}
	if cfg.extended() && !m.lib.Extended() {
		return Status{}, fmt.Errorf("%s has no cnc_sdt*2 calls (extended sampling)", m.lib.Path)
	}
	t, err := m.target(ctx, id)
	if err != nil {
		return Status{}, err
	}

	m.mu.Lock()
	if err := m.free(id, t.Host); err != nil {
		m.mu.Unlock()
		return Status{}, err
	}
	r := newRun(m.lib, m.store, t, cfg)
	m.runs[id] = r
	m.mu.Unlock()

	go r.loop()
	select {
	case err := <-r.ready:
		if err != nil {
			return r.status(), err
		}
	case <-time.After(30 * time.Second):
		return r.status(), errors.New("the CNC did not answer in 30 s")
	}
	return r.status(), nil
}

// free reports whether nothing else talks to the machine's sampler; m.mu
// is held.
func (m *Manager) free(id, host string) error {
	if m.busy[host] {
		return errors.New("another FOCAS2 call (read1shot, axis names) is talking to this CNC")
	}
	for mid, r := range m.runs {
		if !r.active() {
			continue
		}
		if mid == id {
			return errors.New("sampling is already running on this machine")
		}
		// some FOCAS2 builds share one connection per IP inside a process,
		// whatever the port: two machines behind one address would mix
		if r.m.Host == host {
			return fmt.Errorf("machine %s, at the same address, is already sampling", mid)
		}
	}
	return nil
}

// Stop ends the machine's session.
func (m *Manager) Stop(id string) (Status, error) {
	m.mu.Lock()
	r := m.runs[id]
	m.mu.Unlock()
	if r == nil || !r.active() {
		return Status{}, errors.New("no sampling is running on this machine")
	}
	err := r.requestStop(15 * time.Second)
	return r.status(), err
}

// Close stops every session (at shutdown), so no sampling is left running
// on a CNC.
func (m *Manager) Close() {
	m.mu.Lock()
	runs := make([]*run, 0, len(m.runs))
	for _, r := range m.runs {
		runs = append(runs, r)
	}
	m.mu.Unlock()
	for _, r := range runs {
		if r.active() {
			_ = r.requestStop(10 * time.Second)
		}
	}
}

// Status is a machine's sampling state for the browser.
type Status struct {
	Machine     string     `json:"machine"`
	Name        string     `json:"name,omitempty"`
	State       string     `json:"state"` // idle, starting, waiting, running, stopping, stopped, completed, failed
	Error       string     `json:"error,omitempty"`
	Session     int64      `json:"session,omitempty"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	FirstDataAt *time.Time `json:"firstDataAt,omitempty"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
	Config      *Config    `json:"config,omitempty"`
	Samples     []int64    `json:"samples,omitempty"`
	Reads       *ReadStats `json:"reads,omitempty"`
	Events      []Event    `json:"events,omitempty"`
	Calls       []Call     `json:"calls,omitempty"`
}

func (r *run) active() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch r.state {
	case "starting", "waiting", "running", "stopping":
		return true
	}
	return false
}

func (r *run) status() Status {
	r.mu.Lock()
	defer r.mu.Unlock()
	reads := r.reads
	reads.LastPoints = append([]int32(nil), r.reads.LastPoints...)
	s := Status{Machine: r.m.ID, Name: r.m.Name, State: r.state, Error: r.err, Session: r.session,
		Config: &r.cfg, Samples: append([]int64(nil), r.samples...), Reads: &reads,
		Events: append([]Event(nil), r.events...), Calls: append([]Call(nil), r.calls...)}
	for _, t := range []struct {
		v   time.Time
		dst **time.Time
	}{{r.started, &s.StartedAt}, {r.firstData, &s.FirstDataAt}, {r.ended, &s.EndedAt}} {
		if !t.v.IsZero() {
			v := t.v
			*t.dst = &v
		}
	}
	return s
}

// Status of every machine that sampled since deelook started; the others
// are idle.
func (m *Manager) Status() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Status, 0, len(m.runs))
	for _, r := range m.runs {
		out = append(out, r.status())
	}
	slices.SortFunc(out, func(a, b Status) int { return strings.Compare(a.Machine, b.Machine) })
	return out
}

// LiveChannel is the newest stretch of one channel, reduced to buckets so
// peaks survive the reduction: min and max of the values, or for a PMC bit
// channel the AND and OR of the words (a bit set in OR but not in AND
// changed within the bucket).
type LiveChannel struct {
	Label    string   `json:"label"`
	Kind     string   `json:"kind"`
	Signals  []string `json:"signals,omitempty"`
	PeriodUs int      `json:"periodUs"`
	BucketUs int      `json:"bucketUs"` // time one bucket covers
	EndUs    int64    `json:"endUs"`    // time after the last sample, from the first
	Min      []int32  `json:"min"`
	Max      []int32  `json:"max"`
}

// Live is what the live view draws.
type Live struct {
	Channels []LiveChannel `json:"channels"`
	Events   []Event       `json:"events"` // marks within the window
}

// Live returns the last `window` of each channel of the machine's current
// or last session, at most `buckets` buckets per channel.
func (m *Manager) Live(id string, window time.Duration, buckets int) Live {
	out := Live{Channels: []LiveChannel{}, Events: []Event{}}
	m.mu.Lock()
	r := m.runs[id]
	m.mu.Unlock()
	if r == nil {
		return out
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	end := int64(0)
	for i, ch := range r.cfg.Channels {
		p := r.cfg.PeriodUs(i)
		data := r.rings[i].last(int(window/time.Microsecond) / p)
		per := max(1, (len(data)+buckets-1)/buckets)
		lc := LiveChannel{Label: ch.Label, Kind: ch.Kind, Signals: ch.Signals, PeriodUs: p, BucketUs: per * p,
			EndUs: r.samples[i] * int64(p), Min: []int32{}, Max: []int32{}}
		end = max(end, lc.EndUs)
		value := func(w uint16) int32 {
			if r.cfg.Signed(i) {
				return int32(int16(w))
			}
			return int32(w)
		}
		for j := 0; j < len(data); j += per {
			part := data[j:min(j+per, len(data))]
			if ch.Kind == "pmcbit" {
				and, or := part[0], part[0]
				for _, w := range part {
					and, or = and&w, or|w
				}
				lc.Min, lc.Max = append(lc.Min, int32(and)), append(lc.Max, int32(or))
				continue
			}
			lo, hi := value(part[0]), value(part[0])
			for _, w := range part {
				lo, hi = min(lo, value(w)), max(hi, value(w))
			}
			lc.Min, lc.Max = append(lc.Min, lo), append(lc.Max, hi)
		}
		out.Channels = append(out.Channels, lc)
	}
	for _, e := range r.events {
		if e.TimeUs >= end-int64(window/time.Microsecond) {
			out.Events = append(out.Events, e)
		}
	}
	return out
}

// OneShot is a cnc_sdtread1shot request: the present value of up to 8
// servo/spindle data, without sampling.
type OneShot struct {
	ServoPeriodUs int             `json:"servoPeriodUs"`
	Channels      []ChannelConfig `json:"channels"`
}

// OneShotResult holds the values read (signed 16-bit, as servo and spindle
// data are) and the calls made.
type OneShotResult struct {
	At     time.Time `json:"at"`
	Values []int16   `json:"values"`
	Calls  []Call    `json:"calls"`
}

// ReadOnce connects, sets the channels, reads them once and lets go, on a
// machine that is not sampling.
func (m *Manager) ReadOnce(ctx context.Context, id string, req OneShot) (OneShotResult, error) {
	cfg := Config{Mode: "basic", ServoPeriodUs: req.ServoPeriodUs, Channels: req.Channels}
	if cfg.ServoPeriodUs == 0 {
		cfg.ServoPeriodUs = 1000
	}
	if err := cfg.Validate(); err != nil {
		return OneShotResult{}, err
	}
	if err := m.needLib(); err != nil {
		return OneShotResult{}, err
	}
	t, err := m.target(ctx, id)
	if err != nil {
		return OneShotResult{}, err
	}
	// a run is only used for its call log here
	r := &run{}
	var res OneShotResult
	ferr := m.onHost(id, t.Host, func() {
		var h *focas.Handle
		err = r.call("cnc_allclibhndl3", func() (err error) {
			h, err = m.lib.Connect(t.Host, t.Port, 10*time.Second)
			return err
		})
		if err != nil {
			return
		}
		chs := cfg.focasChannels()
		r.cleanup("cnc_sdtclrchnl", h.ClearChannels)
		err = r.call("cnc_sdtsetchnl", func() error { return h.SetChannels(chs, focas.Unlimited) })
		if err == nil {
			var data [8]uint16
			err = r.call("cnc_sdtread1shot", func() (err error) {
				data, err = h.ReadOnce()
				return err
			})
			res.At = time.Now()
			for i := range chs {
				res.Values = append(res.Values, int16(data[i]))
			}
			_ = r.call("cnc_sdtclrchnl", h.ClearChannels)
		}
		_ = r.call("cnc_freelibhndl", func() error { h.Free(); return nil })
	})
	if ferr != nil {
		return OneShotResult{}, ferr
	}
	res.Calls = r.calls
	return res, err
}
