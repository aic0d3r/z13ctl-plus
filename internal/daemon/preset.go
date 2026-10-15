package daemon

// preset.go - named performance presets and automatic AC/battery selection.

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aic0d3r/z13ctl-plus/internal/cli"
	"github.com/dahui/z13ctl/api"
)

const (
	powerPolicyPollInterval = 30 * time.Second // sysfs fallback when UPower is absent
	powerPolicySettle       = 2 * time.Second  // new source must be stable this long
	powerPolicyRetry        = 10 * time.Second // after a failed apply
)

func (d *Daemon) seedDefaultPresets(fresh bool) error {
	if !fresh {
		return nil
	}

	// Without readable cpufreq (unusual hardware, containers) the daemon must still
	// start: seed the presets with a zero floor and let the CPU step fail on apply.
	minKHz := 0
	if cpu, err := cli.ReadCPUPower(); err != nil {
		slog.Warn("cpufreq unreadable; seeding default presets without a CPU frequency floor", "err", err)
	} else {
		minKHz = cpu.MinLimitKHz
	}

	d.mu.Lock()
	setDefaultPowerConfiguration(&d.state, minKHz)
	d.mu.Unlock()
	return d.persistState()
}

func setDefaultPowerConfiguration(state *api.State, minFrequencyKHz int) {
	state.Profile = "balanced"
	state.Battery = 80
	state.Presets = recommendedPresets(minFrequencyKHz)
	state.PowerPolicy = recommendedPowerPolicy()
}

func recommendedPresets(minFrequencyKHz int) map[string]api.Preset {
	panelOn, panelOff := 1, 0
	return map[string]api.Preset{
		"Plugged In": {
			Profile:        "balanced",
			RefreshRate:    180,
			PanelOverdrive: &panelOn,
			CPUPower: &api.CPUPowerPreset{
				MinFrequencyKHz: minFrequencyKHz,
				EPP:             "balance_performance",
				Boost:           true,
			},
		},
		"On Battery": {
			Profile:        "balanced",
			RefreshRate:    60,
			PanelOverdrive: &panelOff,
			CPUPower: &api.CPUPowerPreset{
				MinFrequencyKHz: minFrequencyKHz,
				EPP:             "balance_power",
				Boost:           false,
			},
		},
	}
}

func recommendedPowerPolicy() *api.PowerPolicy {
	return &api.PowerPolicy{
		Enabled:       true,
		ACPreset:      "Plugged In",
		BatteryPreset: "On Battery",
	}
}

func restoreRecommendedPowerConfiguration(state *api.State, minFrequencyKHz int) {
	presets := clonePresetMap(state.Presets)
	if presets == nil {
		presets = make(map[string]api.Preset)
	}
	for name, preset := range recommendedPresets(minFrequencyKHz) {
		presets[name] = preset
	}
	state.Presets = presets
	state.PowerPolicy = recommendedPowerPolicy()
}

func (d *Daemon) restoreRecommended() error {
	cpu, err := cli.ReadCPUPower()
	if err != nil {
		return fmt.Errorf("read CPU hardware limits: %w", err)
	}
	source, err := cli.ReadPowerSource()
	if err != nil {
		return fmt.Errorf("read power source: %w", err)
	}

	d.mu.Lock()
	restoreRecommendedPowerConfiguration(&d.state, cpu.MinLimitKHz)
	d.powerSource = source
	d.mu.Unlock()
	if err := d.persistState(); err != nil {
		return err
	}
	name := "On Battery"
	if source == cli.PowerSourceAC {
		name = "Plugged In"
	}
	return d.applyPreset(name)
}

func (d *Daemon) capturePreset() api.Preset {
	d.mu.Lock()
	p := api.Preset{
		Profile: d.state.Profile,
	}
	if d.state.TDPActive {
		p.TDP = clonePtr(d.state.TDP)
	}
	if d.state.UndervoltActive {
		p.Undervolt = clonePtr(d.state.Undervolt)
	}
	d.mu.Unlock()
	if p.Undervolt != nil {
		p.Undervolt.Active = false
	}

	if p.Profile == "" {
		p.Profile = readProfileFromSysfs()
	}
	if cpu, err := cli.ReadCPUPower(); err == nil {
		p.CPUPower = &api.CPUPowerPreset{
			MinFrequencyKHz: cpu.MinFrequencyKHz,
			EPP:             cpu.EPP,
			Boost:           cpu.Boost,
		}
	}
	p.RefreshRate = d.desiredDisplayRefresh()
	if panel, err := readOptionalInt(cli.FindPanelOverdrivePath()); err == nil {
		p.PanelOverdrive = &panel
	}
	return p
}

func (d *Daemon) savePreset(name string) error {
	name, err := validatePresetName(name)
	if err != nil {
		return err
	}
	preset := d.capturePreset()
	if err := validatePreset(preset); err != nil {
		return err
	}
	d.mu.Lock()
	presets := clonePresetMap(d.state.Presets)
	if presets == nil {
		presets = make(map[string]api.Preset)
	}
	presets[name] = preset
	d.state.Presets = presets
	d.mu.Unlock()
	return d.persistState()
}

func (d *Daemon) deletePreset(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.state.Presets[name]; !ok {
		return fmt.Errorf("preset %q not found", name)
	}
	if policy := d.state.PowerPolicy; policy != nil && (policy.ACPreset == name || policy.BatteryPreset == name) {
		return fmt.Errorf("preset %q is assigned by the power policy", name)
	}
	presets := clonePresetMap(d.state.Presets)
	delete(presets, name)
	d.state.Presets = presets
	if d.state.ActivePreset == name {
		d.state.ActivePreset = ""
	}
	return saveState(d.state)
}

// renamePreset renames a preset, carrying over its power-policy assignments
// and active marker.
func (d *Daemon) renamePreset(from, to string) error {
	to, err := validatePresetName(to)
	if err != nil {
		return err
	}
	d.mu.Lock()
	if err := renamePresetInState(&d.state, from, to); err != nil {
		d.mu.Unlock()
		return err
	}
	d.mu.Unlock()
	return d.persistState()
}

func renamePresetInState(state *api.State, from, to string) error {
	preset, ok := state.Presets[from]
	if !ok {
		return fmt.Errorf("preset %q not found", from)
	}
	if from == to {
		return nil
	}
	if _, exists := state.Presets[to]; exists {
		return fmt.Errorf("preset %q already exists", to)
	}
	presets := clonePresetMap(state.Presets)
	delete(presets, from)
	presets[to] = preset
	state.Presets = presets
	if p := state.PowerPolicy; p != nil {
		policy := *p
		if policy.ACPreset == from {
			policy.ACPreset = to
		}
		if policy.BatteryPreset == from {
			policy.BatteryPreset = to
		}
		state.PowerPolicy = &policy
	}
	if state.ActivePreset == from {
		state.ActivePreset = to
	}
	return nil
}

func (d *Daemon) applyPreset(name string) error {
	d.mu.Lock()
	preset, ok := d.state.Presets[name]
	d.mu.Unlock()
	if !ok {
		return fmt.Errorf("preset %q not found", name)
	}
	return d.applyPresetValue(name, preset)
}

// applyPresetValue applies preset and records name as the active preset; an
// empty name applies an unnamed snapshot. A failing step does not stop the
// remaining ones; failures are reported together afterwards.
func (d *Daemon) applyPresetValue(name string, preset api.Preset) error {
	d.applyMu.Lock()
	defer d.applyMu.Unlock()

	d.mu.Lock()
	next := d.state
	d.mu.Unlock()
	preset = clonePreset(preset)
	if err := validatePreset(preset); err != nil {
		return fmt.Errorf("preset %q: %w", name, err)
	}
	if err := validatePresetHardware(preset); err != nil {
		return fmt.Errorf("preset %q: %w", name, err)
	}
	// Fan mode stays user-controlled: presets neither capture nor apply it,
	// and a FanCurve stored by older versions is ignored.
	next.Profile = preset.Profile
	next.TDP = clonePtr(preset.TDP)
	next.Undervolt = clonePtr(preset.Undervolt)
	next.TDPActive = preset.TDP != nil
	next.UndervoltActive = preset.Undervolt != nil

	var failed []string
	if err := applyTuning(next, true); err != nil {
		failed = append(failed, err.Error())
		if profileAppliedBeforeError(err) {
			_ = d.commitTuningState(next, "")
		}
	} else if err := d.commitTuningState(next, ""); err != nil {
		return err
	}
	if cpu := preset.CPUPower; cpu != nil {
		current, _ := cli.ReadCPUPower()
		if current.MinFrequencyKHz != cpu.MinFrequencyKHz {
			if err := cli.SetCPUMinFrequency(cpu.MinFrequencyKHz); err != nil {
				failed = append(failed, "CPU minimum: "+err.Error())
			}
		}
		if current.EPP != cpu.EPP {
			if err := cli.SetCPUEPP(cpu.EPP); err != nil {
				failed = append(failed, "CPU EPP: "+err.Error())
			}
		}
		if current.Boost != cpu.Boost {
			if err := cli.SetCPUBoost(cpu.Boost); err != nil {
				failed = append(failed, "CPU boost: "+err.Error())
			}
		}
	}
	panelApplied := false
	if preset.PanelOverdrive != nil {
		current, readErr := readOptionalInt(cli.FindPanelOverdrivePath())
		if readErr == nil && current == *preset.PanelOverdrive {
			panelApplied = true
		} else if err := cli.SetPanelOverdrive(*preset.PanelOverdrive); err != nil {
			failed = append(failed, "panel overdrive: "+err.Error())
		} else {
			panelApplied = true
		}
	}
	if preset.RefreshRate != 0 {
		if err := d.setDisplayRefresh(preset.RefreshRate); err != nil {
			failed = append(failed, "refresh rate: "+err.Error())
		}
	}

	d.mu.Lock()
	if panelApplied {
		d.state.PanelOverdrive = *preset.PanelOverdrive
	}
	d.state.ActivePreset = name
	d.mu.Unlock()
	if err := d.persistState(); err != nil {
		return err
	}
	label := name
	if label == "" {
		label = "pre-automation settings"
	}
	if len(failed) > 0 {
		slog.Warn("preset partly applied", "name", label, "failed", failed)
		return fmt.Errorf("%s partly applied: %s", label, strings.Join(failed, "; "))
	}
	slog.Info("preset applied", "name", label)
	return nil
}

// setPreset creates or replaces name with explicit values (the drawer's
// preset editor). Re-applies it when it is the active preset.
func (d *Daemon) setPreset(name string, preset api.Preset) error {
	name, err := validatePresetName(name)
	if err != nil {
		return err
	}
	preset = clonePreset(preset)
	preset.FanCurve = nil
	preset.BaseProfile = ""
	if preset.Undervolt != nil {
		preset.Undervolt.Active = false
	}
	if err := validatePreset(preset); err != nil {
		return err
	}
	if err := validatePresetHardware(preset); err != nil {
		return err
	}
	d.mu.Lock()
	presets := clonePresetMap(d.state.Presets)
	if presets == nil {
		presets = make(map[string]api.Preset)
	}
	presets[name] = preset
	d.state.Presets = presets
	active := d.state.ActivePreset == name
	d.mu.Unlock()
	if err := d.persistState(); err != nil {
		return err
	}
	if active {
		return d.applyPreset(name)
	}
	return nil
}

func readOptionalInt(path string) (int, error) {
	value, err := cli.ReadTrimmedFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(value)
}

func validatePresetName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 40 || strings.ContainsAny(name, "\r\n\t") {
		return "", fmt.Errorf("preset name must be 1-40 printable characters")
	}
	return name, nil
}

func validatePreset(p api.Preset) error {
	if !isPhysicalProfile(p.Profile) {
		return fmt.Errorf("invalid profile %q", p.Profile)
	}
	if p.RefreshRate != 0 && p.RefreshRate != 60 && p.RefreshRate != 180 {
		return fmt.Errorf("refresh rate must be 60 or 180")
	}
	if p.PanelOverdrive != nil && *p.PanelOverdrive != 0 && *p.PanelOverdrive != 1 {
		return fmt.Errorf("panel overdrive must be 0 or 1")
	}
	if p.FanCurve != nil {
		if p.FanCurve.Mode != 1 || len(p.FanCurve.Points) != 8 {
			return fmt.Errorf("fan curve must contain 8 custom points")
		}
		for i, point := range p.FanCurve.Points {
			if point.Temp < 0 || point.Temp > 120 || point.PWM < 0 || point.PWM > 255 {
				return fmt.Errorf("invalid fan curve point %d:%d", point.Temp, point.PWM)
			}
			if i > 0 && (point.Temp <= p.FanCurve.Points[i-1].Temp || point.PWM < p.FanCurve.Points[i-1].PWM) {
				return fmt.Errorf("fan curve temperatures and PWM must be non-decreasing")
			}
		}
	}
	if p.TDP != nil {
		for _, watts := range []int{p.TDP.PL1SPL, p.TDP.PL2SPPT, p.TDP.FPPT} {
			if watts < cli.TDPMin || watts > cli.TDPMaxForced {
				return fmt.Errorf("TDP %dW out of range %d-%d", watts, cli.TDPMin, cli.TDPMaxForced)
			}
		}
	}
	if p.Undervolt != nil {
		if err := cli.ValidateCOValues(p.Undervolt.CPUCO); err != nil {
			return err
		}
	}
	return nil
}

func validatePresetHardware(p api.Preset) error {
	if p.Undervolt != nil && !cli.SMUProbeUndervolt() {
		return fmt.Errorf("undervolt unavailable: ryzen_smu is not loaded")
	}
	if p.CPUPower == nil {
		return nil
	}
	current, err := cli.ReadCPUPower()
	if err != nil {
		return fmt.Errorf("CPU power controls unavailable: %w", err)
	}
	if p.CPUPower.MinFrequencyKHz < current.MinLimitKHz || p.CPUPower.MinFrequencyKHz > current.MaxLimitKHz {
		return fmt.Errorf("CPU minimum %d kHz out of range %d-%d", p.CPUPower.MinFrequencyKHz, current.MinLimitKHz, current.MaxLimitKHz)
	}
	if !slices.Contains(current.EPPChoices, p.CPUPower.EPP) {
		return fmt.Errorf("CPU EPP %q is unavailable", p.CPUPower.EPP)
	}
	return nil
}

func cloneFanCurve(in *api.FanCurveState) *api.FanCurveState {
	if in == nil {
		return nil
	}
	out := *in
	out.Points = append([]api.FanCurvePoint(nil), in.Points...)
	return &out
}

func clonePtr[T any](in *T) *T {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func clonePreset(in api.Preset) api.Preset {
	in.FanCurve = cloneFanCurve(in.FanCurve)
	in.TDP = clonePtr(in.TDP)
	in.Undervolt = clonePtr(in.Undervolt)
	in.CPUPower = clonePtr(in.CPUPower)
	in.PanelOverdrive = clonePtr(in.PanelOverdrive)
	return in
}

func clonePresetMap(in map[string]api.Preset) map[string]api.Preset {
	if in == nil {
		return nil
	}
	out := make(map[string]api.Preset, len(in))
	for name, preset := range in {
		out[name] = clonePreset(preset)
	}
	return out
}

func (d *Daemon) setPowerPolicy(enabled bool, acPreset, batteryPreset string) error {
	// Captured before taking mu: capturePreset reads hardware and locks mu.
	var snapshot *api.Preset
	if enabled {
		d.mu.Lock()
		wasOn := d.state.PowerPolicy != nil && d.state.PowerPolicy.Enabled
		d.mu.Unlock()
		if !wasOn {
			s := d.capturePreset()
			snapshot = &s
		}
	}

	d.mu.Lock()
	current := d.state.PowerPolicy
	if acPreset == "" {
		if current != nil {
			acPreset = current.ACPreset
		} else {
			acPreset = "Plugged In"
		}
	}
	if batteryPreset == "" {
		if current != nil {
			batteryPreset = current.BatteryPreset
		} else {
			batteryPreset = "On Battery"
		}
	}
	if _, ok := d.state.Presets[acPreset]; !ok {
		d.mu.Unlock()
		return fmt.Errorf("AC preset %q not found", acPreset)
	}
	if _, ok := d.state.Presets[batteryPreset]; !ok {
		d.mu.Unlock()
		return fmt.Errorf("battery preset %q not found", batteryPreset)
	}
	if current != nil && current.Enabled == enabled && current.ACPreset == acPreset && current.BatteryPreset == batteryPreset {
		d.mu.Unlock()
		return nil
	}
	wasEnabled := current != nil && current.Enabled
	applyNow := enabled && !wasEnabled
	restore := restoreOnDisable(wasEnabled, enabled, d.state.ActivePreset, d.state.PreAutomation)
	switch {
	case !enabled:
		d.state.PreAutomation = nil
	case !wasEnabled:
		d.state.PreAutomation = snapshot
	}
	if enabled && wasEnabled {
		switch d.powerSource {
		case cli.PowerSourceAC:
			applyNow = current.ACPreset != acPreset
		case cli.PowerSourceBattery:
			applyNow = current.BatteryPreset != batteryPreset
		}
	}
	d.state.PowerPolicy = &api.PowerPolicy{Enabled: enabled, ACPreset: acPreset, BatteryPreset: batteryPreset}
	source := d.powerSource
	d.mu.Unlock()
	if err := d.persistState(); err != nil {
		return err
	}
	if restore != nil {
		return d.applyPresetValue("", *restore)
	}
	if applyNow {
		if source != cli.PowerSourceAC && source != cli.PowerSourceBattery {
			var err error
			source, err = cli.ReadPowerSource()
			if err != nil {
				d.requestPolicyApply()
				return fmt.Errorf("read power source: %w", err)
			}
		}
		name := batteryPreset
		if source == cli.PowerSourceAC {
			name = acPreset
		}
		if err := d.applyPreset(name); err != nil {
			d.requestPolicyApply()
			return err
		}
	}
	return nil
}

func restoreOnDisable(wasEnabled, enabled bool, activePreset string, snapshot *api.Preset) *api.Preset {
	if !wasEnabled || enabled || activePreset == "" {
		return nil
	}
	return snapshot
}

func (d *Daemon) clearActivePreset() {
	d.mu.Lock()
	if d.state.ActivePreset == "" {
		d.mu.Unlock()
		return
	}
	d.state.ActivePreset = ""
	d.mu.Unlock()
	if err := d.persistState(); err != nil {
		slog.Warn("failed to clear active preset", "err", err)
	}
}

func (d *Daemon) requestPolicyApply() {
	select {
	case d.policyCh <- struct{}{}:
	default:
	}
}

func (d *Daemon) applyInitialPowerPolicy() bool {
	source, err := cli.ReadPowerSource()
	if err != nil {
		return false
	}
	d.mu.Lock()
	d.powerSource = source
	policy := clonePtr(d.state.PowerPolicy)
	d.mu.Unlock()
	if policy == nil || !policy.Enabled {
		return false
	}
	name := policy.BatteryPreset
	if source == cli.PowerSourceAC {
		name = policy.ACPreset
	}
	if err := d.applyPreset(name); err != nil {
		slog.Warn("failed to apply initial power preset; watcher will retry", "source", source, "preset", name, "err", err)
		d.requestPolicyApply()
	}
	return true
}

func (d *Daemon) requestPowerCheck() {
	select {
	case d.powerCh <- struct{}{}:
	default:
	}
}

// watchPowerPolicy applies the assigned preset after a real power-source
// transition. UPower OnBattery signals wake it immediately; a slow sysfs poll
// covers systems without UPower. A new source must stay stable for
// powerPolicySettle before its preset is applied.
func (d *Daemon) watchPowerPolicy(ctx context.Context) {
	timer := time.NewTimer(powerPolicyPollInterval)
	defer timer.Stop()
	d.mu.Lock()
	lastAppliedSource := d.powerSource
	observedSource := d.powerSource
	d.mu.Unlock()
	force := false
	nextApply := time.Now().Add(3 * time.Second)

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.policyCh:
			force = true
			nextApply = time.Now().Add(3 * time.Second)
		case <-d.powerCh:
		case <-timer.C:
		}
		d.checkPowerPolicy(&observedSource, &lastAppliedSource, &force, &nextApply)
		wait := powerPolicyPollInterval
		if force {
			wait = max(time.Until(nextApply), 100*time.Millisecond)
		}
		timer.Reset(wait)
	}
}

func (d *Daemon) checkPowerPolicy(observedSource, lastAppliedSource *string, force *bool, nextApply *time.Time) {
	source, err := cli.ReadPowerSource()
	if err != nil {
		return
	}
	d.mu.Lock()
	d.powerSource = source
	policy := clonePtr(d.state.PowerPolicy)
	activePreset := d.state.ActivePreset
	d.mu.Unlock()
	if source != *observedSource {
		*observedSource = source
		assigned := ""
		if policy != nil {
			assigned = policy.BatteryPreset
			if source == cli.PowerSourceAC {
				assigned = policy.ACPreset
			}
		}
		if activePreset == assigned && assigned != "" {
			*lastAppliedSource = source
			*force = false
		} else {
			*force = true
			*nextApply = time.Now().Add(powerPolicySettle)
		}
		return
	}
	if policy == nil || !policy.Enabled || (!*force && source == *lastAppliedSource) {
		return
	}
	if time.Now().Before(*nextApply) {
		return
	}
	name := policy.BatteryPreset
	if source == cli.PowerSourceAC {
		name = policy.ACPreset
	}
	if err := d.applyPreset(name); err != nil {
		slog.Warn("failed to apply power preset; will retry", "source", source, "preset", name, "err", err)
		*nextApply = time.Now().Add(powerPolicyRetry)
		return
	}
	*lastAppliedSource = source
	*force = false
}
