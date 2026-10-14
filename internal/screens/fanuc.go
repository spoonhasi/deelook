package screens

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"deelook/internal/hub"
)

// Fanuc is the Fanuc 0i/30i HMI. Its views:
//
//	pos      position values, modal, F/S, counters (POS screen and the
//	         left half of the split screens)
//	prog     pos + the program text around the executing block
//	offset   pos + 16 rows of the tool offset table (page 1, 2, ...)
//	work     pos + the work coordinate systems EXT, G54-G59
//	alarm    the alarm list
//	message  pos + the alarm list (operator messages)
//
// Every view also reads the header, status line and operator panel state.
var Fanuc = register(&Def{
	ID:        "fanuc",
	Protocols: []string{"nc_focas2_fanuc"},
	View:      fanucView,
	New:       newFanuc,
})

const (
	auxLetters      = "F,M,M2,M3,H,D,T,S" // order of the "aux" array
	offsetRows      = 16                  // tool offset rows per page
	programLines    = 12                  // program lines the PROG page shows
	workCoordinates = "EXT,G54,G55,G56,G57,G58,G59"
)

func fanucView(view, page string) (string, error) {
	switch view {
	case "", "pos":
		return "pos", nil
	case "prog", "work", "alarm", "message":
		return view, nil
	case "offset":
		n, err := strconv.Atoi(page)
		if page == "" {
			n, err = 1, nil
		}
		if err != nil || n < 1 || n > 1000 {
			return "", fmt.Errorf("bad offset page %q", page)
		}
		return "offset/" + strconv.Itoa(n), nil
	}
	return "", fmt.Errorf("unknown view %q", view)
}

type fanuc struct {
	c       *hub.Client
	machine string
	view    string
	page    int // offset page
	opt     Options

	axes    int // axis count the commands were built for; -1 = not read yet
	offsets int // tool offset count the offset page was built for
	keys    []string
	cmds    []string

	commentPath  string // programPath whose comment is cached
	comment      hub.Envelope
	commentRetry time.Time
}

func newFanuc(c *hub.Client, machine, view string, opt Options) PollFunc {
	p := &fanuc{c: c, machine: machine, view: view, opt: opt, axes: -1}
	if v, n, ok := strings.Cut(view, "/"); ok {
		p.view = v
		p.page, _ = strconv.Atoi(n)
	}
	return p.poll
}

func (p *fanuc) build(axes int) {
	ch := "?machine=" + p.machine + "&channel=1"
	ax := fmt.Sprintf("%s&axis=1-%d", ch, axes)
	reads := [][2]string{
		// header, status line, S/T line
		{"programName", "/machine/channel/programName" + ch},
		{"programPath", "/machine/channel/programPath" + ch},
		{"sequenceNumber", "/machine/channel/programSequenceNumber" + ch},
		{"mode", "/machine/channel/operateMode" + ch},
		{"run", "/machine/channel/executionStatus" + ch},
		{"motion", "/machine/channel/motionStatus" + ch},
		{"alarm", "/machine/channel/alarmStatus" + ch},
		{"emergency", "/machine/channel/emergencyStatus" + ch},
		{"clock", "/machine/currentDateTime?machine=" + p.machine},
		{"spindleSpeed", "/machine/channel/spindle/spindleSpeedActual" + ch + "&spindle=1"},
		{"tool", "/machine/channel/activeToolNumber" + ch},
		// operator panel
		{"feedOverride", "/machine/channel/feedOverride" + ch},
		{"spindleOverride", "/machine/channel/spindle/spindleOverride" + ch + "&spindle=1"},
		{"singleBlock", "/machine/channel/singleBlockOn" + ch},
		{"dryRun", "/machine/channel/dryRunOn" + ch},
		{"blockSkip", "/machine/channel/blockSkipOn" + ch},
		{"machineLock", "/machine/channel/machineLockOn" + ch},
	}
	if p.opt.Lamps == "ncguide" {
		// lamp outputs of the NC Guide standard operator panel ladder
		reads = append(reads, [2]string{"pmcY", "/machine/plcAddress/plcType/plcValueList?machine=" + p.machine + "&plcAddress=Y0~Y8&plcType=2"})
	}
	if p.view != "alarm" {
		reads = append(reads,
			[2]string{"axisCount", "/machine/channel/axisCount" + ch},
			[2]string{"gModal", "/machine/channel/gModalList" + ch},
			[2]string{"aux", "/machine/channel/auxModal/auxModalValue" + ch + "&auxModal=" + auxLetters},
			[2]string{"feed", "/machine/channel/feedActual" + ch},
			[2]string{"partsCount", "/machine/channel/partCountActual" + ch},
			[2]string{"runTime", "/machine/channel/operatingDuration" + ch},
			[2]string{"cycleTime", "/machine/channel/programRunDuration" + ch},
		)
		if axes > 0 {
			reads = append(reads,
				[2]string{"axisName", "/machine/channel/axis/axisName" + ax},
				[2]string{"absolute", "/machine/channel/axis/workPosition" + ax},
				[2]string{"relative", "/machine/channel/axis/relativePosition" + ax},
				[2]string{"machine", "/machine/channel/axis/machinePosition" + ax},
				[2]string{"distanceToGo", "/machine/channel/axis/distanceToGo" + ax},
			)
		}
	}
	switch p.view {
	case "prog":
		reads = append(reads,
			[2]string{"lookAhead", "/machine/channel/programLookAhead" + ch},
			[2]string{"blockCounter", "/machine/channel/programBlockCounter" + ch},
		)
	case "offset":
		reads = append(reads, [2]string{"offsetCount", "/machine/channel/toolOffsetCount" + ch})
		// a range past the last offset would fail as a whole, so stop at it
		first, last := (p.page-1)*offsetRows+1, min(p.page*offsetRows, p.offsets)
		if first <= last {
			rng := fmt.Sprintf("%s&toolOffset=%d-%d", ch, first, last)
			reads = append(reads,
				[2]string{"lengthGeometry", "/machine/channel/toolOffset/toolLengthGeometry" + rng},
				[2]string{"lengthWear", "/machine/channel/toolOffset/toolLengthWear" + rng},
				[2]string{"radiusGeometry", "/machine/channel/toolOffset/toolRadiusGeometry" + rng},
				[2]string{"radiusWear", "/machine/channel/toolOffset/toolRadiusWear" + rng},
			)
		}
	case "work":
		reads = append(reads, [2]string{"activeWork", "/machine/channel/activeWorkOffset" + ch})
		if axes > 0 {
			for _, w := range strings.Split(workCoordinates, ",") {
				reads = append(reads, [2]string{"work" + w, "/machine/channel/workOffset/axis/workOffsetValue" + ax + "&workOffset=" + w})
			}
		}
	case "alarm", "message":
		reads = append(reads, [2]string{"alarmList", "/machine/channel/alarmList" + ch})
	}

	p.axes = axes
	p.keys = p.keys[:0]
	p.cmds = p.cmds[:0]
	for _, r := range reads {
		p.keys = append(p.keys, r[0])
		p.cmds = append(p.cmds, r[1])
	}
}

func (p *fanuc) poll(ctx context.Context) (map[string]hub.Envelope, error) {
	if p.axes < 0 {
		// axisCount and toolOffsetCount are cached in the hub at connect, so
		// these cost no NC call.
		env, err := p.c.Read(ctx, "/machine/channel/axisCount?machine="+p.machine+"&channel=1")
		if err != nil {
			return nil, err
		}
		if p.view == "offset" {
			n, err := p.c.Read(ctx, "/machine/channel/toolOffsetCount?machine="+p.machine+"&channel=1")
			if err != nil {
				return nil, err
			}
			p.offsets = intValue(n)
		}
		p.build(intValue(env))
	}

	data, err := p.c.ReadBatch(ctx, p.cmds, p.opt.Timeout)
	if err != nil {
		return nil, err
	}
	vals := make(map[string]hub.Envelope, len(p.keys)+2)
	for i, k := range p.keys {
		vals[k] = data[i]
	}
	// a configuration changed (reconnect): the next poll reads the new one
	if env, ok := vals["offsetCount"]; ok && env.OK() && intValue(env) != p.offsets {
		p.offsets = intValue(env)
		p.build(p.axes)
	}
	if env, ok := vals["axisCount"]; ok && env.OK() && intValue(env) != p.axes {
		p.build(intValue(env))
	}
	if env, ok := vals["lookAhead"]; ok && env.OK() {
		vals["lookAhead"] = firstLines(env, programLines)
	}
	vals["programComment"] = p.programComment(ctx, vals["programPath"])
	if p.opt.Lamps != "" {
		vals["lampProfile"] = stringEnvelope(p.opt.Lamps)
	}
	return vals, nil
}

// programComment returns the comment of the program at path, read once per
// program from the NC memory listing (Fanuc keeps it on the O-number line).
func (p *fanuc) programComment(ctx context.Context, pathEnv hub.Envelope) hub.Envelope {
	var path string
	if !pathEnv.OK() || json.Unmarshal(pathEnv.Value, &path) != nil || path == "" {
		return stringEnvelope("")
	}
	if path == p.commentPath && (p.comment.OK() || time.Now().Before(p.commentRetry)) {
		return p.comment
	}
	p.commentPath = path
	env, err := p.c.Read(ctx, "/machine/ncMemoryPath/entry?machine="+p.machine+"&ncMemoryPath="+url.QueryEscape(path))
	if err != nil {
		env = hub.Envelope{Status: -10, Error: err.Error()}
	}
	if !env.OK() {
		p.comment = env
		p.commentRetry = time.Now().Add(10 * time.Second)
		return env
	}
	var entry struct {
		Comment *string `json:"comment"`
	}
	_ = json.Unmarshal(env.Value, &entry)
	c := ""
	if entry.Comment != nil {
		c = *entry.Comment
	}
	p.comment = stringEnvelope(c)
	return p.comment
}

// firstLines keeps the first n lines of a text value: the look-ahead buffer
// can hold a whole program, and the screen shows a dozen lines of it.
func firstLines(env hub.Envelope, n int) hub.Envelope {
	var s string
	if json.Unmarshal(env.Value, &s) != nil {
		return env
	}
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return stringEnvelope(strings.Join(lines, "\n"))
}

func stringEnvelope(s string) hub.Envelope {
	v, _ := json.Marshal(s)
	return hub.Envelope{Value: v}
}

func intValue(env hub.Envelope) int {
	var n int
	if env.OK() {
		_ = json.Unmarshal(env.Value, &n)
	}
	return n
}
