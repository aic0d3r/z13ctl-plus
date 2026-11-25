package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

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

// curveMeetsHighTDPFloor reports whether the curve cools at least as hard as
// the high-TDP safety curve at every temperature.
func curveMeetsHighTDPFloor(points []api.FanCurvePoint) bool {
	return cli.CheckHighTDPFanCurve(points) == nil
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

const (
	profilePollInterval = 2 * time.Second
	tuningRetryMin      = 10 * time.Second
	tuningRetryMax      = 5 * time.Minute
)

// recordTuning notes the result of the last full tuning apply. A failure is
// reported in get-state (TuningError) and retried by watchTuning, so clients
// never show a saved override as applied when the hardware rejected it.
func (d *Daemon) recordTuning(err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	d.mu.Lock()
	changed := d.tuningErr != msg
	d.tuningErr = msg
	d.mu.Unlock()
	if changed {
		d.broadcast(response{Event: "state-changed"})
	}
}

func hasTuningOverrides(s api.State) bool {
	return s.FanCurveActive || s.TDPActive || s.UndervoltActive
}

// watchTuning keeps the hardware matching the saved tuning. It polls
// platform_profile: a change made outside the daemon (KDE's power slider via
// power-profiles-daemon, the Fn fan-profile hotkey) is adopted as the new
// profile, and the active overrides that the profile write wiped are applied
// again. It also retries a failed tuning apply with exponential backoff.
func (d *Daemon) watchTuning(ctx context.Context) {
	retry := tuningRetryMin
	var nextRetry time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(profilePollInterval):
		}
		attempted, err := d.checkTuning(!time.Now().Before(nextRetry))
		switch {
		case !attempted:
		case err != nil:
			slog.Warn("tuning not applied; will retry", "in", retry, "err", err)
			nextRetry = time.Now().Add(retry)
			retry = min(retry*2, tuningRetryMax)
		default:
			retry, nextRetry = tuningRetryMin, time.Time{}
		}
	}
}

// checkTuning runs one watchTuning step and reports whether it applied tuning.
func (d *Daemon) checkTuning(retryDue bool) (bool, error) {
	// applyMu: every profile write by the daemon holds it, so a mismatch seen
	// here was made by someone else.
	d.applyMu.Lock()
	defer d.applyMu.Unlock()
	d.mu.Lock()
	state := d.state
	failed := d.tuningErr != ""
	d.mu.Unlock()

	current := readProfileFromSysfs()
	if isPhysicalProfile(current) && current != state.Profile {
		slog.Info("platform profile changed outside z13ctl-plus; adopting it and reapplying overrides",
			"from", state.Profile, "to", current)
		state.Profile = current
		d.mu.Lock()
		d.state.Profile = current
		d.state.ActivePreset = "" // a manual profile change ends the preset, as in handleProfile
		d.mu.Unlock()
		if err := d.persistState(); err != nil {
			slog.Warn("failed to save state", "err", err)
		}
		if !hasTuningOverrides(state) && !failed {
			return false, nil
		}
	} else if !failed || !retryDue {
		return false, nil
	}
	err := applyTuning(state, false)
	d.recordTuning(err)
	return true, err
}
