// deelook serves CNC HMI screens to web browsers, redrawn from values read
// through deemesh-hub. It only reads; nothing it serves can write to a machine.
//
// It also records servo/spindle data sampled by Fanuc CNCs (FOCAS2 cnc_sdt*)
// into SQLite, over its own FOCAS2 connection, on any machine the hub lists
// as nc_focas2_fanuc. That needs FANUC's FOCAS2 library with cnc_sdt*
// (Windows 64-bit, Linux 32-bit builds), which deelook cannot ship: without
// it the HMI screens work and the sampling page says what to put where.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"deelook/internal/focas"
	"deelook/internal/hub"
	"deelook/internal/live"
	"deelook/internal/sampling"
	"deelook/internal/screens"
	"deelook/web"
)

// version is this build's, from the VERSION file (build.sh sets it); "dev"
// for a plain go build.
var version = "dev"

func main() {
	o := defineFlags(flag.CommandLine)
	listen, hubURL, defMachine, interval, timeout := o.listen, o.hub, o.machine, o.interval, o.timeout
	lampFlag, fwlib, open := o.lamps, o.fwlib, o.open
	// command line only: a settings file cannot name itself
	config := flag.String("config", "", "settings file (empty: "+settingsName+" next to the executable); "+
		"relative paths in it are taken from its folder")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse() // for -config; parsed again below, so the command line wins over the file
	if *showVersion {
		fmt.Println("deelook", version)
		return
	}
	// the settings file first; where there is none, one with the defaults
	// is put there to edit
	exeDir := executableDir()
	settingsPath := *config
	if settingsPath == "" {
		settingsPath = filepath.Join(exeDir, settingsName)
	}
	if abs, err := filepath.Abs(settingsPath); err == nil {
		settingsPath = abs
	}
	if writeSettingsTemplate(settingsPath, settingsDefault(exeDir)) {
		log.Printf("wrote %s with the default settings, to edit", settingsPath)
	}
	if _, err := os.Stat(settingsPath); err != nil && *config != "" {
		fatal("settings file %s: %v", settingsPath, err) // asked for by name, and not there
	}
	if err := loadSettings(flag.CommandLine, settingsPath); err != nil {
		fatal("%v", err)
	}
	flag.Parse()
	// the data folder: session files in sessions/, the machine list; an
	// old "db" setting names the single database of earlier builds, whose
	// folder it was
	dataDir, legacyDB := *o.data, ""
	if *o.db != "" {
		legacyDB = *o.db
		if dataDir == "" {
			dataDir = filepath.Dir(*o.db)
		}
	}
	if dataDir == "" {
		dataDir = filepath.Join(exeDir, "deelook-data")
	}
	if legacyDB == "" {
		legacyDB = filepath.Join(dataDir, "sampling.db")
	}

	lamps, err := parseLamps(*lampFlag)
	if err != nil {
		fatal("%v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hc := hub.New(*hubURL, *timeout+5*time.Second)
	// a short linger keeps a page reload from restarting the poll, while a
	// view switch does not leave the old view polling for long
	broker := live.NewBroker(hc, *interval, 5*time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/machines", func(w http.ResponseWriter, r *http.Request) {
		ms, err := hc.Machines(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		type machine struct { // no host/port: those stay on the server
			ID       int    `json:"machine_id"`
			Name     string `json:"machine_name"`
			Protocol string `json:"protocol"`
			Screen   string `json:"screen,omitempty"` // the HMI drawn for it, if any
		}
		out := make([]machine, len(ms))
		for i, m := range ms {
			out[i] = machine{ID: m.ID, Name: m.Name, Protocol: m.Protocol}
			if def := screens.For(m.Protocol); def != nil {
				out[i].Screen = def.ID
			}
		}
		writeJSON(w, map[string]any{"default": *defMachine, "machines": out})
	})

	// the library: as set, else next to the executable, else one uploaded
	// earlier into the data folder (sampling.LibraryDir)
	libPath := *fwlib
	if libPath == "" {
		libPath = focas.FindLibrary(exeDir)
		if _, err := os.Stat(libPath); err != nil {
			if up := focas.FindLibrary(sampling.LibraryDir(dataDir)); up != "" {
				if _, err := os.Stat(up); err == nil {
					libPath = up
				}
			}
		}
	}
	lib, libErr := focas.Load(libPath)
	if libErr != nil {
		lib = nil
		log.Printf("no FOCAS2 library, so no sampling (the HMI screens work): %v", libErr)
		log.Printf("for sampling, upload %s on the sampling page, or put it next to the executable or set fwlib in %s (fwlib/README.md)",
			libraryFiles(), settingsName)
	}
	// the data folder: session files (what gets handed around) and
	// deelook.db (the session list and the machine list with CNC addresses)
	store, err := sampling.OpenStore(dataDir, legacyDB)
	if err != nil {
		fatal("%v", err)
	}
	defer store.Close()
	store.SetMinFree(*o.minFree)
	machines, err := sampling.OpenMachines(store, // and the machine list files of earlier builds
		filepath.Join(dataDir, "machines.json"), filepath.Join(dataDir, "deelook-machines.json"),
		filepath.Join(exeDir, "deelook-machines.json"))
	if err != nil {
		fatal("%v", err)
	}
	sampler := sampling.NewManager(hc, store, machines, lib, libPath, libErr)
	defer sampler.Close() // never leave sampling running on a CNC
	sampler.Routes(mux)
	if lib != nil {
		log.Printf("FOCAS2 %s (%s); sampling on the hub's nc_focas2_fanuc machines and those added by address; data in %s",
			lib.Path, lib.Version, dataDir)
	}
	mux.HandleFunc("GET /api/screens/{screen}/stream", func(w http.ResponseWriter, r *http.Request) {
		def, ok := screens.Get(r.PathValue("screen"))
		if !ok {
			http.Error(w, "unknown screen", http.StatusNotFound)
			return
		}
		q := r.URL.Query()
		view, err := def.View(q.Get("view"), q.Get("page"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		machine := q.Get("machine")
		if machine == "" {
			machine = *defMachine
		}
		id, err := resolveMachine(r.Context(), hc, def, machine)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		opt := screens.Options{Timeout: *timeout, Lamps: lamps[id]}
		serveStream(w, r, broker, def, id, view, opt)
	})
	hubRoutes(mux, hc, settingsPath) // the hub address, changed from any page
	// the manual at / and /help; the app draws /hmi and /sampling
	manual := &help{hub: hc, defMachine: *defMachine, sampler: sampler, dataDir: dataDir}
	mux.Handle("GET /{$}", manual)
	mux.Handle("GET /help", manual)
	mux.Handle("GET /help.md", manual)
	mux.Handle("/", spa(web.Dist()))

	srv := &http.Server{
		Addr:              *listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx }, // ends open streams on shutdown
	}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		fatal("%v", err)
	}
	url := pageURL(ln.Addr())
	hubNote := "hub " + *hubURL
	if *hubURL == "" {
		hubNote = "no deemesh-hub: no HMI screens, sampling on machines added by address"
	}
	log.Printf("deelook %s on %s (%s, poll every %s): open %s", version, *listen, hubNote, *interval, url)
	if doubleClicked {
		log.Printf("closing this window stops deelook")
	}
	if *open {
		openBrowser(url)
	}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// resolveMachine turns a machine_id or machine_name, or "" for the first
// machine, into the machine_id. Only machines on the hub's list are ever polled.
func resolveMachine(ctx context.Context, hc *hub.Client, def *screens.Def, machine string) (string, error) {
	ms, err := hc.Machines(ctx)
	if err != nil {
		return "", err
	}
	m, err := pickMachine(ms, def, machine)
	if err != nil {
		return "", err
	}
	return strconv.Itoa(m.ID), nil
}

// pickMachine finds a machine_id or machine_name on the hub's list, or for
// "" the lowest-numbered machine the screen is drawn for, and refuses
// machines it is not drawn for. ("" rather than a word like "auto", which
// could be a machine's name.)
func pickMachine(ms []hub.Machine, def *screens.Def, machine string) (hub.Machine, error) {
	if machine == "" {
		var pick *hub.Machine
		for i, m := range ms {
			if def.Accepts(m.Protocol) && (pick == nil || m.ID < pick.ID) {
				pick = &ms[i]
			}
		}
		if pick == nil {
			return hub.Machine{}, fmt.Errorf("no machine on the hub's list is %v, which screen %s is for", def.Protocols, def.ID)
		}
		return *pick, nil
	}
	for _, m := range ms {
		if strconv.Itoa(m.ID) == machine || m.Name == machine {
			if !def.Accepts(m.Protocol) {
				return hub.Machine{}, fmt.Errorf("machine %s (%s) is %s; screen %s is for %v", machine, m.Name, m.Protocol, def.ID, def.Protocols)
			}
			return m, nil
		}
	}
	return hub.Machine{}, fmt.Errorf("machine %q is not on the hub's machine list", machine)
}

// executableDir is the folder of the running executable.
func executableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

// parseLamps reads "-lamps 1=ncguide,13=ncguide" into machine_id -> profile.
func parseLamps(s string) (map[string]string, error) {
	out := map[string]string{}
	for _, item := range strings.Split(s, ",") {
		if item = strings.TrimSpace(item); item == "" {
			continue
		}
		id, profile, ok := strings.Cut(item, "=")
		if !ok || profile != "ncguide" {
			return nil, fmt.Errorf("-lamps %q: want machine_id=ncguide", item)
		}
		out[strings.TrimSpace(id)] = profile
	}
	return out, nil
}

// serveStream sends snapshots as Server-Sent Events until the browser leaves.
func serveStream(w http.ResponseWriter, r *http.Request, b *live.Broker, def *screens.Def, machine, view string, opt screens.Options) {
	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no") // nginx: do not buffer the stream

	snaps, unsubscribe := b.Subscribe(def, machine, view, opt)
	defer unsubscribe()

	fmt.Fprint(w, "retry: 3000\n\n")
	if err := rc.Flush(); err != nil {
		return
	}
	ping := time.NewTicker(15 * time.Second) // keeps proxies from closing an idle stream
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case data := <-snaps:
			fmt.Fprintf(w, "data: %s\n\n", data)
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}
}

// spa serves the embedded frontend: index.html for the pages it draws,
// which it tells apart by the path, and its files.
func spa(dist fs.FS) http.Handler {
	files := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimSuffix(r.URL.Path, "/") {
		case "/hmi", "/sampling":
			index, err := fs.ReadFile(dist, "index.html")
			if err != nil {
				http.Error(w, "frontend not built: run ./build.sh (or `npm run dev` in web/)", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(index)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if fi, err := fs.Stat(dist, name); err != nil || fi.IsDir() || name == "index.html" {
			http.Error(w, "not found: the pages are /help, /hmi and /sampling", http.StatusNotFound)
			return
		}
		files.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
