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
