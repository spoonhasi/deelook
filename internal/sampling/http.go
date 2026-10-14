package sampling

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"sync"
	"time"

	"deelook/internal/focas"
)

// Routes registers the sampling API.
//
//	GET  /api/sampling/types                 data types per channel kind
//	GET  /api/sampling/machines              machines that can sample (the hub's Fanuc ones, stored
//	                                         for when the hub is down, and those added here), with axes
//	POST /api/sampling/machines              add a machine by address, body: {name, host, port}
//	DELETE /api/sampling/machines/{machine}  remove a machine added here
//	POST /api/sampling/machines/{machine}/names  read its axis names over FOCAS2
//	GET  /api/sampling/status                state of the machines that sampled
//	POST /api/sampling/{machine}/start       body: Config
//	POST /api/sampling/{machine}/stop
//	POST /api/sampling/{machine}/oneshot     body: OneShot (cnc_sdtread1shot)
//	GET  /api/sampling/{machine}/live        newest samples (?window=3s&buckets=600)
//	GET  /api/sampling/sessions              recorded sessions (?machine=&limit=)
//	GET  /api/sampling/sessions/{id}/csv     one session as CSV
//	GET  /api/sampling/sessions/{id}/view    a session reduced for charts (?from=&to= µs, &buckets=)
func (m *Manager) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sampling/types", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"servo": ServoTypes, "spindle": SpindleTypes, "cnc": CncInfoTypes,
			"pmcKinds": focas.PmcKinds})
	})
	mux.HandleFunc("GET /api/sampling/machines", m.handleMachines)
	mux.HandleFunc("POST /api/sampling/machines", func(w http.ResponseWriter, r *http.Request) {
		if !jsonRequest(w, r) {
			return
		}
		var req struct {
			Name string `json:"name"`
			Host string `json:"host"`
			Port int    `json:"port"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
			http.Error(w, "bad request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		id, err := m.AddMachine(req.Name, req.Host, req.Port)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, map[string]string{"machine": id})
	})
	mux.HandleFunc("DELETE /api/sampling/machines/{machine}", func(w http.ResponseWriter, r *http.Request) {
		if err := m.DeleteMachine(r.PathValue("machine")); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/sampling/machines/{machine}/names", func(w http.ResponseWriter, r *http.Request) {
		if !jsonRequest(w, r) {
			return
		}
		if err := m.RefreshNames(r.Context(), r.PathValue("machine")); err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/sampling/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, m.Status())
	})
	mux.HandleFunc("POST /api/sampling/{machine}/start", func(w http.ResponseWriter, r *http.Request) {
		if !jsonRequest(w, r) {
			return
		}
		var cfg Config
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&cfg); err != nil {
			http.Error(w, "bad request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		st, err := m.Start(r.Context(), r.PathValue("machine"), cfg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, st)
	})
	mux.HandleFunc("POST /api/sampling/{machine}/stop", func(w http.ResponseWriter, r *http.Request) {
		if !jsonRequest(w, r) {
			return
		}
		st, err := m.Stop(r.PathValue("machine"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, st)
	})
	mux.HandleFunc("POST /api/sampling/{machine}/oneshot", func(w http.ResponseWriter, r *http.Request) {
		if !jsonRequest(w, r) {
			return
		}
		var req OneShot
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
			http.Error(w, "bad request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		res, err := m.ReadOnce(r.Context(), r.PathValue("machine"), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		writeJSON(w, res)
	})
	mux.HandleFunc("GET /api/sampling/{machine}/live", func(w http.ResponseWriter, r *http.Request) {
		window, err := time.ParseDuration(r.URL.Query().Get("window"))
		if err != nil || window <= 0 || window > liveWindow {
			window = 3 * time.Second
		}
		buckets, err := strconv.Atoi(r.URL.Query().Get("buckets"))
		if err != nil || buckets < 10 || buckets > 4000 {
			buckets = 600
		}
		writeJSON(w, m.Live(r.PathValue("machine"), window, buckets))
	})
	mux.HandleFunc("GET /api/sampling/sessions", func(w http.ResponseWriter, r *http.Request) {
		limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
		if err != nil || limit <= 0 || limit > 1000 {
			limit = 50
		}
		list, err := m.store.Sessions(r.URL.Query().Get("machine"), limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, list)
	})
	mux.HandleFunc("GET /api/sampling/sessions/{id}/view", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad session id", http.StatusBadRequest)
			return
		}
		q := r.URL.Query()
		from, _ := strconv.ParseInt(q.Get("from"), 10, 64)
		to, _ := strconv.ParseInt(q.Get("to"), 10, 64)
		buckets, err := strconv.Atoi(q.Get("buckets"))
		if err != nil || buckets < 10 || buckets > 4000 {
			buckets = 600
		}
		v, err := m.store.View(id, from, to, buckets)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, v)
	})
	mux.HandleFunc("GET /api/sampling/sessions/{id}/csv", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad session id", http.StatusBadRequest)
			return
		}
		si, err := m.store.Session(id)
		if err != nil {
			http.Error(w, "no such session", http.StatusNotFound)
			return
		}
		name := fmt.Sprintf("deelook-%s-%s-session%d.csv", si.MachineName, si.StartedAt.Local().Format("20060102-150405"), si.ID)
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		if err := m.store.WriteCSV(w, id); err != nil {
			// headers are gone already; end the body so the file is visibly cut
			fmt.Fprintf(w, "\n# export failed: %v\n", err)
		}
	})
}

// MachineInfo is a machine that can sample, with the axes to pick from.
type MachineInfo struct {
	Machine  string   `json:"machine"`
	Name     string   `json:"name"`
	Source   string   `json:"source"`          // hub | local (added in deelook)
	Axes     []string `json:"axes"`            // servo axes in logical order (all paths); empty when not known
	Spindles []string `json:"spindles"`        // spindles in logical order
	Paths    int      `json:"paths"`           // CNC paths; 0 when not known
	Error    string   `json:"error,omitempty"` // cannot sample
	Note     string   `json:"note,omitempty"`  // can sample, but something is missing
}

func (m *Manager) handleMachines(w http.ResponseWriter, r *http.Request) {
	list, hubErr := m.machines(r.Context())
	type hubState struct {
		OK       bool       `json:"ok"`
		Error    string     `json:"error,omitempty"`
		SyncedAt *time.Time `json:"syncedAt,omitempty"` // when the hub's list was last received
	}
	out := struct {
		Library  map[string]any `json:"library"`
		Hub      hubState       `json:"hub"`
		Machines []MachineInfo  `json:"machines"`
	}{Library: map[string]any{"path": m.lib.Path, "version": m.lib.Version, "extended": m.lib.Extended()},
		Hub: hubState{OK: hubErr == nil}, Machines: []MachineInfo{}}
	if hubErr != nil {
		out.Hub.Error = hubErr.Error()
	}
	for _, mc := range list {
		if mc.Source == "hub" && !mc.UpdatedAt.IsZero() && (out.Hub.SyncedAt == nil || mc.UpdatedAt.After(*out.Hub.SyncedAt)) {
			t := mc.UpdatedAt
			out.Hub.SyncedAt = &t
		}
		mi := MachineInfo{Machine: mc.ID, Name: mc.Name, Source: mc.Source, Axes: mc.Axes, Spindles: mc.Spindles, Paths: mc.Paths}
		if mi.Axes == nil {
			mi.Axes = []string{}
		}
		if mi.Spindles == nil {
			mi.Spindles = []string{}
		}
		if _, err := mc.target(); err != nil {
			mi.Error = err.Error()
		}
		out.Machines = append(out.Machines, mi)
	}
	// while the hub answers, its machines' axis names come fresh through
	// deemesh (in parallel: a machine that is off must not hold up the
	// others) and are stored for when it does not
	var wg sync.WaitGroup
	for i := range out.Machines {
		mi := &out.Machines[i]
		if mi.Source != "hub" || hubErr != nil || mi.Error != "" {
			continue
		}
		wg.Go(func() {
			fresh := MachineInfo{Machine: mi.Machine, Axes: []string{}, Spindles: []string{}}
			err := m.axes(r, &fresh)
			if err == nil && len(fresh.Axes) > 0 {
				mi.Axes, mi.Spindles, mi.Paths = fresh.Axes, fresh.Spindles, fresh.Paths
				_ = m.store.setNames(mi.Machine, fresh.Axes, fresh.Spindles, fresh.Paths)
			} else if err != nil && len(mi.Axes) == 0 {
				mi.Note = "axis names from deemesh: " + err.Error()
			}
		})
	}
	wg.Wait()
	for i := range out.Machines {
		if mi := &out.Machines[i]; len(mi.Axes) == 0 && mi.Note == "" && mi.Error == "" {
			mi.Note = "axis names not known"
		}
	}
	writeJSON(w, out)
}

// axes reads axis and spindle names through deemesh. FOCAS numbers them
// across all paths in parameter order (No.981/982), which is path 1's axes,
// then path 2's.
func (m *Manager) axes(r *http.Request, mi *MachineInfo) error {
	cc, err := m.hub.Read(r.Context(), "/machine/channelCount?machine="+mi.Machine)
	if err != nil {
		return err
	}
	var channels int
	if !cc.OK() || json.Unmarshal(cc.Value, &channels) != nil || channels < 1 {
		channels = 1
	}
	mi.Paths = channels
	var cmds []string
	for c := 1; c <= channels; c++ {
		q := fmt.Sprintf("?machine=%s&channel=%d", mi.Machine, c)
		cmds = append(cmds, "/machine/channel/axisCount"+q, "/machine/channel/spindleCount"+q)
	}
	data, err := m.hub.ReadBatch(r.Context(), cmds, 5*time.Second)
	if err != nil {
		return err
	}
	var names []string
	for c := 1; c <= channels; c++ {
		var n, s int
		_ = json.Unmarshal(data[2*(c-1)].Value, &n)
		_ = json.Unmarshal(data[2*(c-1)+1].Value, &s)
		if n > 0 {
			names = append(names, fmt.Sprintf("/machine/channel/axis/axisName?machine=%s&channel=%d&axis=1-%d", mi.Machine, c, n))
		}
		for range s {
			mi.Spindles = append(mi.Spindles, "S"+strconv.Itoa(len(mi.Spindles)+1))
		}
	}
	if len(names) == 0 {
		return nil
	}
	res, err := m.hub.ReadBatch(r.Context(), names, 5*time.Second)
	if err != nil {
		return err
	}
	for _, env := range res {
		var list []string
		if env.OK() && json.Unmarshal(env.Value, &list) == nil {
			mi.Axes = append(mi.Axes, list...)
		}
	}
	return nil
}

// jsonRequest refuses state-changing requests that are not JSON, so a page
// on another site cannot start or stop sampling with a plain form post.
func jsonRequest(w http.ResponseWriter, r *http.Request) bool {
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if ct != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
