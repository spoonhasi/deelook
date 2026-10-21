package sampling

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"deelook/internal/focas"
)

// Without a FOCAS2 library the sampling page takes one by upload: a
// library is code deelook runs, so only the FANUC files deelook was tried
// with (fwlib/README.md, by SHA-256) go in this way. Any other build still
// works when put next to the executable (or named by fwlib) by hand. The
// files are kept in the data folder's fwlib/ and never leave this PC.

// knownLib is a FANUC FOCAS2 library file deelook was tried with.
type knownLib struct{ name, platform, version string }

var knownLibs = map[string]knownLib{
	"7b3837e3925902d1e06c5c7e4f9ce39fe7f39aa9caed029096187f5641ec04ab": {"Fwlib64.dll", "windows/amd64", "7.3.0.1"},
	"d3341b43bf3945bdb49d03d1d96436a75911bf3c8d1cdf4f30b9167ef935bfd6": {"fwlibe64.dll", "windows/amd64", "5.3.0.1"},
	"5c10629358c8af28e83a451070436229c180f86f183ac34b9629cfb0d9a71194": {"libfwlib32.so.1.0.2", "linux/386", "1.0.2"},
	"d62b5efc41688c86b31fd029d230c42af9986689b0d3d8ae9739563d171d233d": {"libfwlib32.so.1.0.1", "linux/arm", "1.0.1"},
}

// neededLibs are the files a platform needs together; the first is loaded
// (on Windows it loads the second by name for Ethernet).
var neededLibs = map[string][]string{
	"windows/amd64": {"Fwlib64.dll", "fwlibe64.dll"},
	"linux/386":     {"libfwlib32.so.1.0.2"},
	"linux/arm":     {"libfwlib32.so.1.0.1"},
}

// platform is this build's, as the tables above name it.
const platform = runtime.GOOS + "/" + runtime.GOARCH

// maxLib bounds an upload (the files are 1 to 2 MB).
const maxLib = 32 << 20

// LibraryDir is where uploaded libraries are kept: fwlib/ in the data
// folder (main also looks there at start).
func LibraryDir(dataDir string) string { return filepath.Join(dataDir, "fwlib") }

// handleLibrary takes one library file, raw (Content-Type:
// application/octet-stream, which a form on another site cannot send), and
// once the platform's files are all there loads the library: sampling
// works at once, no restart.
func (m *Manager) handleLibrary(w http.ResponseWriter, r *http.Request) {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/octet-stream" {
		http.Error(w, "send the file as Content-Type: application/octet-stream", http.StatusUnsupportedMediaType)
		return
	}
	m.libMu.Lock()
	defer m.libMu.Unlock()
	if m.library().lib != nil {
		http.Error(w, "a FOCAS2 library is loaded already: to use another, put it next to the executable and restart deelook",
			http.StatusConflict)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxLib))
	if err != nil {
		http.Error(w, "reading the file: "+err.Error(), http.StatusBadRequest)
		return
	}
	sum := sha256.Sum256(body)
	k, ok := knownLibs[hex.EncodeToString(sum[:])]
	if !ok {
		http.Error(w, fmt.Sprintf("deelook이 확인한 FANUC FOCAS2 파일이 아닙니다 (SHA-256 %x…). "+
			"다른 판은 실행 파일 옆에 직접 넣고 deelook을 다시 시작하면 씁니다", sum[:6]), http.StatusBadRequest)
		return
	}
	if k.platform != platform {
		http.Error(w, fmt.Sprintf("%s %s는 %s용입니다 (이 deelook은 %s)", k.name, k.version, k.platform, platform),
			http.StatusBadRequest)
		return
	}
	dir := LibraryDir(filepath.Dir(m.store.dir))
	if err := writeFile(filepath.Join(dir, k.name), body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	need := neededLibs[platform]
	var missing []string
	for _, name := range need {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			missing = append(missing, name)
		}
	}
	out := map[string]any{"file": k.name, "version": k.version, "missing": missing}
	if len(missing) == 0 {
		path := filepath.Join(dir, need[0])
		lib, err := focas.Load(path)
		if err != nil {
			m.libs.Store(&libState{path: path, err: err})
			out["error"] = err.Error()
		} else {
			m.libs.Store(&libState{lib: lib, path: lib.Path})
			log.Printf("FOCAS2 %s (%s) uploaded and loaded: sampling on", lib.Path, lib.Version)
		}
	}
	out["library"] = m.libraryInfo()
	writeJSON(w, out)
}

// writeFile puts data at path through a temporary file, so a half-written
// library is never there under its name.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".upload-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}
