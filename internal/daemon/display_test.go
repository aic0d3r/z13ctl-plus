package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSetDisplayPostureExtendsDockSettle(t *testing.T) {
	d := &Daemon{displayCh: make(chan struct{}, 1)}
	before := time.Now()
	if err := d.setDisplayPosture("desktop"); err != nil {
		t.Fatal(err)
	}
	first := d.display.notBefore
	if first.Before(before.Add(dockSettleInterval - time.Second)) {
		t.Fatalf("dock settle deadline = %v", first)
	}
	time.Sleep(time.Millisecond)
	if err := d.setDisplayPosture("desktop"); err != nil {
		t.Fatal(err)
	}
	if !d.display.notBefore.After(first) {
		t.Fatal("repeated posture did not extend dock settle interval")
	}
}

func TestDisplayIntentValidation(t *testing.T) {
	d := &Daemon{displayCh: make(chan struct{}, 1)}
	if err := d.setDisplayPosture("tent"); err == nil {
		t.Fatal("setDisplayPosture(tent) error = nil")
	}
	if err := d.setDisplayRotation("upsideways"); err == nil {
		t.Fatal("setDisplayRotation(upsideways) error = nil")
	}
	if err := d.setDisplayRefresh(120); err == nil {
		t.Fatal("setDisplayRefresh(120) error = nil")
	}
	if err := d.setDisplayRefresh(180); err != nil {
		t.Fatal(err)
	}
}

// fakeKScreen installs a kscreen-doctor stub reporting the scale in
// $Z13CTL_SCALE (panel state in $Z13CTL_PANEL, default enabled) and
// recording apply arguments in the returned file.
func fakeKScreen(t *testing.T, scale string) string {
	t.Helper()
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	script := "#!/bin/sh\nif [ \"$1\" = \"-o\" ]; then\n  printf 'Output: 1 eDP-1 abc\\n\\t%s\\n\\tModes: 1:2560x1600@180.00*\\n\\tScale: %s\\n\\tRotation: 2\\n' \"${Z13CTL_PANEL:-enabled}\" \"$Z13CTL_SCALE\"\n  exit 0\nfi\nprintf '%s\\n' \"$*\" > \"$Z13CTL_ARGS\"\n"
	if err := os.WriteFile(filepath.Join(dir, "kscreen-doctor"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("Z13CTL_ARGS", argsPath)
	t.Setenv("Z13CTL_SCALE", scale)
	return argsPath
}

// reconcileArgs runs one reconcile for posture and returns the kscreen-doctor
// apply arguments ("" when nothing was applied).
func reconcileArgs(t *testing.T, d *Daemon, argsPath, posture string) string {
	t.Helper()
	_ = os.Remove(argsPath)
	d.display.posture = posture
	if _, retry, err := d.reconcileDisplay(); err != nil || retry {
		t.Fatalf("reconcileDisplay(%s) retry %t, err %v", posture, retry, err)
	}
	args, err := os.ReadFile(argsPath)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(args))
}

func TestReconcileDisplayTabletScalePreservesRotation(t *testing.T) {
	argsPath := fakeKScreen(t, "1.5")
	d := &Daemon{scalePosture: "laptop"}
	if got, want := reconcileArgs(t, d, argsPath, "tablet"), "output.eDP-1.scale.1.75"; got != want {
		t.Fatalf("kscreen-doctor args = %q, want %q", got, want)
	}
}

func TestReconcileDisplayRespectsUserScale(t *testing.T) {
	argsPath := fakeKScreen(t, "1.25")
	d := &Daemon{}
	// Startup posture and the 30 s re-publish leave the user's scale alone.
	if got := reconcileArgs(t, d, argsPath, "tablet"); got != "" {
		t.Fatalf("startup reconcile applied %q", got)
	}
	if got := reconcileArgs(t, d, argsPath, "tablet"); got != "" {
		t.Fatalf("repeat reconcile applied %q", got)
	}
	// Leaving tablet remembers 1.25 there; laptop gets the default.
	if got, want := reconcileArgs(t, d, argsPath, "laptop"), "output.eDP-1.scale.1.5 output.eDP-1.rotation.none"; got != want {
		t.Fatalf("laptop args = %q, want %q", got, want)
	}
	// User picks 1.0 in laptop; returning to tablet restores 1.25.
	t.Setenv("Z13CTL_SCALE", "1")
	if got := reconcileArgs(t, d, argsPath, "laptop"); got != "output.eDP-1.rotation.none" {
		t.Fatalf("repeat laptop args = %q", got)
	}
	if got, want := reconcileArgs(t, d, argsPath, "tablet"), "output.eDP-1.scale.1.25"; got != want {
		t.Fatalf("tablet args = %q, want %q", got, want)
	}
	t.Setenv("Z13CTL_SCALE", "1.25")
	if got, want := reconcileArgs(t, d, argsPath, "laptop"), "output.eDP-1.scale.1 output.eDP-1.rotation.none"; got != want {
		t.Fatalf("laptop return args = %q, want %q", got, want)
	}
}

func TestReconcileDisplayKeepScaleNeverChangesScale(t *testing.T) {
	argsPath := fakeKScreen(t, "1.25")
	d := &Daemon{scalePosture: "laptop"}
	d.state.TabletSettings.KeepDisplayScale = true
	if got := reconcileArgs(t, d, argsPath, "tablet"); got != "" {
		t.Fatalf("keep-scale reconcile applied %q", got)
	}
}

func TestHandleRefreshRateRefusesWhilePanelOff(t *testing.T) {
	fakeKScreen(t, "1.5")
	t.Setenv("Z13CTL_PANEL", "disabled")
	d := &Daemon{displayCh: make(chan struct{}, 1)}
	resp := d.handleRefreshRate(request{Cmd: "refresh-rate", Set: "60"})
	if resp.OK || resp.Error != "the built-in panel is off" {
		t.Fatalf("handleRefreshRate() = %+v, want panel-off refusal", resp)
	}
	if d.display.refresh != 0 {
		t.Fatalf("refused request stored intent %d Hz", d.display.refresh)
	}
}

func TestPresetRefreshStillQueuesWhilePanelOff(t *testing.T) {
	fakeKScreen(t, "1.5")
	t.Setenv("Z13CTL_PANEL", "disabled")
	d := &Daemon{displayCh: make(chan struct{}, 1)}
	// Preset apply uses setDisplayRefresh directly: its rate applies once the panel returns.
	if err := d.setDisplayRefresh(60); err != nil || d.display.refresh != 60 {
		t.Fatalf("setDisplayRefresh(60) = %v, intent %d", err, d.display.refresh)
	}
}
