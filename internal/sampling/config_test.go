package sampling

import (
	"testing"

	"deelook/internal/focas"
)

// An extended sampling with a number of points asks the CNC for endPad
// more and keeps only the points asked for, marks included.
func TestEndPadding(t *testing.T) {
	for _, c := range []struct {
		cfg        Config
		cnc        int32
		keepPoints int64
	}{
		{Config{Mode: "extended", Points: 2000}, 2000 + endPad, 2000},
		{Config{Mode: "extended"}, focas.Unlimited, 0},
		{Config{Mode: "basic", Points: 2000}, 2000, 0},
		{Config{Mode: "basic"}, focas.Unlimited, 0},
	} {
		if got, keep := c.cfg.cncPoints(), c.cfg.keepPoints(); got != c.cnc || keep != c.keepPoints {
			t.Errorf("%s %d points: CNC %d, keeps %d; want %d, %d", c.cfg.Mode, c.cfg.Points, got, keep, c.cnc, c.keepPoints)
		}
	}

	// reads of 600 points with 2000 kept: the third read is cut, the fourth gone
	have := int64(0)
	for _, want := range []int32{600, 600, 600, 200, 0} {
		n := keep(have, 600, 2000)
		if n != want {
			t.Fatalf("after %d: keep %d of 600, want %d", have, n, want)
		}
		have += int64(n)
	}
	if keep(5000, 600, 0) != 600 {
		t.Error("no limit: not every point kept")
	}

	for _, c := range []struct {
		bits     uint16
		at       int64
		wantBits uint16
		wantAt   int64
	}{
		{focas.TrgUp, 1500, focas.TrgUp, 1500},                       // within: as it is
		{focas.TrgDown, 2005, 0, 1999},                               // in the padding: dropped
		{focas.TrgEnded, 2015, focas.TrgEnded, 1999},                 // the end: at the last point kept
		{focas.TrgEnded | focas.TrgDown, 2015, focas.TrgEnded, 1999}, // the end, without the padding's mark
	} {
		bits, at := mark(c.bits, c.at, 2000)
		if bits != c.wantBits || (bits != 0 && at != c.wantAt) {
			t.Errorf("mark 0x%04x at %d: 0x%04x at %d, want 0x%04x at %d", c.bits, c.at, bits, at, c.wantBits, c.wantAt)
		}
	}
	if bits, at := mark(focas.TrgDown, 5000, 0); bits != focas.TrgDown || at != 5000 {
		t.Error("no limit: mark moved")
	}
}
