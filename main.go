// deelook serves CNC HMI screens to web browsers, redrawn from values read
// through deemesh-hub. It only reads; nothing it serves can write to a machine.
//
// It also records servo/spindle data sampled by Fanuc CNCs (FOCAS2 cnc_sdt*)
// into SQLite, over its own FOCAS2 connection, on any machine the hub lists
// as nc_focas2_fanuc. Sampling is part of every deelook: it does not start
// without a FOCAS2 library that has cnc_sdt* (Windows 64-bit, Linux 32-bit
// builds).
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

func main() {
	listen := flag.String("listen", ":8090", "address the web server listens on")
	hubURL := flag.String("hub", "http://127.0.0.1:8080", "deemesh-hub base URL")
	defMachine := flag.String("machine", "1", "machine shown when the URL names none (machine_id or machine_name)")
	interval := flag.Duration("interval", time.Second, "time between polls of one screen")
	timeout := flag.Duration("timeout", 5*time.Second, "hub-side limit for one poll")
	lampFlag := flag.String("lamps", "", "operator panel lamps read from the PMC, per machine_id: \"1=ncguide,13=ncguide\" (others follow machine-independent values)")
	fwlib := flag.String("fwlib", "", "FOCAS2 library for sampling, of the executable's bitness (default: Fwlib64.dll / libfwlib32.so* next to the executable)")
	dbPath := flag.String("db", "", "SQLite database for sampling sessions (default: deelook-data/sampling.db next to the executable)")
	open := flag.Bool("open", doubleClicked, "open the page in a browser once listening (default: when started by double-click on Windows)")
	// deelook.toml next to the executable first, so the command line wins;
	// where there is none, one with the defaults is put there to edit
	exeDir := executableDir()
	settingsPath := filepath.Join(exeDir, settingsName)
	if writeSettingsTemplate(settingsPath) {
		log.Printf("wrote %s with the default settings, to edit", settingsPath)
	}
	if err := loadSettings(settingsPath); err != nil {
		fatal("%v", err)
	}
	flag.Parse()
	if *dbPath == "" {
		*dbPath = filepath.Join(exeDir, "deelook-data", "sampling.db")
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
		}
		out := make([]machine, len(ms))
		for i, m := range ms {
			out[i] = machine{m.ID, m.Name, m.Protocol}
		}
		writeJSON(w, map[string]any{"default": *defMachine, "machines": out})
	})

	libPath := *fwlib
	if libPath == "" {
		libPath = focas.FindLibrary(exeDir)
	}
	lib, err := focas.Load(libPath)
	if err != nil {
		fatal("FOCAS2 library (needed for sampling; -fwlib; not in the repository, see fwlib/README.md): %v", err)
	}
	store, err := sampling.OpenStore(*dbPath)
	if err != nil {
		fatal("%v", err)
	}
	defer store.Close()
	sampler := sampling.NewManager(hc, store, lib)
	defer sampler.Close() // never leave sampling running on a CNC
	sampler.Routes(mux)
	log.Printf("FOCAS2 %s (%s); sampling on the hub's %s machines; database %s", lib.Path, lib.Version, "nc_focas2_fanuc", *dbPath)
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
	log.Printf("deelook on %s (hub %s, poll every %s): open %s", *listen, *hubURL, *interval, url)
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

// resolveMachine turns a machine_id or machine_name into the machine_id,
// and refuses machines the screen is not drawn for. Only machines on the
// hub's list are ever polled.
func resolveMachine(ctx context.Context, hc *hub.Client, def *screens.Def, machine string) (string, error) {
	ms, err := hc.Machines(ctx)
	if err != nil {
		return "", err
	}
	for _, m := range ms {
		if strconv.Itoa(m.ID) == machine || m.Name == machine {
			if !def.Accepts(m.Protocol) {
				return "", fmt.Errorf("machine %s (%s) is %s; screen %s is for %v", machine, m.Name, m.Protocol, def.ID, def.Protocols)
			}
			return strconv.Itoa(m.ID), nil
		}
	}
	return "", fmt.Errorf("machine %q is not on the hub's machine list", machine)
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

// spa serves the embedded frontend; unknown paths get index.html.
func spa(dist fs.FS) http.Handler {
	files := http.FileServerFS(dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := fs.Stat(dist, trimSlash(r.URL.Path)); err != nil {
			index, err := fs.ReadFile(dist, "index.html")
			if err != nil {
				http.Error(w, "frontend not built: run ./build.sh (or `npm run dev` in web/)", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(index)
			return
		}
		files.ServeHTTP(w, r)
	})
}

func trimSlash(p string) string {
	if p == "/" || p == "" {
		return "index.html"
	}
	return p[1:]
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
