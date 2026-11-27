package cmd

import (
	"testing"

	"github.com/dahui/z13ctl/api"
	"github.com/spf13/cobra"
)

func TestToggleTouchFlipsCurrentPostureOnly(t *testing.T) {
	start := api.TabletSettings{DisableTouchscreenInDesktop: true, ScrollSpeed: 2}

	got, on, err := toggleTouch(start, "laptop")
	if err != nil || on || !got.DisableTouchscreenInLaptop || !got.DisableTouchscreenInDesktop || got.ScrollSpeed != 2 {
		t.Fatalf("laptop toggle = %+v, on=%v, err=%v", got, on, err)
	}
	if got, on, _ = toggleTouch(got, "laptop"); !on || got.DisableTouchscreenInLaptop {
		t.Fatalf("second laptop toggle = %+v, on=%v", got, on)
	}
	if got, on, _ = toggleTouch(start, "desktop"); !on || got.DisableTouchscreenInDesktop || got.DisableTouchscreenInLaptop {
		t.Fatalf("desktop toggle = %+v, on=%v", got, on)
	}
	if got, on, err = toggleTouch(start, "tablet"); err == nil || !on || got != start {
		t.Fatalf("tablet toggle = %+v, on=%v, err=%v", got, on, err)
	}
}

func TestTabletTwoFingerHoldContextMenuFlag(t *testing.T) {
	flag := tabletSettingsSetCmd.Flags().Lookup("two-finger-hold-context-menu")
	if flag == nil {
		t.Fatal("two-finger-hold-context-menu flag is missing")
	}
	if flag.DefValue != "false" {
		t.Fatalf("two-finger-hold-context-menu default = %q, want false", flag.DefValue)
	}
	if _, required := flag.Annotations[cobra.BashCompOneRequiredFlag]; !required {
		t.Fatal("two-finger-hold-context-menu flag is not required")
	}
}
