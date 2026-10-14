package main

import (
	"log"
	"runtime"

	"golang.org/x/sys/windows"
)

// openBrowser shows url in the default browser the way Explorer opens a
// link, through the shell, without starting another program to do it
// (rundll32 url.dll,FileProtocolHandler, the usual way, gets deelook taken
// for malware by antivirus heuristics).
func openBrowser(url string) {
	go func() {
		// the shell may hand the link over through COM
		runtime.LockOSThread()
		if windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE) == nil {
			defer windows.CoUninitialize()
		}
		verb, _ := windows.UTF16PtrFromString("open")
		file, err := windows.UTF16PtrFromString(url)
		if err == nil {
			err = windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
		}
		if err != nil {
			log.Printf("open a browser at %s (%v)", url, err)
		}
	}()
}
