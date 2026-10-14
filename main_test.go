package main

import (
	"testing"

	"deelook/internal/hub"
	"deelook/internal/screens"
)

func TestPickMachine(t *testing.T) {
	fanuc, _ := screens.Get("fanuc")
	ms := []hub.Machine{
		{ID: 5, Name: "bench-840d", Protocol: "nc_opcua_siemens"},
		{ID: 13, Name: "sim-ncguide-surface", Protocol: "nc_focas2_fanuc"},
		{ID: 1, Name: "lathe", Protocol: "nc_ezsocket_mitsubishi"},
		{ID: 2, Name: "bench-31i", Protocol: "nc_focas2_fanuc"},
		{ID: 20, Name: "auto", Protocol: "nc_focas2_fanuc"},
	}
	for machine, want := range map[string]int{
		"":     2, // the lowest-numbered FANUC, wherever it is on the list
		"auto": 20, // a name like any other
		"13":   13, "bench-31i": 2,
		"5": 0, "lathe": 0, "7": 0, // not FANUC, not on the list
	} {
		m, err := pickMachine(ms, fanuc, machine)
		if want == 0 && err == nil || want != 0 && (err != nil || m.ID != want) {
			t.Errorf("%q: machine %d, %v; want %d", machine, m.ID, err, want)
		}
	}
	if _, err := pickMachine(ms[:1], fanuc, ""); err == nil {
		t.Error("the first FANUC machine when there is none: no error")
	}
}
