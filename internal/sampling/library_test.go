package sampling

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"deelook/internal/focas"
	"deelook/internal/hub"
)

// The page may upload only the FANUC files deelook knows, for its own
// platform, and only while no library is loaded.
func TestLibraryUpload(t *testing.T) {
	dataDir := t.TempDir()
	st, err := OpenStore(dataDir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	m := NewManager(hub.New("", time.Second), st, nil, nil, "Fwlib64.dll", os.ErrNotExist)
	need := neededLibs[platform]
	if len(need) == 0 {
		t.Skip("no FOCAS2 library for", platform)
	}
	// stand-ins for FANUC's files (they are not in the repository)
	fake := func(content string, k knownLib) []byte {
		sum := sha256.Sum256([]byte(content))
		knownLibs[hex.EncodeToString(sum[:])] = k
		t.Cleanup(func() { delete(knownLibs, hex.EncodeToString(sum[:])) })
		return []byte(content)
	}
	other := "linux/arm"
	if platform == other {
		other = "linux/386"
	}
	elsewhere := fake("library of another platform", knownLib{"libfwlib32.so.9", other, "9"})
	first := fake("not really a FOCAS2 library", knownLib{need[0], platform, "0.0"})

	post := func(ctype string, body []byte) (int, map[string]any) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/sampling/library", strings.NewReader(string(body)))
		r.Header.Set("Content-Type", ctype)
		m.handleLibrary(w, r)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	if code, _ := post("multipart/form-data; boundary=x", first); code != http.StatusUnsupportedMediaType {
		t.Errorf("a form upload: %d", code)
	}
	if code, _ := post("application/octet-stream", []byte("some other build")); code != http.StatusBadRequest {
		t.Errorf("an unknown file: %d", code)
	}
	if code, _ := post("application/octet-stream", elsewhere); code != http.StatusBadRequest {
		t.Errorf("another platform's file: %d", code)
	}
	code, out := post("application/octet-stream", first)
	if code != http.StatusOK || out["file"] != need[0] {
		t.Fatalf("a known file: %d %v", code, out)
	}
	if b, _ := os.ReadFile(filepath.Join(LibraryDir(dataDir), need[0])); string(b) != string(first) {
		t.Errorf("stored %q", b)
	}
	if len(need) > 1 {
		if missing, _ := out["missing"].([]any); len(missing) != 1 || missing[0] != need[1] {
			t.Errorf("missing %v, want [%s]", out["missing"], need[1])
		}
	} else if out["error"] == nil { // complete: deelook tried to load it (a stand-in does not load)
		t.Errorf("a stand-in library loaded: %v", out)
	}
	if lib := out["library"].(map[string]any); lib["available"] != false || lib["uploadable"] != true {
		t.Errorf("library %v", lib)
	}

	// a library loaded: no uploads
	m.libs.Store(&libState{lib: &focas.Lib{Path: "x"}, path: "x"})
	if code, _ := post("application/octet-stream", first); code != http.StatusConflict {
		t.Errorf("upload with a library loaded: %d", code)
	}
}
