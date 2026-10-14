//go:build !windows

package main

import (
	"log"
	"os/exec"
	"runtime"
)

// openBrowser shows url in the default browser.
func openBrowser(url string) {
	if runtime.GOOS != "linux" {
		return
	}
	cmd := exec.Command("xdg-open", url)
	if err := cmd.Start(); err != nil {
		log.Printf("open a browser at %s (%v)", url, err)
		return
	}
	go func() { _ = cmd.Wait() }()
}
