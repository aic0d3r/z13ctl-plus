package daemon

// protocol_test.go — guards the duplicated wire structs: every api.Send*
// call must decode into the daemon's request with no unknown fields, so a
// field added or renamed on one side only fails here instead of silently
// dropping commands. Also covers dispatch validation paths that reject bad
// input before any sysfs write.

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dahui/z13ctl/api"
)

func TestWireProtocolDecodesEveryAPICall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", dir)
	sock := api.SocketPath()
	if err := os.MkdirAll(filepath.Dir(sock), 0o750); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	commands := make(chan string, 32)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			line, readErr := bufio.NewReader(conn).ReadString('\n')
			var msg string
			if readErr != nil {
				msg = `{"ok":false,"error":"read: ` + readErr.Error() + `"}`
			} else {
				dec := json.NewDecoder(strings.NewReader(line))
				dec.DisallowUnknownFields()
				var req request
				if decErr := dec.Decode(&req); decErr != nil {
					msg = `{"ok":false,"error":"wire drift: ` + decErr.Error() + `"}`
				} else {
					msg = "{\"ok\":true}\n"
					commands <- req.Cmd
				}
			}
			_, _ = conn.Write([]byte(strings.TrimRight(msg, "\n") + "\n"))
			_ = conn.Close()
		}
	}()

	cases := []struct {
		cmd  string
		call func() error
	}{
		{"apply", func() error { _, err := api.SendApply("", "FF0000", "000000", "static", "normal", 3); return err }},
		{"off", func() error { _, err := api.SendOff(""); return err }},
		{"brightness", func() error { _, err := api.SendBrightness("", 2); return err }},
		{"profile", func() error { _, err := api.SendProfileSet("balanced"); return err }},
		{"batterylimit", func() error { _, err := api.SendBatteryLimitSet(80); return err }},
		{"bootsound", func() error { _, err := api.SendBootSoundSet(1); return err }},
		{"paneloverdrive", func() error { _, err := api.SendPanelOverdriveSet(1); return err }},
		{"camera", func() error { _, err := api.SendCameraSet(0); return err }},
		{"cpu-min", func() error { _, err := api.SendCPUMinFrequencySet(400000); return err }},
		{"cpu-epp", func() error { _, err := api.SendCPUEPPSet("balance_performance"); return err }},
		{"cpu-boost", func() error { _, err := api.SendCPUBoostSet(true); return err }},
		{"fancurve", func() error {
			_, err := api.SendFanCurveSet("40:20,50:30,60:40,70:50,75:60,80:80,85:120,90:200")
			return err
		}},
		{"tdp", func() error { _, err := api.SendTdpSet("60", "55", "65", "70", true); return err }},
		{"undervolt", func() error { _, err := api.SendUndervoltSet("-20"); return err }},
		{"preset-save", func() error { _, err := api.SendPresetSave("Test"); return err }},
		{"power-policy-set", func() error { _, err := api.SendPowerPolicySet(true, "AC", "Battery"); return err }},
		{"display-posture", func() error { _, err := api.SendDisplayPosture("tablet"); return err }},
		{"display-rotation", func() error { _, err := api.SendDisplayRotation("left"); return err }},
		{"tablet-settings-set", func() error {
			_, err := api.SendTabletSettingsSet(api.TabletSettings{ScrollSensitivity: 3, ScrollSpeed: 2})
			return err
		}},
		{"tablet-integration-report", func() error {
			_, err := api.SendTabletIntegrationReport(api.TabletIntegration{Posture: "tablet"})
			return err
		}},
	}
	for _, tc := range cases {
		if err := tc.call(); err != nil {
			t.Fatalf("%s: %v", tc.cmd, err)
		}
		select {
		case got := <-commands:
			if got != tc.cmd {
				t.Fatalf("daemon decoded cmd %q, want %q", got, tc.cmd)
			}
		default:
			t.Fatalf("%s: daemon did not report a decoded command", tc.cmd)
		}
	}
}

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
