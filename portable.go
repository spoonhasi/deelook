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

// loadSettings sets flags from a TOML settings file whose keys are the
// option names, lamps being a table (machine_id = "ncguide"). Relative db
// and fwlib paths are taken from the file's folder. A missing file is no
// error; an unknown key or a value of the wrong kind is.
func loadSettings(path string) error {
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
		value, err := settingValue(key, settings[key])
		if err != nil {
			return fmt.Errorf("%s: %s: %v", path, key, err)
		}
		if (key == "db" || key == "fwlib") && value != "" && !filepath.IsAbs(value) {
			value = filepath.Join(filepath.Dir(path), value)
		}
		if err := flag.Set(key, value); err != nil {
			return fmt.Errorf("%s: %s: %v", path, key, err)
		}
	}
	return nil
}

// settingValue turns a TOML value into the text of its option.
func settingValue(key string, v any) (string, error) {
	switch key {
	case "hub", "listen", "db", "fwlib", "interval", "timeout":
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
		return "", errors.New("want true or false")
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
