package daemon

import (
	"encoding/json"
	"testing"

	"github.com/dahui/z13ctl/api"
)

// An active 5 W override must be reported as-is, not mistaken for the
// kernel's unset PPT cache and replaced by stock values.
func TestTDPGetReturnsActiveOverride(t *testing.T) {
	want := api.TDPState{PL1SPL: 5, PL2SPPT: 5, FPPT: 5, APUSPPT: 5, PlatformSPPT: 5}
	d := &Daemon{state: api.State{TDPActive: true, TDP: &want}}
	resp := d.handleTDPGet()
	var got api.TDPState
	if err := json.Unmarshal([]byte(resp.Value), &got); !resp.OK || err != nil || got != want {
		t.Fatalf("tdp-get = %+v (%v), want %+v", resp, err, want)
	}
}

func TestProfileProtocolRejectsCustom(t *testing.T) {
	d := &Daemon{}
	resp := d.dispatch(request{Cmd: "profile", Set: "custom"})
	if resp.OK || resp.Error == "" {
		t.Fatalf("custom profile response = %+v", resp)
	}
}

// Commands that take applyMu themselves (deeper in the call) and so must NOT be
// in commandNeedsApplyLock, or they would deadlock. Any other state-changing
// command belongs in that list; update both when adding one.
func TestApplyLockListMatchesInternallyLockingCommands(t *testing.T) {
	for _, cmd := range []string{"preset-apply", "preset-set", "preset-delete", "preset-rename",
		"restore-recommended", "power-policy-set", "get-state", "profile-get"} {
		if commandNeedsApplyLock(cmd) {
			t.Errorf("%s must not be in commandNeedsApplyLock", cmd)
		}
	}
	for _, cmd := range []string{"apply", "off", "brightness", "profile", "batterylimit", "bootsound",
		"paneloverdrive", "cpu-min", "cpu-epp", "cpu-boost", "refresh-rate", "fancurve",
		"fancurve-reset", "tdp", "tdp-reset", "undervolt", "undervolt-reset", "tuning-reset", "preset-save"} {
		if !commandNeedsApplyLock(cmd) {
			t.Errorf("%s must be in commandNeedsApplyLock", cmd)
		}
	}
}
