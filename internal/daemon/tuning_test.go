package daemon

import (
	"slices"
	"testing"

	"github.com/aic0d3r/z13ctl-plus/internal/cli"
	"github.com/dahui/z13ctl/api"
)

func TestPlanTuningHighTDPSafetyIsTemporary(t *testing.T) {
	desired := &api.FanCurveState{Mode: 1, Points: []api.FanCurvePoint{{Temp: 40, PWM: 80}}}
	state := api.State{
		Profile:        "performance",
		FanCurve:       desired,
		FanCurveActive: true,
		TDP:            &api.TDPState{PL1SPL: cli.TDPMaxSafe + 1},
		TDPActive:      true,
	}

	high := planTuning(state)
	if !high.FanSafety || high.Fan == nil || high.Fan.Points[0].PWM != cli.HighTDPMinPWM {
		t.Fatalf("high-TDP plan = %+v", high)
	}
	if state.FanCurve.Points[0].PWM != 80 {
		t.Fatal("safety planning changed desired fan curve")
	}

	state.TDP.PL1SPL = cli.TDPMaxSafe
	safe := planTuning(state)
	if safe.FanSafety || safe.Fan == nil || safe.Fan.Points[0].PWM != 80 {
		t.Fatalf("safe plan did not restore desired fan: %+v", safe)
	}

	state.FanCurveActive = false
	auto := planTuning(state)
	if auto.Fan != nil {
		t.Fatalf("inactive fan override did not restore auto: %+v", auto)
	}
}

func TestPlanTuningIndependentOverrides(t *testing.T) {
	state := api.State{
		FanCurve:        &api.FanCurveState{Mode: 1},
		TDP:             &api.TDPState{PL1SPL: 50},
		TDPActive:       true,
		Undervolt:       &api.UndervoltState{CPUCO: -15},
		UndervoltActive: true,
	}
	plan := planTuning(state)
	if plan.Fan != nil || plan.TDP == nil || plan.Undervolt == nil {
		t.Fatalf("independent plan = %+v", plan)
	}
}

func TestPlanTuningQuietUsesFirmwareAuto(t *testing.T) {
	plan := planTuning(api.State{Profile: "quiet"})
	if plan.Fan != nil {
		t.Fatalf("quiet profile should use firmware auto fan control: %+v", plan.Fan)
	}

	custom := &api.FanCurveState{Mode: 1, Points: []api.FanCurvePoint{{Temp: 40, PWM: 10}}}
	plan = planTuning(api.State{Profile: "quiet", FanCurve: custom, FanCurveActive: true})
	if plan.Fan == nil || plan.Fan.Points[0] != custom.Points[0] {
		t.Errorf("custom curve did not override quiet default: %+v", plan.Fan)
	}
}

func TestPlanTuningHighTDPKeepsStrongUserCurve(t *testing.T) {
	strong := &api.FanCurveState{Mode: 1, Points: []api.FanCurvePoint{
		{Temp: 30, PWM: 255}, {Temp: 40, PWM: 255}, {Temp: 50, PWM: 255}, {Temp: 60, PWM: 255},
		{Temp: 65, PWM: 255}, {Temp: 70, PWM: 255}, {Temp: 75, PWM: 255}, {Temp: 80, PWM: 255},
	}}
	weak := &api.FanCurveState{Mode: 1, Points: []api.FanCurvePoint{
		{Temp: 30, PWM: 100}, {Temp: 40, PWM: 120}, {Temp: 50, PWM: 140}, {Temp: 60, PWM: 160},
		{Temp: 65, PWM: 180}, {Temp: 70, PWM: 200}, {Temp: 75, PWM: 220}, {Temp: 80, PWM: 240},
	}}
	state := api.State{
		TDP:            &api.TDPState{PL1SPL: cli.TDPMaxSafe + 1},
		TDPActive:      true,
		FanCurve:       strong,
		FanCurveActive: true,
	}

	if plan := planTuning(state); !slices.Equal(plan.Fan.Points, strong.Points) {
		t.Error("high TDP with an all-255 user curve: the user curve should stand")
	}

	state.FanCurve = weak
	if plan := planTuning(state); !slices.Equal(plan.Fan.Points, cli.HighTDPFanCurve()) {
		t.Error("high TDP with a sub-floor user curve: the safety curve should apply")
	}
}
