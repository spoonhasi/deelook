package main

// Running deelook from a folder without installing: settings in deelook.toml
// next to the executable, data next to it too, and a double-clicked
// deelook.exe opens the browser and keeps its window open on a startup error.

import (
	"bufio"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"deelook/internal/sampling"

	"github.com/BurntSushi/toml"
)

// settingsName is the settings file read from the executable's folder.
const settingsName = "deelook.toml"

// settingsTemplate is a settings file with every setting at its default,
// commented out, written where none is.
//
//go:embed deelook.example.toml
var settingsTemplate []byte

// writeSettingsTemplate puts the template at path unless a file is there.
// It reports whether it wrote one; a folder it cannot write to is no error
// (deelook runs on its defaults).
func writeSettingsTemplate(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return false
	}
	_, err = f.Write(settingsTemplate)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return false
	}
	return true
}

// doubleClicked guesses that deelook was started from Explorer: on Windows,
// with no arguments.
var doubleClicked = runtime.GOOS == "windows" && len(os.Args) == 1

// options are deelook's settings: command-line flags, which deelook.toml
// sets by the same names.
type options struct {
	listen, hub, machine, lamps, fwlib, data, db *string
	interval, timeout                            *time.Duration
	open                                         *bool
	minFree                                      *uint64
}

// defineFlags defines the options on fs with their defaults, which
// deelook.example.toml repeats (TestSettingsTemplate keeps them equal).
func defineFlags(fs *flag.FlagSet) options {
	minFree := uint64(1 << 30)
	fs.Var(sizeValue{&minFree}, "minfree", "stop sampling when the data folder's disk has less free space than this (0: never)")
	return options{
		minFree:  &minFree,
		listen:   fs.String("listen", ":8090", "address the web server listens on"),
		hub:      fs.String("hub", "http://127.0.0.1:8080", "deemesh-hub base URL"),
		machine:  fs.String("machine", "1", "machine shown when the URL names none (machine_id or machine_name)"),
		interval: fs.Duration("interval", time.Second, "time between polls of one screen"),
		timeout:  fs.Duration("timeout", 5*time.Second, "hub-side limit for one poll"),
		lamps:    fs.String("lamps", "", "operator panel lamps read from the PMC, per machine_id: \"1=ncguide,13=ncguide\" (others follow machine-independent values)"),
		fwlib:    fs.String("fwlib", "", "FOCAS2 library for sampling, of the executable's bitness (empty: Fwlib64.dll / libfwlib32.so* next to the executable)"),
		data:     fs.String("data", "", "folder for sampling data: sessions/ (one SQLite file each) and machines.json (empty: deelook-data next to the executable)"),
		db:       fs.String("db", "", deprecated+"the single database of earlier builds; its folder is the data folder and its sessions move to files of their own"),
		open:     fs.Bool("open", doubleClicked, "open the page in a browser once listening (default: when started by double-click on Windows)"),
	}
}

// deprecated starts the usage of an option kept only so old settings files
// still load.
const deprecated = "deprecated: "

// sizeValue is a byte count option written like "1GB".
type sizeValue struct{ n *uint64 }

func (v sizeValue) String() string {
	if v.n == nil {
		return ""
	}
	return sampling.FormatSize(*v.n)
}

func (v sizeValue) Set(s string) error {
	n, err := sampling.ParseSize(s)
	if err == nil {
		*v.n = n
	}
	return err
}

// loadSettings sets flags of fs from a TOML settings file whose keys are
// the option names, lamps being a table (machine_id = "ncguide") and open
// also "auto" (its default). Relative db and fwlib paths are taken from the
// file's folder. A missing file is no error; an unknown key or a value of
// the wrong kind is.
func loadSettings(fs *flag.FlagSet, path string) error {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var settings map[string]any
	if _, err := toml.Decode(strings.TrimPrefix(string(raw), "\uFEFF"), &settings); err != nil { // Notepad may save a BOM
		return fmt.Errorf("%s: %v", path, err)
	}
	keys := make([]string, 0, len(settings))
	for k := range settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "open" && settings[key] == "auto" {
			continue // the default: when started by double-click
		}
		value, err := settingValue(key, settings[key])
		if err != nil {
			return fmt.Errorf("%s: %s: %v", path, key, err)
		}
		if (key == "data" || key == "db" || key == "fwlib") && value != "" && !filepath.IsAbs(value) {
			value = filepath.Join(filepath.Dir(path), value)
		}
		if err := fs.Set(key, value); err != nil {
			return fmt.Errorf("%s: %s: %v", path, key, err)
		}
	}
	return nil
}

// settingValue turns a TOML value into the text of its option.
func settingValue(key string, v any) (string, error) {
	switch key {
	case "hub", "listen", "data", "db", "fwlib", "interval", "timeout", "minfree":
		if s, ok := v.(string); ok {
			return s, nil
		}
		return "", errors.New(`want a string in quotes, like "1s" or "http://127.0.0.1:8080"`)
	case "machine":
		switch x := v.(type) {
		case string:
			return x, nil
		case int64:
			return strconv.FormatInt(x, 10), nil
		}
		return "", errors.New("want a machine_id or a machine name")
	case "open":
		if b, ok := v.(bool); ok {
			return strconv.FormatBool(b), nil
		}
		return "", errors.New(`want true, false or "auto"`)
	case "lamps":
		table, ok := v.(map[string]any)
		if !ok {
			return "", errors.New(`want a table: [lamps] then lines like 1 = "ncguide"`)
		}
		ids := make([]string, 0, len(table))
		for id := range table {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { // machine ids in number order
			a, errA := strconv.Atoi(ids[i])
			b, errB := strconv.Atoi(ids[j])
			if errA == nil && errB == nil {
				return a < b
			}
			return ids[i] < ids[j]
		})
		var items []string
		for _, id := range ids {
			p, ok := table[id].(string)
			if !ok {
				return "", fmt.Errorf(`machine %s: want a profile name in quotes, like "ncguide"`, id)
			}
			items = append(items, id+"="+p)
		}
		return strings.Join(items, ","), nil
	}
	return "", errors.New("unknown setting")
}

// libraryFiles names the FOCAS2 files sampling needs on this platform.
func libraryFiles() string {
	if runtime.GOOS == "windows" {
		return "Fwlib64.dll and fwlibe64.dll"
	}
	return "libfwlib32.so (32-bit)"
}

// pageURL is where a browser on this PC finds deelook listening on addr.
func pageURL(addr net.Addr) string {
	host, port, err := net.SplitHostPort(addr.String())
	if err != nil {
		return "http://localhost:8090/"
	}
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port) + "/"
}

// openBrowser shows url in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	if err := cmd.Start(); err != nil {
		log.Printf("open a browser at %s (%v)", url, err)
		return
	}
	go func() { _ = cmd.Wait() }()
}

// fatal stops deelook at startup. Started by double-click, the console
// window would close before the message can be read, so it waits for Enter.
func fatal(format string, args ...any) {
	log.Printf(format, args...)
	if doubleClicked {
		fmt.Fprint(os.Stderr, "\nPress Enter to close.")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	os.Exit(1)
}
