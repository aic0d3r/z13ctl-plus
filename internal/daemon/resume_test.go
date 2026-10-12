package daemon

import (
	"testing"

	"github.com/godbus/dbus/v5"
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
