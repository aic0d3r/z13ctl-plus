package daemon

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/dahui/z13ctl/api"
)

func TestTabletValidation(t *testing.T) {
	valid := defaultTabletSettings()
	for _, settings := range []api.TabletSettings{
		{ScrollSensitivity: 0, ScrollSpeed: valid.ScrollSpeed},
		{ScrollSensitivity: 6, ScrollSpeed: valid.ScrollSpeed},
		{ScrollSensitivity: valid.ScrollSensitivity, ScrollSpeed: 0},
		{ScrollSensitivity: valid.ScrollSensitivity, ScrollSpeed: 6},
	} {
		if err := validateTabletSettings(settings); err == nil {
			t.Fatalf("validateTabletSettings(%+v) succeeded", settings)
		}
	}
	if err := validateTabletSettings(valid); err != nil {
		t.Fatalf("valid settings: %v", err)
	}
	if err := validateTabletIntegration(api.TabletIntegration{Posture: "tent"}); err == nil {
		t.Fatal("invalid posture succeeded")
	}
	for _, posture := range []string{"desktop", "laptop", "tablet"} {
		if err := validateTabletIntegration(api.TabletIntegration{Posture: posture}); err != nil {
			t.Fatalf("posture %q: %v", posture, err)
		}
	}
}

func TestTabletProtocolPersistsSettingsAndKeepsStatusLive(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	d := &Daemon{state: defaultState()}
	server, client := net.Pipe()
	defer func() { _ = server.Close() }()
	defer func() { _ = client.Close() }()
	d.addSubscriber(server)

	events := make(chan response, 2)
	go func() {
		decoder := json.NewDecoder(client)
		for i := 0; i < 2; i++ {
			var event response
			if err := decoder.Decode(&event); err != nil {
				return
			}
			events <- event
		}
	}()

	settings := api.TabletSettings{
		DisableTouchscreenInDesktop: false,
		DisableTouchscreenInLaptop:  true,
		TwoFingerHoldContextMenu:    true,
		TouchpadEnabled:             true,
		ScrollSensitivity:           5,
		ScrollSpeed:                 4,
	}
	resp := d.dispatch(request{Cmd: "tablet-settings-set", TabletSettings: &settings})
	if !resp.OK {
		t.Fatalf("settings response = %+v", resp)
	}
	select {
	case event := <-events:
		if event.Event != "state-changed" {
			t.Fatalf("settings event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("settings update was not broadcast")
	}

	status := api.TabletIntegration{Posture: "tablet", Healthy: true, LastSeen: 1234}
	resp = d.dispatch(request{Cmd: "tablet-integration-report", TabletIntegration: &status})
	if !resp.OK {
		t.Fatalf("report response = %+v", resp)
	}
	select {
	case event := <-events:
		if event.Event != "state-changed" {
			t.Fatalf("report event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("status update was not broadcast")
	}

	d.mu.Lock()
	if d.state.TabletSettings != settings || d.state.TabletIntegration == nil || *d.state.TabletIntegration != status {
		t.Fatalf("live tablet state = %+v", d.state)
	}
	d.mu.Unlock()
	persisted, _, err := loadState()
	if err != nil {
		t.Fatal(err)
	}
	if persisted.TabletSettings != settings || persisted.TabletIntegration != nil {
		t.Fatalf("persisted tablet state = settings %+v integration %+v", persisted.TabletSettings, persisted.TabletIntegration)
	}
}
