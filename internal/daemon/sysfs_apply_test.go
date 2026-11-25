package daemon

// sysfs_apply_test.go — profile/tuning/lighting apply paths against a fake
// sysfs tree. These redirect process-wide paths: no t.Parallel().

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aic0d3r/z13ctl-plus/internal/hid"
	"github.com/dahui/z13ctl/api"
)

// fakeProfile adds an ASUS platform-profile device set to profile under the
// fake tree made by fakeSysfs and returns its profile file.
func fakeProfile(t *testing.T, ppt, profile string) string {
	t.Helper()
	dev := filepath.Join(filepath.Dir(ppt), "platform-profile", "platform-profile-0")
	if err := os.MkdirAll(dev, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dev, "choices"), []byte("quiet balanced performance\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dev, "profile")
	if err := os.WriteFile(p, []byte(profile+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestApplyTuningReportsProfileAppliedWhenFanWriteFails(t *testing.T) {
	ppt, _ := fakeSysfs(t, false) // no fan hwmon: the fan write fails
	profile := fakeProfile(t, ppt, "balanced")
	state := api.State{Profile: "performance", TDPActive: true,
		TDP: &api.TDPState{PL1SPL: 80, PL2SPPT: 85, FPPT: 90}} // >75 W forces the fan safety curve

	err := applyTuning(state, true)
	if err == nil {
		t.Fatal("applyTuning succeeded without a fan device")
	}
	if !profileAppliedBeforeError(err) {
		t.Errorf("profileApplied = false, want true (err: %v)", err)
	}
	if got := readTrimmed(t, profile); got != "performance" {
		t.Errorf("profile = %q, want performance", got)
	}
	if _, statErr := os.Stat(filepath.Join(ppt, "ppt_pl1_spl")); !os.IsNotExist(statErr) {
		t.Error("PPT was written although the fan safety curve failed")
	}
}

func TestCheckTuningAdoptsExternalProfileAndClearsPreset(t *testing.T) {
	ppt, _ := fakeSysfs(t, false)
	fakeProfile(t, ppt, "performance") // changed by KDE / Fn key
	d := &Daemon{state: api.State{Profile: "balanced", ActivePreset: "Plugged In"}}

	if applied, err := d.checkTuning(false); applied || err != nil {
		t.Fatalf("checkTuning = (%v, %v), want (false, nil) with no overrides", applied, err)
	}
	if d.state.Profile != "performance" {
		t.Errorf("Profile = %q, want performance", d.state.Profile)
	}
	if d.state.ActivePreset != "" {
		t.Errorf("ActivePreset = %q, want cleared", d.state.ActivePreset)
	}
}

func TestBrightnessAllUpdatesPerDeviceOverrides(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	f, err := os.CreateTemp(t.TempDir(), "hidraw")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	d := &Daemon{
		dev: hid.NewTestDeviceNamed(f, f.Name(), "keyboard"),
		state: api.State{
			Lighting: api.LightingState{Mode: "static", Color: "FF0000", Brightness: 3, Enabled: true},
			Devices: map[string]api.LightingState{
				"lightbar": {Mode: "cycle", Brightness: 3, Enabled: true},
				"keyboard": {}, // pure-off entry: rebuilt from the base state
			},
		},
	}

	if resp := d.handleBrightness(request{Cmd: "brightness", Brightness: 1}); !resp.OK {
		t.Fatalf("handleBrightness: %s", resp.Error)
	}
	if lb := d.state.Devices["lightbar"]; lb.Mode != "cycle" || lb.Brightness != 1 || !lb.Enabled {
		t.Errorf("lightbar = %+v, want cycle at brightness 1", lb)
	}
	if kb := d.state.Devices["keyboard"]; kb.Mode != "static" || kb.Color != "FF0000" || kb.Brightness != 1 {
		t.Errorf("keyboard = %+v, want base state at brightness 1", kb)
	}
	if info, _ := f.Stat(); info.Size() == 0 {
		t.Error("no HID packets written")
	}
}
