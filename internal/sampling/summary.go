package sampling

import (
	"log"
	"time"

	"deelook/internal/hub"
)

// Summary is the state of sampling in short, for the help page: what the
// sampling page would show, without asking a CNC anything.
type Summary struct {
	Library    bool   // a FOCAS2 library with cnc_sdt* is loaded
	LibPath    string // the library, or where it was looked for
	LibVersion string
	Extended   bool   // it has the cnc_sdt*2 calls
	LibError   string // why there is none
	Machines   []SummaryMachine
	Running    []Status
	Storage    Storage
}

// SummaryMachine is a machine that can sample, without its address.
type SummaryMachine struct {
	ID, Name, Source string // source: hub | local
	Axes, Spindles   []string
}

// Summary takes the hub's machine list as just read (or the error that kept
// it from being read), which brings the stored list up to date as the
// sampling page does.
func (m *Manager) Summary(ms []hub.Machine, hubErr error) Summary {
	if hubErr == nil {
		if err := m.list.syncHub(ms, time.Now()); err != nil {
			log.Printf("machine list in %s: %v", indexName, err)
		}
	}
	s := Summary{LibPath: m.libPath, Storage: m.store.Storage()}
	if m.lib != nil {
		s.Library, s.LibPath, s.LibVersion, s.Extended = true, m.lib.Path, m.lib.Version, m.lib.Extended()
	} else if m.libErr != nil {
		s.LibError = m.libErr.Error()
	}
	for _, mc := range m.list.list() {
		s.Machines = append(s.Machines, SummaryMachine{mc.ID, mc.Name, mc.Source, mc.Axes, mc.Spindles})
	}
	for _, st := range m.Status() {
		switch st.State {
		case "starting", "waiting", "running", "stopping":
			s.Running = append(s.Running, st)
		}
	}
	return s
}
