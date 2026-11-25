package api

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func captureRequest(t *testing.T, send func() (bool, error)) Request {
	t.Helper()
	runtimeDir := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	if err := os.MkdirAll(filepath.Dir(SocketPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", SocketPath())
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	requestCh := make(chan Request, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		var got Request
		_ = json.NewDecoder(conn).Decode(&got)
		requestCh <- got
		_, _ = conn.Write([]byte("{\"ok\":true}\n"))
	}()

	handled, err := send()
	if err != nil || !handled {
		t.Fatalf("send() = %t, %v", handled, err)
	}
	return <-requestCh
}

func assertCommandSent(t *testing.T, send func() (bool, error), want string) {
	t.Helper()
	if got := captureRequest(t, send); got.Cmd != want {
		t.Fatalf("command = %q, want %q", got.Cmd, want)
	}
}

func TestSubscribeFiltersEvents(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(SocketPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", SocketPath())
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		var req Request
		_ = json.NewDecoder(conn).Decode(&req) // consume the request before replying
		_, _ = conn.Write([]byte("{\"ok\":true}\n{\"ok\":false,\"event\":\"state-changed\"}\n{\"ok\":false,\"event\":\"gui-toggle\"}\n"))
	}()

	ch, cancel, err := Subscribe([]string{"gui-toggle"})
	if err != nil || ch == nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer cancel()
	var got []string
	for ev := range ch {
		got = append(got, ev)
	}
	if len(got) != 1 || got[0] != "gui-toggle" {
		t.Fatalf("events = %v, want [gui-toggle]", got)
	}
}

func TestSendRestoreRecommendedProtocol(t *testing.T) {
	assertCommandSent(t, SendRestoreRecommended, "restore-recommended")
}

func TestSendTuningResetProtocol(t *testing.T) {
	assertCommandSent(t, SendTuningReset, "tuning-reset")
}

func TestTabletProtocol(t *testing.T) {
	settings := TabletSettings{
		DisableTouchscreenInDesktop: true,
		DisableTouchscreenInLaptop:  true,
		TwoFingerHoldContextMenu:    true,
		TouchpadEnabled:             true,
		ScrollSensitivity:           4,
		ScrollSpeed:                 5,
	}
	got := captureRequest(t, func() (bool, error) { return SendTabletSettingsSet(settings) })
	if got.Cmd != "tablet-settings-set" || got.TabletSettings == nil || *got.TabletSettings != settings {
		t.Fatalf("settings request = %+v", got)
	}

	status := TabletIntegration{
		Posture:            "tablet",
		FolioPresent:       false,
		Healthy:            true,
		LastSeen:           1234,
		TouchscreenEnabled: true,
		TouchScrollEnabled: true,
	}
	got = captureRequest(t, func() (bool, error) { return SendTabletIntegrationReport(status) })
	if got.Cmd != "tablet-integration-report" || got.TabletIntegration == nil || *got.TabletIntegration != status {
		t.Fatalf("integration request = %+v", got)
	}

	data, err := json.Marshal(Response{OK: true, State: &State{TabletSettings: settings, TabletIntegration: &status}})
	if err != nil {
		t.Fatal(err)
	}
	var stateResponse Response
	if err := json.Unmarshal(data, &stateResponse); err != nil {
		t.Fatal(err)
	}
	if stateResponse.State == nil || stateResponse.State.TabletSettings != settings || stateResponse.State.TabletIntegration == nil || *stateResponse.State.TabletIntegration != status {
		t.Fatalf("state response = %+v", stateResponse.State)
	}
}

func TestStateProtocolIncludesTuningFlags(t *testing.T) {
	data, err := json.Marshal(State{
		SchemaVersion:   StateSchemaVersion,
		Profile:         "balanced",
		FanCurveActive:  true,
		TDPActive:       true,
		UndervoltActive: true,
		FanSafetyActive: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"schema_version", "fan_curve_active", "tdp_active", "undervolt_active", "fan_safety_active"} {
		if _, ok := value[field]; !ok {
			t.Fatalf("state JSON %s missing %q", data, field)
		}
	}
}

func TestSocketPath(t *testing.T) {
	tests := []struct {
		name    string
		runtime string
		want    string
	}{
		{name: "XDG runtime", runtime: "/run/user/1000", want: "/run/user/1000/z13ctl-plus/z13ctl-plus.sock"},
		{name: "fallback", want: "/tmp/z13ctl-plus/z13ctl-plus.sock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_RUNTIME_DIR", tt.runtime)
			if got := SocketPath(); got != tt.want {
				t.Fatalf("SocketPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSendCommandOnlyFallsBackWhenDaemonIsAbsent(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	if handled, err := SendOff(""); handled || err != nil {
		t.Fatalf("no socket: handled %t, err %v; want fallback", handled, err)
	}

	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := filepath.Dir(SocketPath())
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", SocketPath())
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o700) }()
	if handled, err := SendOff(""); !handled || err == nil {
		t.Fatalf("permission denied: handled %t, err %v; want an error, not a silent fallback", handled, err)
	}
}

func TestTabletSettingsPath(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	want := "/run/user/1000/z13ctl-plus/tablet-settings.json"
	if got := TabletSettingsPath(); got != want {
		t.Fatalf("TabletSettingsPath() = %q, want %q", got, want)
	}
}
