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
	fs := flag.CommandLine
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	defer func() { flag.CommandLine = fs }()
	hub := flag.String("hub", "http://127.0.0.1:8080", "")
	machine := flag.String("machine", "1", "")
	interval := flag.Duration("interval", time.Second, "")
	db := flag.String("db", "", "")
	lamps := flag.String("lamps", "", "")
	open := flag.Bool("open", true, "")

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
	if err := loadSettings(path); err != nil {
		t.Fatal(err)
	}
	if *hub != "http://10.0.0.5:8080" || *machine != "13" || *interval != 500*time.Millisecond || *open {
		t.Errorf("hub %q machine %q interval %v open %v", *hub, *machine, *interval, *open)
	}
	if *lamps != "1=ncguide,13=ncguide" {
		t.Errorf("lamps %q", *lamps)
	}
	if *db != filepath.Join(dir, `data\s.db`) {
		t.Errorf("db %q: relative paths follow the file's folder", *db)
	}
	// the command line, parsed after the file, wins
	if err := flag.CommandLine.Parse([]string{"-hub", "http://other:8080"}); err != nil || *hub != "http://other:8080" {
		t.Errorf("command line: %q %v", *hub, err)
	}

	for text, want := range map[string]string{
		"nosuch = 1\n":            "unknown setting",
		"hub = 8080\n":            "want a string",
		"open = \"yes\"\n":        "want true or false",
		"lamps = \"1=ncguide\"\n": "want a table",
		"interval = \"soon\"\n":   "interval",
		"hub = \"http://x\n":      "line 1",
	} {
		write(text)
		if err := loadSettings(path); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want an error about %q", text, err, want)
		}
	}
	if err := loadSettings(filepath.Join(dir, "missing.toml")); err != nil {
		t.Errorf("a missing file: %v", err)
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
