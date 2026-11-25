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
}{
	"batterylimit": {"a battery charge limit", hasChargeLimit}, "batterylimit-get": {"a battery charge limit", hasChargeLimit},
	"bootsound": {"the ASUS boot sound attribute", hasBootSound}, "bootsound-get": {"the ASUS boot sound attribute", hasBootSound},
	"paneloverdrive": {"the ASUS panel overdrive attribute", hasOverdrive}, "paneloverdrive-get": {"the ASUS panel overdrive attribute", hasOverdrive},
	"camera": {"a built-in camera", hasCamera}, "camera-get": {"a built-in camera", hasCamera},
	"fancurve": {"ASUS custom fan curves", hasFanCurve}, "fancurve-get": {"ASUS custom fan curves", hasFanCurve}, "fancurve-reset": {"ASUS custom fan curves", hasFanCurve},
	"tdp": {"ASUS PPT power limits", hasTDP}, "tdp-get": {"ASUS PPT power limits", hasTDP}, "tdp-reset": {"ASUS PPT power limits", hasTDP},
}

func hasChargeLimit() bool { return pathExists(FindBatteryThresholdPath()) }
func hasBootSound() bool   { return pathExists(FindBootSoundPath()) }
func hasOverdrive() bool   { return pathExists(FindPanelOverdrivePath()) }
func hasCamera() bool      { return FindCameraPath() != "" }
func hasFanCurve() bool    { return FindFanCurveHwmonPath() != "" }
func hasTDP() bool         { return pathExists(pptBasePath + "/ppt_pl1_spl") }

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
		Camera:         FindCameraPath() != "",
		// A readable eDP-1 block (enabled or disabled) means an internal panel exists.
		Display: display.Refresh > 0 || display.PanelOff,
	}
}
