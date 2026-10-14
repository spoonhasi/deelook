package sampling

import (
	"errors"
	"fmt"
	"log"
	"runtime"
	"slices"
	"sync"
	"time"

	"deelook/internal/focas"
)

const (
	readEvery  = 20 * time.Millisecond  // cnc_sdtreadsmpl(2) interval when a call returns at once
	flushEvery = 500 * time.Millisecond // database write interval
	readPoints = 8192                   // buffer per channel and read (multiple of 8)
	liveWindow = 4 * time.Second        // kept in memory for the live view
	diskEvery  = 5 * time.Second        // free disk space check interval
	maxCalls   = 40                     // FOCAS calls kept for the browser
	maxEvents  = 200                    // trigger marks kept for the browser
)

// target is the machine a run connects to.
type target struct {
	ID, Name, Host string
	Port           uint16
}

// Event is a moment the CNC marked (cnc_sdtreadsmpl2 trigger bits).
type Event struct {
	TimeUs int64     `json:"timeUs"` // from the first sample, at the last point read with the mark
	Bits   uint16    `json:"bits"`   // focas.TrgSeq, TrgUp, TrgDown, TrgEnded as the CNC reported them
	At     time.Time `json:"at"`     // when deelook received it
}

// Call is one FOCAS2 call other than the repeated reads, for the browser
// to show what a session does.
type Call struct {
	At      time.Time `json:"at"`
	Fn      string    `json:"fn"`
	Ms      float64   `json:"ms"`
	Result  string    `json:"result"`            // "EW_OK" or the error
	Cleanup bool      `json:"cleanup,omitempty"` // tidying before setup: refusals are expected
}

// ReadStats describes the repeated cnc_sdtreadsmpl(2) calls: how much one
// call carries and whether reading keeps up with the CNC.
type ReadStats struct {
	Calls      int64   `json:"calls"`
	LastPoints []int32 `json:"lastPoints"` // per channel, in the last call
	MaxPoints  int32   `json:"maxPoints"`  // most points of channel 1 in one call
	AvgMs      float64 `json:"avgMs"`      // mean call time over the last second
	MaxMs      float64 `json:"maxMs"`      // longest call so far
	Rate       float64 `json:"rate"`       // points/s of channel 1 read over the last second
	Expected   float64 `json:"expected"`   // points/s of channel 1 the CNC takes
	Behind     bool    `json:"behind"`     // reading slower than the CNC samples
}

// run is one sampling session on one machine. Its loop owns the FOCAS
// handle and runs on a locked OS thread, since a handle belongs to the
// thread that allocated it.
type run struct {
	lib   *focas.Lib
	store *Store
	m     target
	cfg   Config

	stop  chan struct{} // closed to ask the loop to end
	ready chan error    // the start result, sent once
	done  chan struct{} // closed when the loop has ended

	mu        sync.Mutex
	state     string // starting, waiting, running, stopping, stopped, completed, failed
	err       string
	session   int64
	started   time.Time
	firstData time.Time
	ended     time.Time
	samples   []int64
	rings     []*ring
	events    []Event
	calls     []Call
	reads     ReadStats
}

func newRun(lib *focas.Lib, store *Store, m target, cfg Config) *run {
	r := &run{lib: lib, store: store, m: m, cfg: cfg, state: "starting",
		stop: make(chan struct{}), ready: make(chan error, 1), done: make(chan struct{}),
		samples: make([]int64, len(cfg.Channels))}
	for i := range cfg.Channels {
		r.rings = append(r.rings, newRing(int(liveWindow/time.Microsecond)/cfg.PeriodUs(i)))
	}
	r.reads.LastPoints = make([]int32, len(cfg.Channels))
	r.reads.Expected = 1e6 / float64(cfg.PeriodUs(0))
	return r
}

// call runs one FOCAS function and logs it for the browser.
func (r *run) call(fn string, f func() error) error {
	t0 := time.Now()
	err := f()
	c := Call{At: t0, Fn: fn, Ms: float64(time.Since(t0).Microseconds()) / 1000, Result: "EW_OK"}
	if err != nil {
		c.Result = err.Error()
	}
	r.mu.Lock()
	r.calls = append(r.calls, c)
	if len(r.calls) > maxCalls {
		r.calls = r.calls[len(r.calls)-maxCalls:]
	}
	r.mu.Unlock()
	return err
}

// cleanup is call for the tidying before a setup, which the CNC refuses
// (EW_REJECT) when there is nothing to tidy.
func (r *run) cleanup(fn string, f func() error) {
	_ = r.call(fn, f)
	r.mu.Lock()
	r.calls[len(r.calls)-1].Cleanup = true
	r.mu.Unlock()
}

// batch is what the writer stores in one transaction.
type batch struct {
	chunks []Chunk
	events []Event
}

// loop connects, starts sampling and reads until asked to stop, the CNC
// finished, or the machine fails. The start result goes to r.ready.
func (r *run) loop() {
	defer close(r.done)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var h *focas.Handle
	err := r.call("cnc_allclibhndl3", func() (err error) {
		h, err = r.lib.Connect(r.m.Host, r.m.Port, 10*time.Second)
		return err
	})
	if err != nil {
		r.fail(fmt.Errorf("connect to machine %s: %w", r.m.ID, err)) // no address: browsers see this
		return
	}
	defer r.call("cnc_freelibhndl", func() error { h.Free(); return nil })

	ext := r.cfg.extended()
	// order[j] is the config channel of the j-th channel set on the CNC
	order := make([]int, len(r.cfg.Channels))
	for i := range order {
		order[i] = i
	}
	// a session left by a process that died keeps the CNC's sampler busy
	// ("sampling already started"; after a failed cnc_sdtreadsmpl2 even new
	// connections fail): close whatever is there before setting up
	r.cleanup("cnc_sdtcancelsmpl", h.Cancel)
	if ext {
		r.cleanup("cnc_sdtendsmpl2", h.End2)
	}
	r.cleanup("cnc_sdtclrchnl", h.ClearChannels)

	if ext {
		var chs []focas.Channel2
		chs, order, err = r.cfg.focasChannels2()
		if err == nil {
			err = r.call("cnc_sdtsetchnl2", func() error { return h.SetChannels2(chs, r.cfg.points()) })
			if focas.Is(err, focas.EW_VERSION) {
				err = fmt.Errorf("%w: this CNC does not support one of the channels (PMC byte/word channels need a 30i-B with newer software)", err)
			}
		}
		if err == nil {
			cond, trg := r.cfg.trigger()
			err = r.call("cnc_sdtstartsmpl2", func() error { return h.Start2(r.cfg.path(), cond, trg) })
		}
		if err != nil {
			r.fail(err)
			return
		}
	} else {
		chs := r.cfg.focasChannels()
		err := r.call("cnc_sdtsetchnl", func() error { return h.SetChannels(chs, r.cfg.points()) })
		if err == nil {
			err = r.call("cnc_sdtstartsmpl", func() error { return h.Start(r.cfg.path(), int32(r.cfg.StartSeq)) })
		}
		if err != nil {
			r.fail(err)
			return
		}
	}
	started := time.Now()
	id, err := r.store.CreateSession(r.m.ID, r.m.Name, r.cfg, started)
	if err != nil {
		r.closeSampling(h, false)
		r.fail(err)
		return
	}
	state := "running"
	if !ext && r.cfg.StartSeq > 0 {
		state = "waiting" // until block N runs
	}
	r.mu.Lock()
	r.session, r.started, r.state = id, started, state
	r.mu.Unlock()
	r.ready <- nil
	log.Printf("sampling start: machine %s session %d %s (%s)", r.m.ID, id, r.cfg.Mode, r.lib.Path)

	// the writer keeps database writes from delaying the reads: the CNC's
	// buffer overflows (EW_BUFFER) if it is not read in time
	batches := make(chan batch, 64)
	var writeErr error
	writeFailed := make(chan error, 1) // the loop stops at once: reading on would only lose data
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for b := range batches {
			if writeErr != nil {
				continue // drained, not written
			}
			if err := r.store.Add(id, b.chunks, b.events); err != nil {
				writeErr = err
				writeFailed <- err
				log.Printf("sampling session %d: write: %v", id, err)
			}
		}
	}()

	reader := h.NewReader(len(r.cfg.Channels), readPoints)
	defer reader.Close()
	tick := time.NewTicker(readEvery)
	defer tick.Stop()
	var pending batch
	lastFlush := time.Now()
	window := struct {
		at            time.Time
		base          int64
		calls         int
		ms            float64
		slow, healthy int // consecutive seconds behind / keeping up
	}{at: time.Now()}
	var failure, lowDisk error
	finished, stopping := false, false // finished: the CNC took all points
	lastDiskCheck := time.Now()

	for {
		select {
		case <-r.stop:
			stopping = true
		case err := <-writeFailed:
			failure = fmt.Errorf("saving the samples failed, so sampling stopped: %w", err)
		case <-tick.C:
		}
		if failure != nil {
			break
		}
		// a disk running full stops the session in good order, before
		// writes fail (and the PC with them)
		if !stopping && time.Since(lastDiskCheck) >= diskEvery {
			lastDiskCheck = time.Now()
			if lowDisk = r.store.lowDisk(); lowDisk != nil {
				stopping = true
			}
		}
		t0 := time.Now()
		var stat int16 = 1
		var trg uint16
		if ext {
			trg, err = reader.Read2()
		} else {
			stat, err = reader.Read()
		}
		now := time.Now()
		ms := float64(now.Sub(t0).Microseconds()) / 1000
		if err != nil {
			if focas.Is(err, focas.EW_BUFFER) {
				err = fmt.Errorf("%w: the CNC's sample buffer overflowed (read too late)", err)
			}
			failure = err
			_ = r.call(map[bool]string{false: "cnc_sdtreadsmpl", true: "cnc_sdtreadsmpl2"}[ext], func() error { return err })
		} else {
			r.mu.Lock()
			got := false
			for j, n := range reader.Counts {
				i := order[j]
				r.reads.LastPoints[i] = max(n, 0)
				if n <= 0 {
					continue
				}
				got = true
				data := append([]uint16(nil), reader.Data[j][:n]...)
				pending.chunks = append(pending.chunks, Chunk{Channel: i, First: r.samples[i], Data: data, ReceivedAt: now})
				r.samples[i] += int64(n)
				r.rings[i].push(data)
			}
			if got && r.firstData.IsZero() {
				r.firstData = now
				if r.state == "waiting" {
					r.state = "running"
				}
			}
			// a trigger shows in the one read that ends at it; after the end
			// (STSM) every read repeats the bits, but the loop stops there
			if trg != 0 {
				e := Event{TimeUs: max(r.samples[0]-1, 0) * int64(r.cfg.PeriodUs(0)), Bits: trg, At: now}
				pending.events = append(pending.events, e)
				r.events = append(r.events, e)
				if len(r.events) > maxEvents {
					r.events = r.events[len(r.events)-maxEvents:]
				}
			}
			r.reads.Calls++
			r.reads.MaxPoints = max(r.reads.MaxPoints, reader.Counts[slices.Index(order, 0)])
			r.reads.MaxMs = max(r.reads.MaxMs, ms)
			window.calls++
			window.ms += ms
			if d := now.Sub(window.at); d >= time.Second {
				r.reads.Rate = float64(r.samples[0]-window.base) / d.Seconds()
				r.reads.AvgMs = window.ms / float64(window.calls)
				// behind: the CNC keeps sampling at its period, so reading
				// fewer points per second than that means data pile up there
				// (basic: EW_BUFFER at last; extended with a number of points:
				// the points not read when the CNC ends are lost)
				if !r.firstData.IsZero() && r.reads.Rate < 0.97*r.reads.Expected {
					window.slow, window.healthy = window.slow+1, 0
				} else {
					window.slow, window.healthy = 0, window.healthy+1
				}
				if window.slow >= 2 {
					r.reads.Behind = true
				} else if window.healthy >= 3 {
					r.reads.Behind = false
				}
				window.at, window.base, window.calls, window.ms = now, r.samples[0], 0, 0
			}
			r.mu.Unlock()
			finished = (!ext && stat == 0) || (ext && trg&focas.TrgEnded != 0)
		}
		if (len(pending.chunks) > 0 || len(pending.events) > 0) &&
			(now.Sub(lastFlush) >= flushEvery || stopping || finished || failure != nil) {
			batches <- pending
			pending, lastFlush = batch{}, now
		}
		if failure != nil || stopping || finished {
			break
		}
	}

	if !focas.Link(failure) {
		r.closeSampling(h, finished)
	}
	close(batches)
	<-writerDone
	if failure == nil && writeErr != nil {
		failure = writeErr
	}

	status, errText := "stopped", ""
	switch {
	case failure != nil:
		status, errText = "failed", failure.Error()
	case finished:
		status = "completed"
	case lowDisk != nil:
		errText = lowDisk.Error()
	}
	r.mu.Lock()
	r.state, r.err, r.ended = status, errText, time.Now()
	samples := append([]int64(nil), r.samples...)
	r.mu.Unlock()
	if err := r.store.FinishSession(id, status, errText, r.ended, samples); err != nil {
		log.Printf("sampling session %d: finish: %v", id, err)
	}
	log.Printf("sampling %s: machine %s session %d %s", status, r.m.ID, id, errText)
}

// closeSampling ends the CNC side: cnc_sdtendsmpl after the CNC finished a
// basic sampling, cnc_sdtcancelsmpl to stop one early. An extended one
// always wants cnc_sdtendsmpl2, which also stops it early (also after a
// failed read, or the CNC's sampler stays stuck); cnc_sdtcancelsmpl only
// if that fails.
func (r *run) closeSampling(h *focas.Handle, finished bool) {
	switch {
	case r.cfg.extended():
		if r.call("cnc_sdtendsmpl2", h.End2) != nil && !finished {
			_ = r.call("cnc_sdtcancelsmpl", h.Cancel)
		}
	case finished:
		_ = r.call("cnc_sdtendsmpl", h.End)
	default:
		_ = r.call("cnc_sdtcancelsmpl", h.Cancel)
	}
}

func (r *run) fail(err error) {
	r.mu.Lock()
	r.state, r.err = "failed", err.Error()
	r.mu.Unlock()
	r.ready <- err
	log.Printf("sampling start failed: machine %s: %v", r.m.ID, err)
}

// requestStop asks the loop to end and waits for it.
func (r *run) requestStop(wait time.Duration) error {
	r.mu.Lock()
	if r.state == "running" || r.state == "waiting" {
		r.state = "stopping"
	}
	r.mu.Unlock()
	select {
	case <-r.stop:
	default:
		close(r.stop)
	}
	select {
	case <-r.done:
		return nil
	case <-time.After(wait):
		return errors.New("sampling did not stop in time")
	}
}

// ring keeps the newest raw words of one channel for the live view.
type ring struct {
	buf  []uint16
	next int
	full bool
}

func newRing(n int) *ring { return &ring{buf: make([]uint16, max(n, 8))} }

func (g *ring) push(data []uint16) {
	for _, w := range data {
		g.buf[g.next] = w
		g.next++
		if g.next == len(g.buf) {
			g.next, g.full = 0, true
		}
	}
}

// last returns up to n newest words, oldest first.
func (g *ring) last(n int) []uint16 {
	size := g.next
	if g.full {
		size = len(g.buf)
	}
	n = min(n, size)
	out := make([]uint16, n)
	start := g.next - n
	for i := range n {
		j := start + i
		if j < 0 {
			j += len(g.buf)
		}
		out[i] = g.buf[j]
	}
	return out
}
