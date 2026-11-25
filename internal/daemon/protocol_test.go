package daemon

// protocol_test.go — dispatch validation paths that reject bad input before
// any sysfs write.

import "testing"

// TestDispatchValidationRejectsBadInput exercises rejection paths that run
// before any sysfs write (opus rule: dispatch tests must not touch hardware).
func TestDispatchValidationRejectsBadInput(t *testing.T) {
	d := &Daemon{}
	for _, req := range []request{
		{Cmd: "tdp", Set: "999"},
		{Cmd: "tdp", Set: "60", PL2: "999"},
		{Cmd: "tdp", Set: "4"},
		{Cmd: "brightness", Brightness: 9},
		{Cmd: "fancurve", Set: "40:20,50:30"}, // wrong point count
		{Cmd: "fancurve", Set: "50:20,40:30,45:40,55:50,60:60,65:70,70:80,75:90"},
		{Cmd: "display-posture", Posture: "sideways"},
		{Cmd: "display-rotation", Rotation: "diagonal"},
		{Cmd: "profile", Set: "custom"},
		{Cmd: "nope"},
	} {
		if resp := d.dispatch(req); resp.OK {
			t.Fatalf("%+v: expected an error response, got OK", req)
		}
	}
}
