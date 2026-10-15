package cli

// capabilities.go — probe which hardware controls this machine has, so clients
// can hide the rest instead of showing controls that can only fail.

import (
	"os"

	"github.com/dahui/z13ctl/api"
)

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// commandNeeds lists the commands that only make sense with specific hardware.
var commandNeeds = map[string]struct {
	what string
	has  func() bool
}{}

func init() {
	battery := func() bool { return pathExists(FindBatteryThresholdPath()) }
	boot := func() bool { return pathExists(FindBootSoundPath()) }
	overdrive := func() bool { return pathExists(FindPanelOverdrivePath()) }
	fan := func() bool { return FindFanCurveHwmonPath() != "" }
	tdp := func() bool { return pathExists(pptBasePath + "/ppt_pl1_spl") }
	for cmd, need := range map[string]struct {
		what string
		has  func() bool
	}{
		"batterylimit": {"a battery charge limit", battery}, "batterylimit-get": {"a battery charge limit", battery},
		"bootsound": {"the ASUS boot sound attribute", boot}, "bootsound-get": {"the ASUS boot sound attribute", boot},
		"paneloverdrive": {"the ASUS panel overdrive attribute", overdrive}, "paneloverdrive-get": {"the ASUS panel overdrive attribute", overdrive},
		"fancurve": {"ASUS custom fan curves", fan}, "fancurve-get": {"ASUS custom fan curves", fan}, "fancurve-reset": {"ASUS custom fan curves", fan},
		"tdp": {"ASUS PPT power limits", tdp}, "tdp-get": {"ASUS PPT power limits", tdp}, "tdp-reset": {"ASUS PPT power limits", tdp},
	} {
		commandNeeds[cmd] = need
	}
}

// Unsupported names the missing hardware when cmd cannot run on this machine,
// or returns "" when it can (or needs no special hardware). It lets the daemon
// answer "not supported" instead of surfacing a raw "no such file" error.
func Unsupported(cmd string) string {
	if need, ok := commandNeeds[cmd]; ok && !need.has() {
		return need.what
	}
	return ""
}

// DetectCapabilities probes sysfs for each control. display and hasBattery come
// from reads get-state already performed, so nothing is read twice.
func DetectCapabilities(display DisplaySummary, hasBattery bool) *api.Capabilities {
	return &api.Capabilities{
		Profile:        pathExists(FindProfilePath()),
		FanCurve:       FindFanCurveHwmonPath() != "",
		TDP:            pathExists(pptBasePath + "/ppt_pl1_spl"),
		Battery:        hasBattery,
		BatteryLimit:   pathExists(FindBatteryThresholdPath()),
		BootSound:      pathExists(FindBootSoundPath()),
		PanelOverdrive: pathExists(FindPanelOverdrivePath()),
		// A readable eDP-1 block (enabled or disabled) means an internal panel exists.
		Display: display.Refresh > 0 || display.PanelOff,
	}
}
