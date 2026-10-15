package cli

import "testing"

func TestDetectCapabilitiesFromReads(t *testing.T) {
	cases := []struct {
		name    string
		display DisplaySummary
		battery bool
		wantDsp bool
	}{
		{"no panel, no battery", DisplaySummary{External: true}, false, false},
		{"panel on", DisplaySummary{Refresh: 180}, true, true},
		{"panel off (docked)", DisplaySummary{PanelOff: true}, true, true},
	}
	for _, c := range cases {
		got := DetectCapabilities(c.display, c.battery)
		if got.Display != c.wantDsp || got.Battery != c.battery {
			t.Errorf("%s: display=%v battery=%v, want %v/%v", c.name, got.Display, got.Battery, c.wantDsp, c.battery)
		}
	}
}
