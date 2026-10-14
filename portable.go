package main

// Running deelook from a folder without installing: settings in deelook.conf
// next to the executable, data next to it too, and a double-clicked
// deelook.exe opens the browser and keeps its window open on a startup error.

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// confName is the settings file read from the executable's folder.
const confName = "deelook.conf"

// doubleClicked guesses that deelook was started from Explorer: on Windows,
// with no arguments.
var doubleClicked = runtime.GOOS == "windows" && len(os.Args) == 1

// loadConf sets flags from a settings file: one "name = value" per line,
// names as on the command line, # comments. Relative db and fwlib paths are
// taken from the file's folder. A missing file is no error.
func loadConf(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\uFEFF")) // Notepad may save a BOM
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		name = strings.TrimLeft(strings.TrimSpace(name), "-")
		value = strings.Trim(strings.TrimSpace(value), `"`)
		if !ok || name == "" {
			return fmt.Errorf("%s line %d: want name = value", path, n)
		}
		if (name == "db" || name == "fwlib") && value != "" && !filepath.IsAbs(value) {
			value = filepath.Join(filepath.Dir(path), value)
		}
		if err := flag.Set(name, value); err != nil {
			return fmt.Errorf("%s line %d: %s: %v", path, n, name, err)
		}
	}
	return sc.Err()
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
