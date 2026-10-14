package main

import (
	"flag"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadSettings(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	o := defineFlags(fs)

	dir := t.TempDir()
	path := filepath.Join(dir, settingsName)
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(BOM + `# a comment
hub = "http://10.0.0.5:8080"   # after a value too
machine = 13
interval = "500ms"
db = 'data\s.db'
open = false

[lamps]
13 = "ncguide"
1 = "ncguide"
`)
	if err := loadSettings(fs, path); err != nil {
		t.Fatal(err)
	}
	if *o.hub != "http://10.0.0.5:8080" || *o.machine != "13" || *o.interval != 500*time.Millisecond || *o.open {
		t.Errorf("hub %q machine %q interval %v open %v", *o.hub, *o.machine, *o.interval, *o.open)
	}
	if *o.lamps != "1=ncguide,13=ncguide" {
		t.Errorf("lamps %q", *o.lamps)
	}
	if *o.db != filepath.Join(dir, `data\s.db`) {
		t.Errorf("db %q: relative paths follow the file's folder", *o.db)
	}
	// the command line, parsed after the file, wins
	if err := fs.Parse([]string{"-hub", "http://other:8080"}); err != nil || *o.hub != "http://other:8080" {
		t.Errorf("command line: %q %v", *o.hub, err)
	}

	for text, want := range map[string]string{
		"nosuch = 1\n":            "unknown setting",
		"hub = 8080\n":            "want a string",
		"open = \"yes\"\n":        `want true, false or "auto"`,
		"lamps = \"1=ncguide\"\n": "want a table",
		"interval = \"soon\"\n":   "interval",
		"hub = \"http://x\n":      "line 1",
	} {
		write(text)
		if err := loadSettings(fs, path); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want an error about %q", text, err, want)
		}
	}
	if err := loadSettings(fs, filepath.Join(dir, "missing.toml")); err != nil {
		t.Errorf("a missing file: %v", err)
	}
}

// TestSettingsTemplate checks that the deelook.toml written on first start
// holds the values deelook uses without it: every option, at its default.
func TestSettingsTemplate(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	o := defineFlags(fs)
	defaults := map[string]string{}
	fs.VisitAll(func(f *flag.Flag) { defaults[f.Name] = f.DefValue })

	dir := t.TempDir()
	path := filepath.Join(dir, settingsName)
	if !writeSettingsTemplate(path) {
		t.Fatal("no template written")
	}
	if err := loadSettings(fs, path); err != nil {
		t.Fatal(err)
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	for name, def := range defaults {
		f := fs.Lookup(name)
		switch {
		case strings.HasPrefix(f.Usage, deprecated): // kept for old files only
		case name == "open": // "auto": left at its default, which depends on how deelook started
			if set[name] {
				t.Errorf("open set to %s, want auto", f.Value)
			}
		case !set[name]:
			t.Errorf("the template does not write %s", name)
		case name == "data": // relative to the file's folder = next to the executable, as main defaults it
			if want := filepath.Join(dir, "deelook-data"); *o.data != want {
				t.Errorf("data %q, want %q", *o.data, want)
			}
		case f.Value.String() != def:
			t.Errorf("%s = %q in the template, default %q", name, f.Value, def)
		}
	}

	// a file already there stays as it is
	const mine = "hub = \"http://x:8080\"\n"
	if err := os.WriteFile(path, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	if writeSettingsTemplate(path) {
		t.Error("wrote over a settings file")
	}
	if b, _ := os.ReadFile(path); string(b) != mine {
		t.Errorf("settings file changed: %q", b)
	}
}

// BOM is what Notepad may put at the start of a UTF-8 file.
const BOM = "\xef\xbb\xbf"

func TestPageURL(t *testing.T) {
	for addr, want := range map[string]string{
		"[::]:8090":         "http://localhost:8090/",
		"0.0.0.0:8090":      "http://localhost:8090/",
		"127.0.0.1:9000":    "http://127.0.0.1:9000/",
		"192.168.0.10:8090": "http://192.168.0.10:8090/",
	} {
		a, _ := net.ResolveTCPAddr("tcp", addr)
		if got := pageURL(a); got != want {
			t.Errorf("%s: %s, want %s", addr, got, want)
		}
	}
}
