package main

import (
	"flag"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConf(t *testing.T) {
	fs := flag.CommandLine
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	defer func() { flag.CommandLine = fs }()
	hub := flag.String("hub", "http://127.0.0.1:8080", "")
	db := flag.String("db", "", "")
	lamps := flag.String("lamps", "", "")
	open := flag.Bool("open", true, "")

	dir := t.TempDir()
	conf := filepath.Join(dir, confName)
	text := "\uFEFF# comment\n\nhub = http://10.0.0.5:8080\n-lamps = 1=ncguide\ndb = data/s.db\nopen = false\n"
	if err := os.WriteFile(conf, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := loadConf(conf); err != nil {
		t.Fatal(err)
	}
	if *hub != "http://10.0.0.5:8080" || *lamps != "1=ncguide" || *open {
		t.Errorf("hub %q lamps %q open %v", *hub, *lamps, *open)
	}
	if *db != filepath.Join(dir, "data", "s.db") {
		t.Errorf("db %q: relative paths follow the file's folder", *db)
	}
	// the command line, parsed after the file, wins
	if err := flag.CommandLine.Parse([]string{"-hub", "http://other:8080"}); err != nil || *hub != "http://other:8080" {
		t.Errorf("command line: %q %v", *hub, err)
	}

	if err := os.WriteFile(conf, []byte("nosuchoption = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if loadConf(conf) == nil {
		t.Error("unknown option accepted")
	}
	if err := loadConf(filepath.Join(dir, "missing.conf")); err != nil {
		t.Errorf("a missing file: %v", err)
	}
}

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
