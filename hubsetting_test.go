package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeHub(t *testing.T) {
	for in, want := range map[string]string{
		"":                           "",
		"  ":                         "",
		"http://127.0.0.1:8080":      "http://127.0.0.1:8080",
		"http://127.0.0.1:8080/":     "http://127.0.0.1:8080",
		"192.0.2.10:8080":            "http://192.0.2.10:8080",
		"https://hub.example:8443":   "https://hub.example:8443",
		"ftp://192.0.2.10":           "!",
		"http://192.0.2.10/machine":  "!",
		"http://u:p@192.0.2.10:8080": "!",
		"http://":                    "!",
	} {
		got, err := normalizeHub(in)
		if want == "!" && err == nil || want != "!" && (err != nil || got != want) {
			t.Errorf("%q: %q, %v; want %q", in, got, err, want)
		}
	}
}

// The hub line changes, the rest of the file (comments, other values, the
// [lamps] table, Windows line ends) stays as it was.
func TestSetSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), settingsName)
	orig := "# deelook settings\r\n# the hub\r\nhub = \"http://127.0.0.1:8080\"\r\nlisten = \":8090\"\r\n\r\n[lamps]\r\n1 = \"ncguide\"\r\n"
	if err := os.WriteFile(path, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setSetting(path, "hub", ""); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if want := strings.Replace(orig, `hub = "http://127.0.0.1:8080"`, `hub = ""`, 1); string(b) != want {
		t.Errorf("got\n%q\nwant\n%q", b, want)
	}
	// no hub line yet: one goes in before the first table
	if err := os.WriteFile(path, []byte("listen = \":8090\"\n\n[lamps]\n1 = \"ncguide\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setSetting(path, "hub", "http://192.0.2.10:8080"); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	if want := "listen = \":8090\"\n\nhub = \"http://192.0.2.10:8080\"\n[lamps]\n1 = \"ncguide\"\n"; string(b) != want {
		t.Errorf("got\n%q\nwant\n%q", b, want)
	}
	// and the file still reads as settings, with the new value
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	o := defineFlags(fs)
	if err := loadSettings(fs, path); err != nil || *o.hub != "http://192.0.2.10:8080" {
		t.Errorf("read back: %q, %v", *o.hub, err)
	}
}
