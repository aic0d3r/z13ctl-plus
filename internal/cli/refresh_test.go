package cli

import (
	"fmt"
	"strings"
	"testing"
)

func TestSupportedRefreshRates(t *testing.T) {
	t.Parallel()
	got := SupportedRefreshRates()
	want := []int{60, 180}
	if len(got) != len(want) {
		t.Fatalf("SupportedRefreshRates() = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("SupportedRefreshRates()[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

// kscreenJSON builds `kscreen-doctor -j` output from output objects.
func kscreenJSON(outputs ...string) string {
	return `{"outputs":[` + strings.Join(outputs, ",") + `],"screen":{}}`
}

// kscreenOut is one output whose modes are 52 = 180 Hz and 53 = 60 Hz.
func kscreenOut(name string, enabled bool, current string, scale float64, rotation int) string {
	return fmt.Sprintf(`{"name":%q,"enabled":%t,"connected":true,"currentModeId":%q,"scale":%g,"rotation":%d,`+
		`"modes":[{"id":"52","name":"2560x1600@180","refreshRate":179.99},{"id":"53","name":"2560x1600@60","refreshRate":60.00}]}`,
		name, enabled, current, scale, rotation)
}

func TestParseInternalDisplay(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		out  string
		want InternalDisplayState
		ok   bool
	}{
		{"180 Hz", kscreenJSON(kscreenOut("eDP-1", true, "52", 1.5, 2)), InternalDisplayState{true, 180, 1.5, "left"}, true},
		{"60 Hz", kscreenJSON(kscreenOut("eDP-1", true, "53", 1.75, 1)), InternalDisplayState{true, 60, 1.75, "none"}, true},
		{"no current mode", kscreenJSON(kscreenOut("eDP-1", true, "", 1, 8)), InternalDisplayState{true, 0, 1, "right"}, true},
		{"picks eDP-1 only", kscreenJSON(kscreenOut("HDMI-A-1", true, "53", 1, 1), kscreenOut("eDP-1", false, "52", 2, 4)),
			InternalDisplayState{false, 180, 2, "inverted"}, true},
		{"no eDP-1", kscreenJSON(kscreenOut("HDMI-A-1", true, "53", 1, 1)), InternalDisplayState{}, false},
		{"empty", "", InternalDisplayState{}, false},
		{"not JSON", "Output: 1 eDP-1 abc\n", InternalDisplayState{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseInternalDisplay([]byte(tc.out))
			if got != tc.want || ok != tc.ok {
				t.Errorf("parseInternalDisplay() = %+v, %t; want %+v, %t", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestApplyInternalDisplayUnsupported(t *testing.T) {
	t.Parallel()
	err := ApplyInternalDisplay(0, "", 144)
	if err == nil {
		t.Fatal("ApplyInternalDisplay(144 Hz) = nil, want error")
	}
}

func TestSummarizeDisplays(t *testing.T) {
	t.Parallel()
	var (
		panelOn  = kscreenOut("eDP-1", true, "52", 1.5, 1)
		panelOff = kscreenOut("eDP-1", false, "52", 1.5, 1) // keeps its mode while disabled
		extOn    = kscreenOut("DP-8", true, "53", 1, 1)
		extOff   = kscreenOut("HDMI-A-1", false, "53", 1, 1)
	)
	cases := []struct {
		name    string
		outputs []string
		want    DisplaySummary
	}{
		{"built-in only", []string{panelOn}, DisplaySummary{Refresh: 180}},
		{"docked, panel off", []string{extOn, panelOff}, DisplaySummary{PanelOff: true, External: true}},
		{"second screen", []string{extOn, panelOn}, DisplaySummary{Refresh: 180, External: true}},
		{"two externals, one disabled", []string{extOff, extOn, panelOff}, DisplaySummary{PanelOff: true, External: true}},
		{"external connected but disabled", []string{extOff, panelOn}, DisplaySummary{Refresh: 180}},
		{"no eDP-1", []string{extOn}, DisplaySummary{External: true}},
		{"no outputs", nil, DisplaySummary{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := summarizeDisplays([]byte(kscreenJSON(tc.outputs...))); got != tc.want {
				t.Fatalf("summarizeDisplays() = %+v, want %+v", got, tc.want)
			}
		})
	}
	if got := summarizeDisplays(nil); got != (DisplaySummary{}) {
		t.Errorf("failed read = %+v, want zero", got)
	}
}
