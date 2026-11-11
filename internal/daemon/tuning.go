package daemon

import (
	"errors"
	"fmt"

	"github.com/aic0d3r/z13ctl-plus/internal/cli"
	"github.com/dahui/z13ctl/api"
)

type tuningApplyError struct {
	err            error
	profileApplied bool
}

func (e *tuningApplyError) Error() string { return e.err.Error() }
func (e *tuningApplyError) Unwrap() error { return e.err }

func profileAppliedBeforeError(err error) bool {
	var applyErr *tuningApplyError
	return errors.As(err, &applyErr) && applyErr.profileApplied
}

type tuningPlan struct {
	Fan       *api.FanCurveState
	TDP       *api.TDPState
	Undervolt *api.UndervoltState
	FanSafety bool
}

// curveMeetsHighTDPFloor reports whether every point of the curve is at or
// above cli.HighTDPMinPWM, so it never drops below the safety floor (it can still
// be flatter than the safety curve's ramp).
func curveMeetsHighTDPFloor(points []api.FanCurvePoint) bool {
	for _, p := range points {
		if p.PWM < cli.HighTDPMinPWM {
			return false
		}
	}
	return len(points) > 0
}

func planTuning(state api.State) tuningPlan {
	plan := tuningPlan{}
	if state.TDPActive && state.TDP != nil {
		plan.TDP = clonePtr(state.TDP)
		plan.FanSafety = state.TDP.PL1SPL > cli.TDPMaxSafe
	}
	userCurveMeetsFloor := state.FanCurveActive && state.FanCurve != nil && curveMeetsHighTDPFloor(state.FanCurve.Points)
	if plan.FanSafety && !userCurveMeetsFloor {
		// High sustained TDP: enforce the safety floor. A user curve whose
		// every PWM is already at or above the floor is stronger and stands.
		plan.Fan = &api.FanCurveState{Mode: 1, Points: cli.HighTDPFanCurve()}
	} else if state.FanCurveActive && state.FanCurve != nil {
		plan.Fan = cloneFanCurve(state.FanCurve)
	}
	if state.UndervoltActive && state.Undervolt != nil {
		plan.Undervolt = clonePtr(state.Undervolt)
	}
	return plan
}

func applyTuning(state api.State, setProfile bool) error {
	plan := planTuning(state)
	if setProfile {
		if err := cli.SetProfile(state.Profile); err != nil {
			return &tuningApplyError{err: fmt.Errorf("profile: %w", err)}
		}
	}
	if plan.Fan != nil {
		if err := cli.SetBothFanCurves(plan.Fan.Points); err != nil {
			return &tuningApplyError{err: fmt.Errorf("fan curve: %w", err), profileApplied: setProfile}
		}
	} else if setProfile {
		if err := cli.ResetAllFanCurves(); err != nil {
			return &tuningApplyError{err: fmt.Errorf("reset fan curve: %w", err), profileApplied: true}
		}
	}
	if plan.TDP != nil {
		if err := cli.SetTDP(0, plan.TDP.PL1SPL, plan.TDP.PL2SPPT, plan.TDP.FPPT); err != nil {
			return &tuningApplyError{err: fmt.Errorf("TDP: %w", err), profileApplied: setProfile}
		}
	}
	if cli.SMUProbeUndervolt() {
		if plan.Undervolt != nil {
			if err := cli.SetCurveOptimizer(plan.Undervolt.CPUCO); err != nil {
				return &tuningApplyError{err: fmt.Errorf("undervolt: %w", err), profileApplied: setProfile}
			}
		} else if setProfile {
			if err := cli.ResetCurveOptimizer(); err != nil {
				return &tuningApplyError{err: fmt.Errorf("reset undervolt: %w", err), profileApplied: setProfile}
			}
		}
	}
	return nil
}

func (d *Daemon) commitTuningState(next api.State, activePreset string) error {
	d.mu.Lock()
	d.state.Profile = next.Profile
	d.state.FanCurve = cloneFanCurve(next.FanCurve)
	d.state.TDP = clonePtr(next.TDP)
	d.state.Undervolt = clonePtr(next.Undervolt)
	d.state.FanCurveActive = next.FanCurveActive
	d.state.TDPActive = next.TDPActive
	d.state.UndervoltActive = next.UndervoltActive
	if d.state.Undervolt != nil {
		d.state.Undervolt.Active = next.UndervoltActive
	}
	d.state.ActivePreset = activePreset
	d.mu.Unlock()
	return d.persistState()
}
