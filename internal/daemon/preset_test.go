package daemon

import (
	"testing"

	"github.com/dahui/z13ctl/api"
)

func TestValidatePreset(t *testing.T) {
	panel := 1
	valid := api.Preset{Profile: "quiet", RefreshRate: 60, PanelOverdrive: &panel}
	if err := validatePreset(valid); err != nil {
		t.Fatalf("validatePreset(valid) = %v", err)
	}

	invalid := valid
	invalid.RefreshRate = 120
	if err := validatePreset(invalid); err == nil {
		t.Fatal("validatePreset(120 Hz) error = nil")
	}

	if err := validatePreset(api.Preset{Profile: "custom"}); err == nil {
		t.Fatal("validatePreset(empty custom) error = nil")
	}
}

func TestClonePresetIsIndependent(t *testing.T) {
	original := api.Preset{
		Profile:   "balanced",
		FanCurve:  &api.FanCurveState{Mode: 1, Points: []api.FanCurvePoint{{Temp: 30, PWM: 50}}},
		Undervolt: &api.UndervoltState{CPUCO: -20},
	}
	cloned := clonePreset(original)
	cloned.FanCurve.Points[0].PWM = 100
	cloned.Undervolt.Active = true
	if original.FanCurve.Points[0].PWM != 50 || original.Undervolt.Active {
		t.Fatal("clonePreset shares mutable preset state")
	}
}

func TestRecommendedPresetsAreCompleteAndIndependent(t *testing.T) {
	presets := recommendedPresets(400000)
	for name, preset := range presets {
		if preset.Profile != "balanced" || preset.FanCurve != nil || preset.TDP != nil || preset.Undervolt != nil {
			t.Errorf("%s tuning snapshot = %+v", name, preset)
		}
	}
	state := defaultState()
	state.Battery = 95
	state.Presets = map[string]api.Preset{"Gaming": {Profile: "performance"}}
	restoreRecommendedPowerConfiguration(&state, 400000)
	if state.Battery != 95 || state.Presets["Gaming"].Profile != "performance" {
		t.Fatal("restoring recommendations changed unrelated state")
	}
	if policy := state.PowerPolicy; policy == nil || !policy.Enabled || policy.ACPreset != "Plugged In" || policy.BatteryPreset != "On Battery" {
		t.Fatalf("restored policy = %+v", policy)
	}
}

func TestValidatePresetName(t *testing.T) {
	if got, err := validatePresetName("  Gaming  "); err != nil || got != "Gaming" {
		t.Fatalf("validatePresetName() = %q, %v", got, err)
	}
	if _, err := validatePresetName("bad\nname"); err == nil {
		t.Fatal("validatePresetName(newline) error = nil")
	}
}

func TestSetDefaultPowerConfiguration(t *testing.T) {
	state := defaultState()
	setDefaultPowerConfiguration(&state, 625000)

	if state.Profile != "balanced" || state.Battery != 80 {
		t.Fatalf("baseline = profile %q, battery %d", state.Profile, state.Battery)
	}
	if len(state.Presets) != 2 {
		t.Fatalf("presets = %d, want 2", len(state.Presets))
	}

	tests := []struct {
		name    string
		refresh int
		panel   int
		epp     string
		boost   bool
	}{
		{"Plugged In", 180, 1, "balance_performance", true},
		{"On Battery", 60, 0, "balance_power", false},
	}
	for _, tt := range tests {
		preset, ok := state.Presets[tt.name]
		if !ok {
			t.Fatalf("missing preset %q", tt.name)
		}
		if preset.Profile != "balanced" || preset.RefreshRate != tt.refresh {
			t.Errorf("%s profile/refresh = %s/%d", tt.name, preset.Profile, preset.RefreshRate)
		}
		if preset.PanelOverdrive == nil || *preset.PanelOverdrive != tt.panel {
			t.Errorf("%s panel overdrive = %v, want %d", tt.name, preset.PanelOverdrive, tt.panel)
		}
		if preset.CPUPower == nil {
			t.Fatalf("%s CPU power is nil", tt.name)
		}
		if preset.CPUPower.MinFrequencyKHz != 625000 || preset.CPUPower.EPP != tt.epp || preset.CPUPower.Boost != tt.boost {
			t.Errorf("%s CPU power = %+v", tt.name, preset.CPUPower)
		}
		if preset.FanCurve != nil || preset.TDP != nil || preset.Undervolt != nil {
			t.Errorf("%s contains custom tuning", tt.name)
		}
	}

	policy := state.PowerPolicy
	if policy == nil || !policy.Enabled || policy.ACPreset != "Plugged In" || policy.BatteryPreset != "On Battery" {
		t.Fatalf("policy = %+v", policy)
	}
}

func TestRestoreOnDisable(t *testing.T) {
	snap := &api.Preset{Profile: "quiet"}
	tests := []struct {
		name                string
		wasEnabled, enabled bool
		active              string
		want                *api.Preset
	}{
		{"disable untouched preset", true, false, "70W", snap},
		{"disable after manual change", true, false, "", nil},
		{"already disabled", false, false, "70W", nil},
		{"stays enabled", true, true, "70W", nil},
	}
	for _, tt := range tests {
		if got := restoreOnDisable(tt.wasEnabled, tt.enabled, tt.active, snap); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestRenamePresetInState(t *testing.T) {
	state := api.State{
		Presets:      map[string]api.Preset{"70W": {Profile: "balanced"}, "Battery": {Profile: "quiet"}},
		PowerPolicy:  &api.PowerPolicy{Enabled: true, ACPreset: "70W", BatteryPreset: "Battery"},
		ActivePreset: "70W",
	}
	if err := renamePresetInState(&state, "70W", "Battery"); err == nil {
		t.Fatal("rename onto existing preset succeeded")
	}
	if err := renamePresetInState(&state, "70W", "Desk"); err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Presets["Desk"]; !ok || state.PowerPolicy.ACPreset != "Desk" || state.ActivePreset != "Desk" {
		t.Fatalf("rename not carried over: %+v %+v %q", state.Presets, state.PowerPolicy, state.ActivePreset)
	}
	if _, ok := state.Presets["70W"]; ok {
		t.Fatal("old name still present")
	}
}
