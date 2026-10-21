package sampling

import (
	"fmt"
	"strconv"
	"strings"

	"deelook/internal/focas"
)

// Config is what a session samples and how. Two families of calls:
//
//	basic     cnc_sdtsetchnl, cnc_sdtstartsmpl, cnc_sdtreadsmpl,
//	          cnc_sdtcancelsmpl / cnc_sdtendsmpl: servo and spindle data,
//	          started at once or when block N runs, until stopped or for a
//	          number of points
//	extended  cnc_sdtsetchnl2, cnc_sdtstartsmpl2, cnc_sdtreadsmpl2,
//	          cnc_sdtendsmpl2: also PMC signals and CNC information, and the
//	          CNC marks when block N runs or a PMC signal rises/falls; until
//	          stopped or for a number of points
type Config struct {
	Mode          string          `json:"mode"`          // "basic" or "extended"
	ServoPeriodUs int             `json:"servoPeriodUs"` // 1000 or 250; spindles are always 1000
	Points        int             `json:"points"`        // 0: until stopped; else points per channel
	Path          int             `json:"path"`          // CNC path of StartSeq / Trigger.Seq (1..)
	StartSeq      int             `json:"startSeq"`      // basic: start when this block runs (0: at once)
	Trigger       Trigger         `json:"trigger"`       // extended: what the CNC marks
	Channels      []ChannelConfig `json:"channels"`
}

// Trigger is what an extended session has the CNC mark.
type Trigger struct {
	Seq    int    `json:"seq"`    // block number (0: none)
	Signal string `json:"signal"` // PMC address like "Y0.4" ("": none)
	Unit   int    `json:"unit"`   // PMC path of Signal (default 1)
	Up     bool   `json:"up"`     // mark the rise of Signal
	Down   bool   `json:"down"`   // mark the fall of Signal
}

// ChannelConfig is one channel as the browser asks for it.
type ChannelConfig struct {
	Kind    string   `json:"kind"`              // servo, spindle; extended also pmcbit, pmcbyte, pmcword, cnc
	Axis    int      `json:"axis"`              // servo axis 1..32, spindle 1..8 (logical, all paths); cnc: path 1..15
	Type    int      `json:"type"`              // DataType.Type (cnc: 1 O number, 2 N number)
	Shift   int      `json:"shift"`             // right shift 0..15 (servo, spindle)
	Signals []string `json:"signals,omitempty"` // pmc*: addresses; pmcbit up to 16 ("Y0.4"), byte/word one ("R100")
	Unit    int      `json:"unit,omitempty"`    // pmc*: PMC path (default 1)
	Label   string   `json:"label"`             // shown in the browser and the CSV header
}

const maxChannels = 8

func (c *Config) extended() bool { return c.Mode == "extended" }

// Validate checks a config against the cnc_sdtsetchnl(2) rules that can be
// known without the machine (per-DSP and per-amplifier limits are left to
// the CNC, which answers EW_DATA).
func (c *Config) Validate() error {
	if c.Mode != "basic" && c.Mode != "extended" {
		return fmt.Errorf("mode must be basic or extended")
	}
	if c.ServoPeriodUs != 1000 && c.ServoPeriodUs != 250 {
		return fmt.Errorf("servo period must be 1000 or 250 µs")
	}
	if c.Points != 0 && c.Points < 8 {
		return fmt.Errorf("points: 8 or more (or 0 for until stopped)")
	}
	if c.Path < 0 || c.Path > 10 {
		return fmt.Errorf("path 1..10")
	}
	if c.StartSeq < 0 || c.Trigger.Seq < 0 {
		return fmt.Errorf("block numbers are positive")
	}
	if len(c.Channels) == 0 || len(c.Channels) > maxChannels {
		return fmt.Errorf("1 to %d channels", maxChannels)
	}
	servo, spindle, bits, words := false, false, 0, false
	seen := map[string]bool{}
	for i, ch := range c.Channels {
		if err := ch.validate(c.extended()); err != nil {
			return fmt.Errorf("channel %d: %w", i+1, err)
		}
		key := fmt.Sprint(ch.Kind, ch.Axis, ch.Type, ch.Signals)
		if seen[key] {
			return fmt.Errorf("channel %d: same data twice", i+1)
		}
		seen[key] = true
		servo = servo || ch.Kind == "servo"
		spindle = spindle || ch.Kind == "spindle"
		if ch.Kind == "pmcbit" {
			bits++
		}
		words = words || ch.Kind == "pmcbyte" || ch.Kind == "pmcword"
	}
	if bits > 1 {
		return fmt.Errorf("PMC bit signals go in one channel (up to 16 there)")
	}
	for _, ch := range c.Channels {
		if ch.Kind == "pmcbit" && words && len(ch.Signals) > 8 {
			return fmt.Errorf("with a PMC byte or word channel, the bit channel takes 8 signals at most")
		}
	}
	// cnc_sdtsetchnl: 0.25 ms servo with 1 ms spindle only reads until stopped
	if c.ServoPeriodUs == 250 && servo && spindle && c.Points != 0 {
		return fmt.Errorf("0.25 ms servo with a spindle channel needs 'until stopped' (the CNC refuses a number of points)")
	}
	if c.extended() {
		if c.Trigger.Signal != "" {
			sig, err := parsePmc(c.Trigger.Signal, c.Trigger.Unit, true)
			if err != nil {
				return fmt.Errorf("trigger signal: %w", err)
			}
			// cnc_sdtstartsmpl2 lists no T, C or Z for the trigger
			if k := focas.PmcKinds[sig.Adr]; strings.IndexByte("TCZ", k) >= 0 {
				return fmt.Errorf("trigger signal: %c addresses cannot be a trigger", k)
			}
			if !c.Trigger.Up && !c.Trigger.Down {
				return fmt.Errorf("trigger signal: choose rise, fall or both")
			}
		}
	}
	return nil
}

func (ch *ChannelConfig) validate(extended bool) error {
	switch ch.Kind {
	case "servo":
		if ch.Axis < 1 || ch.Axis > 32 {
			return fmt.Errorf("servo axis 1..32")
		}
	case "spindle":
		if ch.Axis < 1 || ch.Axis > 8 {
			return fmt.Errorf("spindle 1..8")
		}
	case "pmcbit", "pmcbyte", "pmcword", "cnc":
		if !extended {
			return fmt.Errorf("%s channels need extended sampling (cnc_sdtsetchnl2)", ch.Kind)
		}
	default:
		return fmt.Errorf("unknown kind %q", ch.Kind)
	}
	switch ch.Kind {
	case "servo", "spindle", "cnc":
		if _, ok := findType(ch.Kind, ch.Type); !ok {
			return fmt.Errorf("unknown %s data type %d", ch.Kind, ch.Type)
		}
	}
	if ch.Kind == "cnc" && (ch.Axis < 1 || ch.Axis > 15) {
		return fmt.Errorf("CNC information path 1..15")
	}
	if ch.Shift < 0 || ch.Shift > 15 {
		return fmt.Errorf("shift 0..15")
	}
	if strings.HasPrefix(ch.Kind, "pmc") {
		max := 1
		if ch.Kind == "pmcbit" {
			max = 16
		}
		if len(ch.Signals) == 0 || len(ch.Signals) > max {
			return fmt.Errorf("%s takes 1..%d PMC addresses", ch.Kind, max)
		}
		for _, s := range ch.Signals {
			if _, err := parsePmc(s, ch.Unit, ch.Kind == "pmcbit"); err != nil {
				return err
			}
		}
	}
	return nil
}

// parsePmc reads a PMC address: "Y0.4" for a bit, "R100" for a byte or
// word. unit is the PMC path (0 means 1).
func parsePmc(s string, unit int, bit bool) (focas.PmcSignal, error) {
	s = strings.ToUpper(strings.TrimSpace(s))
	if unit == 0 {
		unit = 1
	}
	if unit < 1 || (unit > 5 && unit != 9) {
		return focas.PmcSignal{}, fmt.Errorf("PMC path must be 1..5 or 9")
	}
	if len(s) < 2 {
		return focas.PmcSignal{}, fmt.Errorf("PMC address %q: like Y0.4 or R100", s)
	}
	k := strings.IndexByte(focas.PmcKinds, s[0])
	if k < 0 {
		return focas.PmcSignal{}, fmt.Errorf("PMC address %q: kind must be one of %s", s, focas.PmcKinds)
	}
	num, b, hasBit := strings.Cut(s[1:], ".")
	no, err := strconv.Atoi(num)
	if err != nil || no < 0 || no > 65535 {
		return focas.PmcSignal{}, fmt.Errorf("PMC address %q: bad number", s)
	}
	sig := focas.PmcSignal{Unit: int16(unit), Adr: int8(k), No: uint16(no)}
	if bit {
		if !hasBit {
			return sig, fmt.Errorf("PMC address %q: a bit signal needs .0-.7", s)
		}
		n, err := strconv.Atoi(b)
		if err != nil || n < 0 || n > 7 {
			return sig, fmt.Errorf("PMC address %q: bit 0..7", s)
		}
		sig.Bit = uint8(n)
	} else if hasBit {
		return sig, fmt.Errorf("PMC address %q: a byte/word signal has no bit", s)
	}
	return sig, nil
}

// PeriodUs is the sampling period of channel i.
func (c *Config) PeriodUs(i int) int {
	if c.Channels[i].Kind == "spindle" {
		return 1000
	}
	return c.ServoPeriodUs
}

// Signed reports whether channel i's 16-bit words are signed (servo and
// spindle data) or not (PMC signals, CNC information).
func (c *Config) Signed(i int) bool {
	k := c.Channels[i].Kind
	return k == "servo" || k == "spindle"
}

func (c *Config) path() int16 {
	if c.Path == 0 {
		return 1
	}
	return int16(c.Path)
}

func (c *Config) points() int32 {
	if c.Points == 0 {
		return focas.Unlimited
	}
	return int32(c.Points)
}

// endPad is how many points more than asked an extended sampling with a
// number of points asks the CNC for. When such a sampling ends, the CNC
// returns the last few points of the PMC and CNC information channels as
// 0 whatever the signals were (measured on a 30i-B and NC Guide 0i-F: 4
// PMC points, 8 N number points). The padding takes those: it is read
// but not kept.
const endPad = 16

// cncPoints is the datanum given to the CNC: the points asked for, padded
// for an extended sampling with a number of points (endPad).
func (c *Config) cncPoints() int32 {
	n := c.points()
	if n > 0 && c.extended() {
		n += endPad
	}
	return n
}

// keepPoints is the number of points a channel keeps: the points asked for
// when the CNC was asked for more (cncPoints), else 0 for all.
func (c *Config) keepPoints() int64 {
	if c.cncPoints() != c.points() {
		return int64(c.Points)
	}
	return 0
}

// keep is how many of n points just read a channel keeps, having kept
// have, when it keeps at most limit (0: all).
func keep(have int64, n int32, limit int64) int32 {
	if limit <= 0 {
		return n
	}
	return int32(max(0, min(int64(n), limit-have)))
}

// mark is a trigger read at point at (from the first sample) as kept when
// only limit points are (0: all): within them it stays; in the padding
// only the end (STSM) does, at the last point kept.
func mark(bits uint16, at, limit int64) (uint16, int64) {
	if limit > 0 && at >= limit {
		return bits & focas.TrgEnded, limit - 1
	}
	return bits, at
}

func (c *Config) periodCode() uint16 {
	if c.ServoPeriodUs == 250 {
		return 2 // 0.25 ms; spindle channels stay at 1 ms
	}
	return 0
}

// focasChannels turns a basic config into IDBSDTCHAN entries.
func (c *Config) focasChannels() []focas.Channel {
	out := make([]focas.Channel, len(c.Channels))
	for i, ch := range c.Channels {
		axis, period := int8(ch.Axis), c.periodCode()
		if ch.Kind == "spindle" {
			axis, period = -axis, 0
		}
		out[i] = focas.Channel{Type: int16(ch.Type), ChNo: int8(i + 1), Axis: axis, Shift: period<<8 | uint16(ch.Shift)}
	}
	return out
}

// focasChannels2 turns an extended config into IDBSDTCHAN2 entries, and
// order maps each entry (and the data cnc_sdtreadsmpl2 returns for it) to
// its channel in c.Channels.
//
// The PMC bit channel goes last whatever its place in the config: measured
// on NC Guide 0i-F and a 30i-B with Fwlib64 6.9 and 7.3, any channel after
// it comes back shifted by two words per 8-word block, and its words even
// leak into the trigger bits (a steady 1 read as "SEQ").
func (c *Config) focasChannels2() (out []focas.Channel2, order []int, err error) {
	for i, ch := range c.Channels {
		if ch.Kind != "pmcbit" {
			order = append(order, i)
		}
	}
	for i, ch := range c.Channels {
		if ch.Kind == "pmcbit" {
			order = append(order, i)
		}
	}
	out = make([]focas.Channel2, len(order))
	for j, i := range order {
		ch := c.Channels[i]
		e := focas.Channel2{ChNo: int8(j + 1), Type: int16(ch.Type), Shift: c.periodCode()<<8 | uint16(ch.Shift)}
		switch ch.Kind {
		case "servo":
			e.Axis = int8(ch.Axis)
		case "spindle":
			e.Axis, e.Shift = -int8(ch.Axis), uint16(ch.Shift)
		case "cnc":
			e.Axis, e.Shift = int8(focas.AxisCncInfo+ch.Axis), c.periodCode()<<8
		case "pmcbit", "pmcbyte", "pmcword":
			e.Type, e.Shift = 0, c.periodCode()<<8
			e.Axis = map[string]int8{"pmcbit": focas.AxisPmcBit, "pmcbyte": focas.AxisPmcByte, "pmcword": focas.AxisPmcWord}[ch.Kind]
			for k, s := range ch.Signals {
				sig, err := parsePmc(s, ch.Unit, ch.Kind == "pmcbit")
				if err != nil {
					return nil, nil, err
				}
				e.IO[k] = sig
			}
		}
		out[j] = e
	}
	return out, order, nil
}

// trigger turns the trigger into cnc_sdtstartsmpl2's condition and data.
func (c *Config) trigger() (uint16, focas.TrgData) {
	var cond uint16
	var trg focas.TrgData
	if c.Trigger.Seq > 0 {
		cond |= focas.TrgSeq
		trg.SeqNo = int32(c.Trigger.Seq)
	}
	if c.Trigger.Signal != "" {
		trg.Pmc, _ = parsePmc(c.Trigger.Signal, c.Trigger.Unit, true)
		if c.Trigger.Up {
			cond |= focas.TrgUp
		}
		if c.Trigger.Down {
			cond |= focas.TrgDown
		}
	}
	return cond, trg
}
