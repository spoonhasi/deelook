package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"deelook/internal/hub"
	"deelook/internal/sampling"
)

func TestHelp(t *testing.T) {
	if !strings.Contains(helpText, statusLine) {
		t.Fatal("help.md has no status line to replace")
	}
	// a hub with two machines, one of them FANUC; their addresses must not
	// reach the page
	hubSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":0,"value":[
			{"machine_id":1,"machine_name":"bench-31i","protocol":"nc_focas2_fanuc","host":"192.0.2.10","port":8193},
			{"machine_id":2,"machine_name":"mill","protocol":"nc_opcua_siemens","host":"192.0.2.11","port":4840}]}`))
	}))
	defer hubSrv.Close()
	dir := t.TempDir()
	st, err := sampling.OpenStore(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ms, err := sampling.OpenMachines(st)
	if err != nil {
		t.Fatal(err)
	}
	hc := hub.New(hubSrv.URL, 5*time.Second)
	h := &help{hub: hc, hubURL: hubSrv.URL, defMachine: "auto", dataDir: dir,
		sampler: sampling.NewManager(hc, st, ms, nil, "Fwlib64.dll", errors.New("not found"))}

	get := func(path, accept string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		if accept != "" {
			r.Header.Set("Accept", accept)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, c := range []struct{ path, accept, typ, want string }{
		{"/", "*/*", "text/markdown", "## 페이지"},
		{"/help", "", "text/markdown", "| 1 | bench-31i | 허브 `nc_focas2_fanuc` | `/hmi?machine=1` (`/hmi`의 기본), `/sampling` |"},
		{"/help", "", "text/markdown", "`/hmi`가 기본으로 보여 줄 기계 (설정 `machine = \"auto\"`): 1 `bench-31i`"},
		{"/help", "", "text/markdown", "| 2 | mill | 허브 `nc_opcua_siemens` | 없음 (FANUC가 아님) |"},
		{"/help", "text/html,application/xhtml+xml", "text/html", "<table>"},
		{"/", "text/html", "text/html", `<a href="/hmi">`},
		{"/help.md", "text/html", "text/markdown", "FOCAS2 라이브러리: **없음**"},
	} {
		w := get(c.path, c.accept)
		body := w.Body.String()
		if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), c.typ) || !strings.Contains(body, c.want) {
			t.Errorf("%s (Accept %q): %d %s, want %s with %q", c.path, c.accept, w.Code, w.Header().Get("Content-Type"), c.typ, c.want)
		}
		if strings.Contains(body, "192.0.2.") || strings.Contains(body, statusLine) {
			t.Errorf("%s: a machine address or the status placeholder in the page", c.path)
		}
	}
	// the HMI's address of earlier builds
	if w := get("/?machine=2", "text/html"); w.Code != http.StatusFound || w.Header().Get("Location") != "/hmi?machine=2" {
		t.Errorf("/?machine=2: %d to %q", w.Code, w.Header().Get("Location"))
	}
}

func TestSPA(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":  {Data: []byte("<!doctype html><title>app</title>")},
		"assets/a.js": {Data: []byte("x")},
	}
	h := spa(dist)
	for path, want := range map[string]int{
		"/hmi": 200, "/hmi/": 200, "/sampling": 200, "/sampling/": 200, "/assets/a.js": 200,
		"/assets/": 404, "/index.html": 404, "/nothing": 404, "/api/nothing": 404,
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != want {
			t.Errorf("%s: %d, want %d", path, w.Code, want)
		}
		if want == 200 && path != "/assets/a.js" && !strings.Contains(w.Body.String(), "<title>app</title>") {
			t.Errorf("%s: not the app", path)
		}
	}
}
