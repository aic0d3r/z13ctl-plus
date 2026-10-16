package daemon

import (
	"testing"

	"github.com/dahui/z13ctl/api"
	"github.com/godbus/dbus/v5"

	"github.com/aic0d3r/z13ctl-plus/internal/cli"
)

func TestOnBatteryChanged(t *testing.T) {
	sig := func(props map[string]dbus.Variant) *dbus.Signal {
		return &dbus.Signal{Body: []any{"org.freedesktop.UPower", props, []string{}}}
	}
	if !onBatteryChanged(sig(map[string]dbus.Variant{"OnBattery": dbus.MakeVariant(true)})) {
		t.Fatal("OnBattery change not detected")
	}
	if onBatteryChanged(sig(map[string]dbus.Variant{"LidIsClosed": dbus.MakeVariant(true)})) {
		t.Fatal("lid change reported as power change")
	}
	if onBatteryChanged(&dbus.Signal{}) {
		t.Fatal("empty signal reported as power change")
	}
}

func TestResumePreset(t *testing.T) {
	policy := &api.PowerPolicy{Enabled: true, ACPreset: "AC", BatteryPreset: "Bat"}
	st := api.State{ActivePreset: "AC", PowerPolicy: policy}
	if got := resumePreset(st, cli.PowerSourceAC, cli.PowerSourceBattery); got != "Bat" {
		t.Fatalf("unplugged while asleep: got %q, want Bat", got)
	}
	if got := resumePreset(st, cli.PowerSourceAC, cli.PowerSourceAC); got != "AC" {
		t.Fatalf("same source: got %q, want AC", got)
	}
	st.ActivePreset = "Manual"
	if got := resumePreset(st, cli.PowerSourceAC, cli.PowerSourceAC); got != "Manual" {
		t.Fatalf("manual preset not kept: got %q", got)
	}
	st.PowerPolicy = &api.PowerPolicy{ACPreset: "AC", BatteryPreset: "Bat"}
	if got := resumePreset(st, cli.PowerSourceAC, cli.PowerSourceBattery); got != "Manual" {
		t.Fatalf("policy disabled: got %q, want Manual", got)
	}
}
