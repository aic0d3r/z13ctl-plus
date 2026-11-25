package daemon

// state_test.go — Tests for state persistence: saveState, loadState, defaultState.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dahui/z13ctl/api"
)

func TestSaveAndLoadState_RoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	s := api.State{
		Lighting: api.LightingState{
			Enabled:    true,
			Mode:       "cycle",
			Color:      "FF0000",
			Color2:     "000000",
			Speed:      "normal",
			Brightness: 3,
		},
		Profile: "performance",
		Battery: 80,
	}
	if err := saveState(s); err != nil {
		t.Fatalf("saveState: %v", err)
	}
	got, fresh, err := loadState()
	if err != nil {
		t.Fatal(err)
	}
	if fresh {
		t.Fatal("saved state reported as fresh")
	}
	if got.Lighting != s.Lighting {
		t.Errorf("Lighting mismatch: got %+v, want %+v", got.Lighting, s.Lighting)
	}
	if got.Profile != s.Profile {
		t.Errorf("Profile: got %q, want %q", got.Profile, s.Profile)
	}
	if got.Battery != s.Battery {
		t.Errorf("Battery: got %d, want %d", got.Battery, s.Battery)
	}
	if got.Devices != nil {
		t.Errorf("Devices should be nil for all-device state, got %v", got.Devices)
	}
}

func TestTabletIntegrationIsNotPersisted(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	settings := api.TabletSettings{
		DisableTouchscreenInDesktop: false,
		DisableTouchscreenInLaptop:  true,
		TwoFingerHoldContextMenu:    true,
		TouchpadEnabled:             true,
		ScrollSensitivity:           5,
		ScrollSpeed:                 4,
	}
	status := &api.TabletIntegration{Posture: "tablet", Healthy: true, LastSeen: 1234}
	if err := saveState(api.State{
		SchemaVersion:     api.StateSchemaVersion,
		Profile:           "balanced",
		TabletSettings:    settings,
		TabletIntegration: status,
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "tablet_integration") {
		t.Fatalf("persisted live tablet status: %s", data)
	}
	got, _, err := loadState()
	if err != nil {
		t.Fatal(err)
	}
	if got.TabletSettings != settings || got.TabletIntegration != nil {
		t.Fatalf("loaded tablet state = settings %+v integration %+v", got.TabletSettings, got.TabletIntegration)
	}
}

func TestSaveAndLoadState_WithDevices(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	s := api.State{
		Lighting: api.LightingState{
			Enabled: true, Mode: "cycle", Color: "FF0000",
			Color2: "000000", Speed: "normal", Brightness: 3,
		},
		Devices: map[string]api.LightingState{
			"keyboard": {
				Enabled: true, Mode: "static", Color: "00FFFF",
				Color2: "000000", Speed: "normal", Brightness: 2,
			},
			"lightbar": {
				Enabled: true, Mode: "cycle", Color: "FF0000",
				Color2: "000000", Speed: "fast", Brightness: 3,
			},
		},
	}
	if err := saveState(s); err != nil {
		t.Fatalf("saveState: %v", err)
	}
	got, fresh, err := loadState()
	if err != nil {
		t.Fatal(err)
	}
	if fresh {
		t.Fatal("saved state reported as fresh")
	}

	if got.Lighting != s.Lighting {
		t.Errorf("Lighting mismatch: got %+v, want %+v", got.Lighting, s.Lighting)
	}
	if len(got.Devices) != 2 {
		t.Fatalf("Devices len: got %d, want 2", len(got.Devices))
	}
	if got.Devices["keyboard"] != s.Devices["keyboard"] {
		t.Errorf("keyboard mismatch: got %+v, want %+v", got.Devices["keyboard"], s.Devices["keyboard"])
	}
	if got.Devices["lightbar"] != s.Devices["lightbar"] {
		t.Errorf("lightbar mismatch: got %+v, want %+v", got.Devices["lightbar"], s.Devices["lightbar"])
	}
}

func TestLoadState_MissingFile(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "nonexistent"))

	got, fresh, err := loadState()
	if err != nil {
		t.Fatal(err)
	}
	if !fresh {
		t.Fatal("missing state file not reported as fresh")
	}
	def := defaultState()
	if got.Lighting != def.Lighting {
		t.Errorf("missing file: got Lighting %+v, want default %+v", got.Lighting, def.Lighting)
	}
}

func TestDefaultTabletSettings(t *testing.T) {
	want := api.TabletSettings{
		DisableTouchscreenInDesktop: true,
		TwoFingerHoldContextMenu:    false,
		TouchpadEnabled:             false,
		ScrollSensitivity:           3,
		ScrollSpeed:                 2,
	}
	if got := defaultState().TabletSettings; got != want {
		t.Fatalf("tablet defaults = %+v, want %+v", got, want)
	}
}

func TestProjectTabletSettings(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	settings := api.TabletSettings{
		DisableTouchscreenInDesktop: true,
		TwoFingerHoldContextMenu:    true,
		TouchpadEnabled:             true,
		ScrollSensitivity:           4,
		ScrollSpeed:                 5,
	}
	if err := projectTabletSettings(settings); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(api.TabletSettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("projection mode = %o, want 600", got)
	}
	data, err := os.ReadFile(api.TabletSettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	var got api.TabletSettings
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got != settings {
		t.Fatalf("projected settings = %+v, want %+v", got, settings)
	}
}

func TestLoadState_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)

	path := filepath.Join(dir, "z13ctl-plus", "state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, fresh, err := loadState(); err == nil || fresh {
		t.Fatalf("invalid state load = fresh %t, err %v", fresh, err)
	}

	// The daemon recovers: the bad file is kept aside and defaults load.
	if _, fresh, err := loadOrRecoverState(); err != nil || !fresh {
		t.Fatalf("recover = fresh %t, err %v", fresh, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("unusable state was not moved aside")
	}
	if backups, _ := filepath.Glob(path + ".unusable-*"); len(backups) != 1 {
		t.Errorf("backups = %v, want one", backups)
	}
}

func TestLoadState_ExistingEmptyStateIsNotFresh(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	path := filepath.Join(dir, "z13ctl-plus", "state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, fresh, err := loadState(); err != nil || fresh {
		t.Fatal("existing empty state reported as fresh")
	}
}

func TestLoadStateRejectsNewerSchema(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	path := filepath.Join(dir, "z13ctl-plus", "state.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"schema_version":999,"profile":"balanced"}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadState(); err == nil {
		t.Fatal("newer state schema was accepted")
	}
}

func TestMigrateLegacyCustomState(t *testing.T) {
	s := api.State{
		Profile:   "custom",
		FanCurve:  &api.FanCurveState{Mode: 1, Points: make([]api.FanCurvePoint, 8)},
		TDP:       &api.TDPState{PL1SPL: 80},
		Undervolt: &api.UndervoltState{CPUCO: -20},
		Presets: map[string]api.Preset{
			"Custom":   {Profile: "custom", BaseProfile: "quiet", TDP: &api.TDPState{PL1SPL: 60}},
			"Physical": {Profile: "performance", TDP: &api.TDPState{PL1SPL: 60}},
		},
	}
	if !migrateState(&s, "performance") {
		t.Fatal("legacy state was not migrated")
	}
	if s.SchemaVersion != api.StateSchemaVersion || s.Profile != "performance" {
		t.Fatalf("migrated identity = schema %d profile %q", s.SchemaVersion, s.Profile)
	}
	if !s.FanCurveActive || !s.TDPActive || !s.UndervoltActive || !s.Undervolt.Active {
		t.Fatalf("active flags = fan %t TDP %t UV %t/%t", s.FanCurveActive, s.TDPActive, s.UndervoltActive, s.Undervolt.Active)
	}
	if got := s.Presets["Custom"]; got.Profile != "quiet" || got.BaseProfile != "" || got.TDP == nil {
		t.Fatalf("custom preset migration = %+v", got)
	}
	if got := s.Presets["Physical"]; got.TDP != nil {
		t.Fatalf("physical preset retained dormant tuning: %+v", got)
	}
}

func TestMigrateSchemaOneAddsTabletDefaultsWithoutChangingTuning(t *testing.T) {
	fan := &api.FanCurveState{Mode: 1}
	tdp := &api.TDPState{PL1SPL: 60}
	s := api.State{
		SchemaVersion:  1,
		Profile:        "performance",
		FanCurve:       fan,
		TDP:            tdp,
		FanCurveActive: true,
		TDPActive:      true,
	}
	if !migrateState(&s, "balanced") {
		t.Fatal("schema one state was not migrated")
	}
	if s.FanCurve != fan || s.TDP != tdp || !s.FanCurveActive || !s.TDPActive {
		t.Fatalf("tablet migration changed tuning: %+v", s)
	}
	if s.TabletSettings != defaultTabletSettings() || s.SchemaVersion != api.StateSchemaVersion {
		t.Fatalf("tablet migration = schema %d settings %+v", s.SchemaVersion, s.TabletSettings)
	}
}

func TestMigrateLegacyPhysicalStateClearsDormantTuning(t *testing.T) {
	s := api.State{
		Profile:   "quiet",
		FanCurve:  &api.FanCurveState{Mode: 1},
		TDP:       &api.TDPState{PL1SPL: 60},
		Undervolt: &api.UndervoltState{CPUCO: -10, Active: true},
	}
	migrateState(&s, "performance")
	if s.Profile != "quiet" || s.FanCurve != nil || s.TDP != nil || s.Undervolt != nil {
		t.Fatalf("migrated physical state = %+v", s)
	}
}

func TestMigrateLegacyStateCopiesWithoutOverwrite(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	legacy := []byte(`{"schema_version":1,"profile":"balanced"}`)
	if err := os.MkdirAll(filepath.Dir(LegacyStatePath()), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(LegacyStatePath(), legacy, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := MigrateLegacyState(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(legacy) {
		t.Fatalf("migrated state = %q, want %q", got, legacy)
	}

	if writeErr := os.WriteFile(LegacyStatePath(), []byte(`{"profile":"performance"}`), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	if migrateErr := MigrateLegacyState(); migrateErr == nil {
		t.Fatal("MigrateLegacyState overwrote existing plus state")
	}
	got, err = os.ReadFile(StatePath())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(legacy) {
		t.Fatalf("state changed after refused migration: %q", got)
	}
}

func TestMigrateLegacyStateRejectsInvalidState(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "invalid JSON", data: []byte("not-json")},
		{name: "newer schema", data: []byte(`{"schema_version":999}`)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("XDG_STATE_HOME", t.TempDir())
			if err := os.MkdirAll(filepath.Dir(LegacyStatePath()), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(LegacyStatePath(), tt.data, 0o600); err != nil {
				t.Fatal(err)
			}

			if err := MigrateLegacyState(); err == nil {
				t.Fatal("MigrateLegacyState accepted invalid state")
			}
			if _, err := os.Stat(StatePath()); !os.IsNotExist(err) {
				t.Fatalf("plus state exists after invalid migration: %v", err)
			}
		})
	}
}
