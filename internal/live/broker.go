// Package live polls each (screen, machine, view) once, however many
// browsers watch it, and fans the snapshots out to the subscribers.
//
// deemesh shares one connection per machine among all its clients, so the
// number of viewers must not multiply the load on the machine.
package live

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"deelook/internal/hub"
	"deelook/internal/screens"
)

// Snapshot is what the browser receives on every poll.
type Snapshot struct {
	Screen  string                  `json:"screen"`
	Machine string                  `json:"machine"`
	View    string                  `json:"view"`
	At      time.Time               `json:"at"`              // when this poll finished (server clock)
	OK      bool                    `json:"ok"`              // false: the hub could not be reached
	Error   string                  `json:"error,omitempty"` // why, when !OK
	Values  map[string]hub.Envelope `json:"values,omitempty"`
}

type Broker struct {
	hub      *hub.Client
	interval time.Duration // time between poll starts
	linger   time.Duration // keep polling this long after the last viewer leaves

	mu    sync.Mutex
	feeds map[string]*feed
}

func NewBroker(c *hub.Client, interval, linger time.Duration) *Broker {
	return &Broker{hub: c, interval: interval, linger: linger, feeds: map[string]*feed{}}
}

type feed struct {
	key  string
	subs map[chan []byte]struct{}
	last []byte // latest marshaled snapshot, sent at once to a new subscriber
	stop context.CancelFunc
	idle *time.Timer // pending stop after the last subscriber left
}

// Subscribe starts (or joins) the poller for one view of def on machine.
// view must be canonical (def.View). The channel holds at most one pending
// snapshot: a slow reader skips to the newest. Call the returned func when done.
func (b *Broker) Subscribe(def *screens.Def, machine, view string, opt screens.Options) (<-chan []byte, func()) {
	key := def.ID + "/" + machine + "/" + view
	ch := make(chan []byte, 1)

	b.mu.Lock()
	f := b.feeds[key]
	if f == nil {
		ctx, cancel := context.WithCancel(context.Background())
		f = &feed{key: key, subs: map[chan []byte]struct{}{}, stop: cancel}
		b.feeds[key] = f
		go b.run(ctx, f, def, machine, view, opt)
		log.Printf("poll start %s", key)
	}
	if f.idle != nil {
		f.idle.Stop()
		f.idle = nil
	}
	f.subs[ch] = struct{}{}
	if f.last != nil {
		ch <- f.last
	}
	b.mu.Unlock()

	return ch, func() { b.unsubscribe(f, ch) }
}

func (b *Broker) unsubscribe(f *feed, ch chan []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(f.subs, ch)
	if len(f.subs) > 0 || f.idle != nil {
		return
	}
	f.idle = time.AfterFunc(b.linger, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if len(f.subs) == 0 && b.feeds[f.key] == f {
			f.stop()
			delete(b.feeds, f.key)
			log.Printf("poll stop %s", f.key)
		}
	})
}

func (b *Broker) run(ctx context.Context, f *feed, def *screens.Def, machine, view string, opt screens.Options) {
	poll := def.New(b.hub, machine, view, opt)
	var lastValues map[string]hub.Envelope
	for {
		start := time.Now()
		pctx, cancel := context.WithTimeout(ctx, opt.Timeout+5*time.Second)
		vals, err := poll(pctx)
		cancel()
		if ctx.Err() != nil {
			return
		}

		snap := Snapshot{Screen: def.ID, Machine: machine, View: view, At: time.Now(), OK: err == nil, Values: vals}
		if err != nil {
			snap.Error = err.Error()
			snap.Values = lastValues // keep showing the last values, marked stale
		} else {
			lastValues = vals
		}
		data, merr := json.Marshal(snap)
		if merr != nil {
			log.Printf("poll %s: %v", f.key, merr)
		} else {
			b.publish(f, data)
		}

		wait := b.interval - time.Since(start)
		if wait < 100*time.Millisecond {
			wait = 100 * time.Millisecond // never poll back to back
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (b *Broker) publish(f *feed, data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	f.last = data
	for ch := range f.subs {
		select {
		case <-ch: // drop the unread older snapshot
		default:
		}
		ch <- data
	}
}
