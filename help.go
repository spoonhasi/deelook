package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"

	"deelook/internal/hub"
	"deelook/internal/sampling"
	"deelook/internal/screens"
)

// helpText is the manual at / and /help, for people and agents alike: what
// deelook is, its pages, settings, data and HTTP API. Browsers get it as a
// page; anything else gets the Markdown itself.
//
//go:embed help.md
var helpText string

// statusLine in help.md stands where the state of this deelook goes.
const statusLine = "_실행 중인 deelook의 `/help`에서는 여기에 허브 연결, FOCAS2 라이브러리, 기계, 수집, 디스크 상태가 나옵니다._"

// help serves the manual with the state of this deelook in it: the hub, the
// FOCAS2 library, the machines, sampling and the disk. Machine addresses
// stay out of it, as out of every page.
type help struct {
	hub        *hub.Client // its address can change while deelook runs (hubsetting.go)
	defMachine string      // the machine setting: what /hmi shows without ?machine=
	sampler    *sampling.Manager
	dataDir    string
}

var markdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)

func (h *help) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// earlier builds drew the HMI at /, so /?machine=2 goes on to it
	if r.URL.Path == "/" && r.URL.Query().Has("machine") {
		http.Redirect(w, r, "/hmi?"+r.URL.RawQuery, http.StatusFound)
		return
	}
	md := strings.Replace(helpText, statusLine, h.status(r.Context()), 1)
	w.Header().Set("Cache-Control", "no-cache") // the state changes
	w.Header().Set("Vary", "Accept")
	if r.URL.Path == "/help.md" || !strings.Contains(r.Header.Get("Accept"), "text/html") {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = io.WriteString(w, md)
		return
	}
	var body bytes.Buffer
	if err := markdown.Convert([]byte(md), &body); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	addr := strings.TrimPrefix(strings.TrimPrefix(h.hub.Base(), "http://"), "https://")
	if addr == "" {
		addr = "없음"
	}
	chip := fmt.Sprintf(`<span class="hub" title="바꾸기·지우기는 HMI 화면이나 시계열 수집 페이지의 오른쪽 위에서">deemesh-hub %s</span>`,
		html.EscapeString(addr))
	_, _ = io.WriteString(w, strings.Replace(helpHead, "<!--hub-->", chip, 1))
	_, _ = body.WriteTo(w)
	_, _ = io.WriteString(w, helpFoot)
}

// status is the state of this deelook as a Markdown list.
func (h *help) status(ctx context.Context) string {
	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, "- "+format+"\n", a...) }

	line("이 페이지를 만든 때: %s", time.Now().Format("2006-01-02 15:04:05 -07:00"))
	if v := buildInfo(); v != "" {
		line("deelook %s, 빌드: %s", version, v)
	} else {
		line("deelook %s", version)
	}

	hctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	ms, hubErr := h.hub.Machines(hctx)
	cancel()
	switch {
	case errors.Is(hubErr, hub.ErrNoHub):
		line("deemesh-hub: **쓰지 않음** (주소 없음) — HMI 화면은 나오지 않고, 시계열 수집은 직접 추가한 기계와 마지막으로 받은 허브 목록으로 됩니다. 주소는 페이지 오른쪽 위에서 넣습니다.")
	case hubErr != nil:
		line("deemesh-hub %s: **연결 안 됨** — HMI 화면이 나오지 않습니다. 시계열 수집은 마지막으로 받은 기계 목록으로 됩니다. (%s)",
			code(h.hub.Base()), code(hubErr.Error()))
	default:
		line("deemesh-hub %s: 연결됨, 기계 %d대", code(h.hub.Base()), len(ms))
	}
	hmi, _ := screens.Get("fanuc")
	shown := "" // the machine_id /hmi shows without ?machine=
	if hubErr == nil {
		setting := fmt.Sprintf("설정 `machine = %q`", h.defMachine)
		if h.defMachine == "" {
			setting += ": 번호가 가장 작은 FANUC 기계"
		}
		if m, err := pickMachine(ms, hmi, h.defMachine); err != nil {
			line("`/hmi`가 기본으로 보여 줄 기계 (%s): **없음** (%s)", setting, code(err.Error()))
		} else {
			shown = strconv.Itoa(m.ID)
			line("`/hmi`가 기본으로 보여 줄 기계 (%s): %s %s", setting, shown, code(m.Name))
		}
	}

	s := h.sampler.Summary(ms, hubErr)
	switch {
	case !s.Library:
		line("FOCAS2 라이브러리: **없음** — 시계열 수집을 할 수 없고 HMI 화면만 됩니다. (%s)", code(s.LibError))
	case s.Extended:
		line("FOCAS2 라이브러리: %s, 버전 %s, 기본·확장 방식 모두 됨", code(s.LibPath), code(s.LibVersion))
	default:
		line("FOCAS2 라이브러리: %s, 버전 %s, 기본 방식만 됨 (`cnc_sdt*2` 없음)", code(s.LibPath), code(s.LibVersion))
	}
	if len(s.Running) == 0 {
		line("수집 중: 없음")
	}
	for _, st := range s.Running {
		name := st.Machine
		if st.Name != "" {
			name += " (" + st.Name + ")"
		}
		line("수집 중: 기계 %s, 세션 %d, %s", code(name), st.Session, st.State)
	}
	st := s.Storage
	disk := "디스크 남은 공간을 알 수 없음"
	if st.Total > 0 {
		disk = "디스크 남은 공간 " + sampling.FormatSize(st.Free)
		if st.MinFree > 0 {
			disk += ", 이보다 적으면 수집 중단 " + sampling.FormatSize(st.MinFree)
			if st.Free < st.MinFree {
				disk += " — **수집할 수 없음**"
			}
		}
	}
	line("데이터 폴더: %s — 세션 파일 %d개 (합계 %s), %s", code(h.dataDir), st.Sessions, sampling.FormatSize(uint64(max(st.Used, 0))), disk)

	b.WriteString("\n**기계** — 번호를 주소의 `machine=`에 씁니다. HMI 화면은 FANUC(`nc_focas2_fanuc`) 기계만 그리고, " +
		"축은 수집 채널의 축 번호(1부터) 순입니다.\n\n")
	b.WriteString("| 번호 | 이름 | 어디서 | 열 수 있는 페이지 | 축 |\n|---|---|---|---|---|\n")
	sampled := map[string]sampling.SummaryMachine{}
	for _, mc := range s.Machines {
		sampled[mc.ID] = mc
	}
	row := func(id, name, from, pages string, mc sampling.SummaryMachine) {
		axes := strings.Join(mc.Axes, " ")
		if len(mc.Spindles) > 0 {
			axes += " · 주축 " + strings.Join(mc.Spindles, " ")
		}
		if mc.ID != "" && axes == "" {
			axes = "아직 모름 (시계열 수집 페이지가 읽음)"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", cell(id), cell(name), from, pages, cell(axes))
	}
	slices.SortFunc(ms, func(a, b hub.Machine) int { return a.ID - b.ID })
	for _, m := range ms { // the hub's list as read just now
		id := strconv.Itoa(m.ID)
		var pages []string
		if hmi != nil && hmi.Accepts(m.Protocol) {
			pages = append(pages, "`/hmi?machine="+id+"`")
			if id == shown {
				pages[0] += " (`/hmi`의 기본)"
			}
		}
		if mc, ok := sampled[id]; ok && mc.Source == "hub" {
			pages = append(pages, "`/sampling`")
		}
		if len(pages) == 0 {
			pages = append(pages, "없음 (FANUC가 아님)")
		}
		row(id, m.Name, "허브 "+code(m.Protocol), strings.Join(pages, ", "), sampled[id])
	}
	for _, mc := range s.Machines {
		switch {
		case mc.Source == "local":
			row(mc.ID, mc.Name, "deelook에 직접 추가", "`/sampling`", mc)
		case hubErr != nil: // the hub's, as last received
			row(mc.ID, mc.Name, "허브 (마지막으로 받은 목록)", "`/sampling`", mc)
		}
	}
	if len(ms) == 0 && len(s.Machines) == 0 {
		b.WriteString("| | (없음) | | | |\n")
	}
	return b.String()
}

// cell is s for a Markdown table cell.
func cell(s string) string {
	return strings.ReplaceAll(strings.Join(strings.Fields(s), " "), "|", `\|`)
}

// code is s as a Markdown code span, safe whatever s holds.
func code(s string) string {
	s = strings.Join(strings.Fields(strings.ReplaceAll(s, "`", "'")), " ")
	if s == "" {
		return "(없음)"
	}
	return "`" + s + "`"
}

// buildInfo is the commit deelook was built from, when the build knew it.
func buildInfo() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var rev, at, dirty string
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			at = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				dirty = ", 커밋 뒤 고친 소스"
			}
		}
	}
	if rev == "" {
		return ""
	}
	if len(rev) > 7 {
		rev = rev[:7]
	}
	if t, err := time.Parse(time.RFC3339, at); err == nil {
		at = ", " + t.Local().Format("2006-01-02") + " 커밋"
	}
	return "`" + rev + "`" + at + dirty + ", " + bi.GoVersion
}

// helpHead and helpFoot wrap the manual into a page in the app's colours,
// with the app's tabs.
const helpHead = `<!doctype html>
<html lang="ko">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>deelook 도움말</title>
<link rel="alternate" type="text/markdown" href="/help.md">
<script>
  // addresses of earlier builds: /#/ was the HMI, /#/sampling the sampling
  function earlier() {
    if (location.hash.startsWith('#/')) {
      location.replace((location.hash === '#/' ? '/hmi' : location.hash.slice(1)) + location.search)
    }
  }
  earlier()
  addEventListener('hashchange', earlier)
</script>
<style>
  html, body { margin: 0; background: #1e1e1e; color: #c8c8c8; font-family: system-ui, sans-serif; }
  .tabs { display: flex; gap: 2px; padding: 6px 8px 0; border-bottom: 1px solid #333; }
  .tabs a { padding: 6px 14px; color: #c8c8c8; text-decoration: none; border: 1px solid transparent;
    border-bottom: none; border-radius: 6px 6px 0 0; font-size: 14px; }
  .tabs a.on { color: #fff; background: #252525; border-color: #333; }
  .tabs .hub { margin-left: auto; align-self: center; margin-bottom: 4px; padding: 3px 10px; border: 1px solid #444;
    border-radius: 12px; font-size: 12px; color: #9a9a9a; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 50vw; }
  .help { max-width: 920px; margin: 0 auto; padding: 8px 16px 48px; line-height: 1.6; font-size: 15px; }
  .help h1 { color: #fff; font-size: 26px; margin: 16px 0 8px; }
  .help h2 { color: #fff; font-size: 19px; margin: 32px 0 8px; padding-bottom: 4px; border-bottom: 1px solid #333; }
  .help a { color: #8ab4f8; }
  .help strong { color: #e8e8e8; }
  .help code { font-family: ui-monospace, Consolas, monospace; font-size: 0.9em; background: #2a2a2a;
    padding: 1px 4px; border-radius: 3px; overflow-wrap: anywhere; }
  .help pre { background: #252525; border: 1px solid #333; border-radius: 6px; padding: 10px 12px; overflow-x: auto; }
  .help pre code { background: none; padding: 0; overflow-wrap: normal; }
  .help blockquote { margin: 12px 0; padding: 4px 14px; border-left: 3px solid #555; background: #232323; color: #b0b0b0; }
  .help table { display: block; overflow-x: auto; border-collapse: collapse; margin: 8px 0; font-size: 14px; }
  .help th, .help td { border: 1px solid #333; padding: 5px 10px; text-align: left; vertical-align: top; }
  .help th { background: #252525; color: #e8e8e8; }
  .help ul { padding-left: 22px; }
</style>
</head>
<body>
<nav class="tabs" aria-label="페이지">
  <a href="/help" class="on" aria-current="page">도움말</a>
  <a href="/hmi">HMI 화면</a>
  <a href="/sampling">시계열 수집</a>
  <!--hub-->
</nav>
<main class="help">
`

const helpFoot = `</main>
</body>
</html>
`
