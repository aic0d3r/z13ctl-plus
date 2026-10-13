package cli

import (
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

func TestParseInternalDisplayRefresh(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		out  string
		want int
	}{
		{
			name: "eDP-1 at 180Hz",
			out:  "Output: 1 eDP-1 abc-123\n\tenabled\n\tconnected\n\tModes:  1:2560x1600@180.00*  2:2560x1600@60.00  3:1920x1200@180.00\n",
			want: 180,
		},
		{
			name: "eDP-1 at 60Hz",
			out:  "Output: 1 eDP-1 abc-123\n\tenabled\n\tconnected\n\tModes:  1:2560x1600@180.00  2:2560x1600@60.00*  3:1920x1200@180.00\n",
			want: 60,
		},
		{
			name: "no current mode marker",
			out:  "Output: 1 eDP-1 abc-123\n\tModes:  1:2560x1600@180.00  2:2560x1600@60.00\n",
			want: 0,
		},
		{
			name: "multiple outputs, picks eDP-1 only",
			out: "Output: 2 HDMI-1 def-456\n\tModes:  1:1920x1080@60.00*  2:1920x1080@144.00\n" +
				"Output: 1 eDP-1 abc-123\n\tModes:  1:2560x1600@180.00*  2:2560x1600@60.00\n",
			want: 180,
		},
		{
			name: "kscreen-doctor missing eDP-1",
			out:  "Output: 2 HDMI-1 def-456\n\tModes:  1:1920x1080@60.00*\n",
			want: 0,
		},
		{
			name: "empty output",
			out:  "",
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state, _ := parseInternalDisplay([]byte(tc.out))
			if state.Refresh != tc.want {
				t.Errorf("parseInternalDisplay().Refresh = %d, want %d", state.Refresh, tc.want)
			}
		})
	}
}

func TestParseInternalDisplay(t *testing.T) {
	t.Parallel()
	out := []byte("Output: 1 eDP-1 abc-123\n\tenabled\n\tModes: 1:2560x1600@180.00*\n\tScale: 1.5\n\tRotation: 2\n")
	state, ok := parseInternalDisplay(out)
	if !ok {
		t.Fatal("parseInternalDisplay() did not find eDP-1")
	}
	if !state.Enabled || state.Refresh != 180 || state.Scale != 1.5 || state.Rotation != "left" {
		t.Fatalf("parseInternalDisplay() = %+v", state)
	}
}

func TestParseInternalDisplayANSI(t *testing.T) {
	t.Parallel()
	out := []byte("\x1b[01;32mOutput: \x1b[0;0m1 eDP-1 abc-123\n\t\x1b[01;32menabled\x1b[0;0m\n\t\x1b[01;34mModes: \x1b[0;0m 1:\x1b[01;32m2560x1600@60.00*\x1b[0;0m\n\t\x1b[01;33mScale: \x1b[0;0m1.5\n\t\x1b[01;33mRotation: \x1b[0;0m1\n")
	state, ok := parseInternalDisplay(out)
	if !ok || !state.Enabled || state.Refresh != 60 || state.Scale != 1.5 || state.Rotation != "none" {
		t.Fatalf("parseInternalDisplay(ANSI) = %+v, %t", state, ok)
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
	const (
		panelOn  = "Output: 2 eDP-1 abc\n\tenabled\n\tconnected\n\tModes: 58:2560x1600@180.00*!  59:2560x1600@60.00\n"
		panelOff = "Output: 2 eDP-1 abc\n\tdisabled\n\tconnected\n\tModes: 58:2560x1600@180.00*!  59:2560x1600@60.00\n"
		extOn    = "Output: 1 DP-8 x\n\tenabled\n\tconnected\n\tModes: 1:3840x2160@60.00*!\n\tHDR: disabled\n"
		extOff   = "Output: 3 HDMI-A-1 y\n\tdisabled\n\tconnected\n\tModes: 1:1920x1080@60.00*\n"
	)
	cases := []struct {
		name string
		out  string
		want DisplaySummary
	}{
		{"built-in only", panelOn, DisplaySummary{Refresh: 180}},
		// Live docked output: eDP-1 keeps its 180* mode while disabled.
		{"docked, panel off", extOn + panelOff, DisplaySummary{PanelOff: true, External: true}},
		{"second screen", extOn + panelOn, DisplaySummary{Refresh: 180, External: true}},
		{"two externals, one disabled", extOff + extOn + panelOff, DisplaySummary{PanelOff: true, External: true}},
		{"external connected but disabled", extOff + panelOn, DisplaySummary{Refresh: 180}},
		{"no eDP-1 block", extOn, DisplaySummary{External: true}},
		{"empty read", "", DisplaySummary{}},
		{"ANSI", "\x1b[01;32mOutput: \x1b[0;0m1 DP-8 x\n\t\x1b[01;32menabled\x1b[0;0m\n" + panelOff, DisplaySummary{PanelOff: true, External: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := summarizeDisplays([]byte(tc.out)); got != tc.want {
				t.Fatalf("summarizeDisplays() = %+v, want %+v", got, tc.want)
			}
		})
	}
}
