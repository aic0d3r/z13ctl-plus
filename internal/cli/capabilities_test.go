package cli

import "testing"

func TestUnsupported(t *testing.T) {
	for _, cmd := range []string{"apply", "get-state", "profile", "cpu-boost", "no-such-command"} {
		if got := Unsupported(cmd); got != "" {
			t.Errorf("Unsupported(%q) = %q, want empty", cmd, got)
		}
	}
	if len(commandNeeds) != 12 {
		t.Errorf("expected 12 hardware-gated commands, got %d", len(commandNeeds))
	}
	for cmd, need := range commandNeeds {
		if (Unsupported(cmd) == "") != need.has() {
			t.Errorf("Unsupported(%q) disagrees with its probe", cmd)
		}
	}
}

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
