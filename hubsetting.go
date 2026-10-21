package main

// The deemesh-hub address can be changed while deelook runs (the address
// at the top right of every page): it applies at once and is written into
// deelook.toml, which keeps holding the values in use. An empty address
// runs deelook without deemesh: no HMI screens, sampling on the machines
// added by address.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"deelook/internal/hub"

	"github.com/BurntSushi/toml"
)

// normalizeHub checks a hub address the page sent: "" for none, else
// http(s)://host[:port] (http:// is added when there is no scheme).
func normalizeHub(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.User != nil {
		return "", errors.New("주소는 http://호스트:포트 꼴로 넣으세요 (예: http://127.0.0.1:8080)")
	}
	return u.Scheme + "://" + u.Host, nil
}

// setSetting writes key = value into the settings file at path and keeps
// the rest of it as it is, comments included: the line that sets key before
// any [table] is replaced, or a line is added before the first table.
func setSetting(path, key string, value any) error {
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var enc bytes.Buffer
	if err := toml.NewEncoder(&enc).Encode(map[string]any{key: value}); err != nil {
		return err
	}
	line := strings.TrimSpace(enc.String())
	lines := strings.Split(string(b), "\n")
	tables := len(lines)
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "[") {
			tables = i
			break
		}
	}
	sets := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(key) + `\s*=`)
	done := false
	for i := 0; i < tables && !done; i++ {
		if sets.MatchString(lines[i]) {
			cr := strings.HasSuffix(lines[i], "\r") // keep the file's line ends
			lines[i] = line
			if cr {
				lines[i] += "\r"
			}
			done = true
		}
	}
	if !done {
		lines = append(lines[:tables], append([]string{line}, lines[tables:]...)...)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".deelook-*.toml")
	if err != nil {
		return err
	}
	_, err = tmp.WriteString(strings.Join(lines, "\n"))
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

// hubInfo is what the page shows about the hub.
type hubInfo struct {
	URL      string `json:"url"`               // "" for none
	OK       bool   `json:"ok"`                // it answered
	Machines int    `json:"machines"`          // on its list
	Error    string `json:"error,omitempty"`   // why it did not
	Warning  string `json:"warning,omitempty"` // applied, but not written into deelook.toml
}

func hubState(ctx context.Context, hc *hub.Client) hubInfo {
	info := hubInfo{URL: hc.Base()}
	if info.URL == "" {
		return info
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ms, err := hc.Machines(ctx)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.OK, info.Machines = true, len(ms)
	return info
}

// hubRoutes registers GET /api/hub (the address and whether it answers)
// and PUT /api/hub {"url"} (change it; "" for none).
func hubRoutes(mux *http.ServeMux, hc *hub.Client, settingsPath string) {
	mux.HandleFunc("GET /api/hub", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, hubState(r.Context(), hc))
	})
	mux.HandleFunc("PUT /api/hub", func(w http.ResponseWriter, r *http.Request) {
		// JSON only: a form on another site cannot send it
		if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
			http.Error(w, "send Content-Type: application/json", http.StatusUnsupportedMediaType)
			return
		}
		var req struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
			http.Error(w, "bad request body: "+err.Error(), http.StatusBadRequest)
			return
		}
		u, err := normalizeHub(req.URL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		hc.SetBase(u)
		info := hubState(r.Context(), hc)
		if err := setSetting(settingsPath, "hub", u); err != nil {
			info.Warning = fmt.Sprintf("바로 적용했지만 %s에 쓰지 못해 다음에 시작할 때는 예전 주소입니다: %v", settingsName, err)
		}
		writeJSON(w, info)
	})
}
