// Package screens defines, per controller type, which deemesh addresses are
// read for each view of its HMI and under which keys the browser gets them.
package screens

import (
	"context"
	"slices"
	"time"

	"deelook/internal/hub"
)

// PollFunc reads one snapshot of a view's values, keyed for the browser.
// A value the machine could not give is a negative envelope in the map; the
// error return is for a poll that did not reach the hub at all.
type PollFunc func(ctx context.Context) (map[string]hub.Envelope, error)

// Options are per-machine settings that change what is read.
type Options struct {
	Timeout time.Duration // hub-side limit for one batch
	Lamps   string        // panel lamp profile ("" = from machine-independent values)
}

type Def struct {
	ID        string
	Protocols []string // deemesh protocols this HMI is drawn for
	// View checks a view requested by the browser and returns its canonical
	// name, which is also the key its poller is shared under.
	View func(view, page string) (string, error)
	// New prepares polling of one view on one machine. State kept between
	// polls (axis count, cached program comment) lives in the closure.
	New func(c *hub.Client, machine, view string, opt Options) PollFunc
}

func (d *Def) Accepts(protocol string) bool { return slices.Contains(d.Protocols, protocol) }

var registry = map[string]*Def{}

func register(d *Def) *Def {
	registry[d.ID] = d
	return d
}

func Get(id string) (*Def, bool) {
	d, ok := registry[id]
	return d, ok
}

// For is the screen drawn for machines of a protocol, nil when none is.
func For(protocol string) *Def {
	for _, d := range registry {
		if d.Accepts(protocol) {
			return d
		}
	}
	return nil
}
