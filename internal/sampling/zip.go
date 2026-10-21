package sampling

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// filterFrom reads a session filter from a query: machine, since, until
// (RFC 3339), status (comma separated) and before.
func filterFrom(q url.Values) (SessionFilter, error) {
	f := SessionFilter{Machine: q.Get("machine"), Since: q.Get("since"), Until: q.Get("until")}
	if st := q.Get("status"); st != "" {
		f.Status = strings.Split(st, ",")
	}
	if b := q.Get("before"); b != "" {
		n, err := strconv.ParseInt(b, 10, 64)
		if err != nil {
			return f, fmt.Errorf("bad before %q", b)
		}
		f.Before = n
	}
	return f, f.Check()
}

// maxZip bounds the sessions of one zip (the numbers go in the address).
const maxZip = 2000

// handleZip sends finished sessions' files in one zip, as they are (the
// names keep start time, machine and number). It streams: nothing is
// gathered first, however large the files.
func (m *Manager) handleZip(w http.ResponseWriter, r *http.Request) {
	var ids []int64
	for _, s := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			http.Error(w, "bad session id "+strconv.Quote(s), http.StatusBadRequest)
			return
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 || len(ids) > maxZip {
		http.Error(w, fmt.Sprintf("give 1 to %d session numbers: ?ids=1,2,3", maxZip), http.StatusBadRequest)
		return
	}
	var paths []string
	for _, id := range ids {
		path, err := m.store.File(id)
		if err != nil {
			http.Error(w, fmt.Sprintf("session %d: %v", id, err), http.StatusConflict)
			return
		}
		paths = append(paths, path)
	}
	name := fmt.Sprintf("deelook-sessions-%s.zip", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	zw := zip.NewWriter(w)
	for _, path := range paths {
		if err := addFile(zw, path); err != nil {
			// the answer is under way: stop it here, the zip ends broken
			log.Printf("session zip: %s: %v", path, err)
			return
		}
	}
	if err := zw.Close(); err != nil {
		log.Printf("session zip: %v", err)
	}
}

func addFile(zw *zip.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	h, err := zip.FileInfoHeader(fi)
	if err != nil {
		return err
	}
	h.Name, h.Method = filepath.Base(path), zip.Deflate
	dst, err := zw.CreateHeader(h)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, f)
	return err
}
