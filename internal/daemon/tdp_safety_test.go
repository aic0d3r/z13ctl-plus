package daemon

// tdp_safety_test.go — the high-sustained-TDP thermal guard: fans must be on the
// safety curve before PPT is raised, and restored if the PPT write fails.
// Runs against a fake sysfs tree; these tests redirect process-wide paths, so
// they must not use t.Parallel().

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aic0d3r/z13ctl-plus/internal/cli"
	"github.com/dahui/z13ctl/api"
)

// fakeSysfs returns the PPT dir and the fan hwmon dir (empty string when
// withFan is false) under a temp tree wired into the cli package.
func fakeSysfs(t *testing.T, withFan bool) (ppt, fan string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	ppt = filepath.Join(root, "ppt")
	hwmon := filepath.Join(root, "hwmon")
	for _, dir := range []string{ppt, hwmon} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if withFan {
		fan = filepath.Join(hwmon, "hwmon0")
		if err := os.MkdirAll(fan, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(fan, "name"), []byte("asus_custom_fan_curve\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(cli.UseSysfsRoots(ppt, hwmon))
	return ppt, fan
}

func readTrimmed(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

func TestHighTDPSetsSafetyFansBeforePPT(t *testing.T) {
	ppt, fan := fakeSysfs(t, true)
	d := &Daemon{state: api.State{Profile: "performance"}}

	if resp := d.handleTDP(request{Cmd: "tdp", Set: "80", Force: true}); !resp.OK {
		t.Fatalf("handleTDP failed: %s", resp.Error)
	}
	if got := readTrimmed(t, filepath.Join(fan, "pwm1_enable")); got != "1" {
		t.Errorf("pwm1_enable = %q, want 1 (custom safety curve)", got)
	}
	if got := readTrimmed(t, filepath.Join(fan, "pwm1_auto_point8_pwm")); got != "255" {
		t.Errorf("last curve point pwm = %q, want 255", got)
	}
	if got := readTrimmed(t, filepath.Join(ppt, "ppt_pl1_spl")); got != "80" {
		t.Errorf("ppt_pl1_spl = %q, want 80", got)
	}
}

func TestHighTDPRefusedWhenFansCannotBeSet(t *testing.T) {
	ppt, _ := fakeSysfs(t, false) // no fan hwmon device
	d := &Daemon{state: api.State{Profile: "performance"}}

	if resp := d.handleTDP(request{Cmd: "tdp", Set: "80", Force: true}); resp.OK {
		t.Fatal("handleTDP succeeded without being able to set the safety fan curve")
	}
	if _, err := os.Stat(filepath.Join(ppt, "ppt_pl1_spl")); !os.IsNotExist(err) {
		t.Fatalf("ppt_pl1_spl was written before the fans were safe (stat err: %v)", err)
	}
}

func TestHighTDPRollsFansBackWhenPPTWriteFails(t *testing.T) {
	ppt, fan := fakeSysfs(t, true)
	if err := os.Mkdir(filepath.Join(ppt, "ppt_fppt"), 0o755); err != nil { // writing a directory fails
		t.Fatal(err)
	}
	// An active safe override keeps the rollback off the real platform_profile.
	d := &Daemon{state: api.State{
		Profile:   "performance",
		TDPActive: true,
		TDP:       &api.TDPState{PL1SPL: 50, PL2SPPT: 50, FPPT: 50, APUSPPT: 50, PlatformSPPT: 50},
	}}

	if resp := d.handleTDP(request{Cmd: "tdp", Set: "80", Force: true}); resp.OK {
		t.Fatal("handleTDP succeeded although the PPT write failed")
	}
	if got := readTrimmed(t, filepath.Join(fan, "pwm1_enable")); got != "3" {
		t.Errorf("pwm1_enable = %q after failed TDP set, want 3 (firmware auto restored)", got)
	}
}
