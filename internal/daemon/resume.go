package daemon

// resume.go — sleep/resume + lid watcher via DBus signals.
//
// Listens for org.freedesktop.login1.Manager.PrepareForSleep on the system
// bus: lighting is turned off before sleep and all volatile settings (lighting,
// undervolt, TDP, fan curves) are reapplied on resume.
//
// Also listens for org.freedesktop.UPower.LidIsClosed changes: when the lid
// reopens after a lock (no suspend), lighting is reapplied because the Aura
// device loses its config while the lid is closed.

import (
	"context"
	"log/slog"

	"github.com/godbus/dbus/v5"

	"github.com/aic0d3r/z13ctl-plus/internal/aura"
)

// watchResume connects to the system DBus and listens for resume events.
// When PrepareForSleep(false) is received, restoreVolatileState is called.
// Blocks until ctx is cancelled.
func (d *Daemon) watchResume(ctx context.Context) {
	conn, err := dbus.SystemBus()
	if err != nil {
		slog.Warn("cannot connect to system DBus for resume watcher", "err", err)
		return
	}

	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.login1.Manager"),
		dbus.WithMatchMember("PrepareForSleep"),
	); err != nil {
		slog.Warn("failed to add DBus match rule for PrepareForSleep", "err", err)
		return
	}

	// UPower emits PropertiesChanged for LidIsClosed when the lid opens/closes.
	// login1 has no lid signal; this covers the lock-only case (no suspend),
	// where the Aura device loses its config while the lid is closed.
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath("/org/freedesktop/UPower"),
		dbus.WithMatchInterface("org.freedesktop.DBus.Properties"),
		dbus.WithMatchMember("PropertiesChanged"),
	); err != nil {
		slog.Warn("failed to add DBus match rule for UPower lid", "err", err)
	}

	ch := make(chan *dbus.Signal, 8)
	conn.Signal(ch)
	slog.Info("resume watcher started (PrepareForSleep + UPower lid)")

	for {
		select {
		case <-ctx.Done():
			conn.RemoveSignal(ch)
			return
		case sig := <-ch:
			if sig == nil {
				continue
			}
			switch sig.Name {
			case "org.freedesktop.login1.Manager.PrepareForSleep":
				d.handlePrepareForSleep(sig)
			case "org.freedesktop.DBus.Properties.PropertiesChanged":
				if sig.Path != "/org/freedesktop/UPower" {
					continue
				}
				if onBatteryChanged(sig) {
					d.requestPowerCheck()
				}
				if lidOpened(sig) {
					slog.Info("lid opened, restoring lighting")
					d.restoreLighting()
				}
			}
		}
	}
}

// handlePrepareForSleep turns lighting off before sleep and restores all
// volatile state (lighting, TDP, fans, undervolt) on resume.
func (d *Daemon) handlePrepareForSleep(sig *dbus.Signal) {
	if len(sig.Body) < 1 {
		return
	}
	sleeping, ok := sig.Body[0].(bool)
	if !ok {
		return
	}
	if sleeping {
		slog.Info("system entering sleep")
		// Hold d.mu across the device use: the hotplug watcher may otherwise
		// swap (and close) d.dev mid-write.
		d.mu.Lock()
		defer d.mu.Unlock()
		if d.dev != nil {
			if err := aura.TurnOff(d.dev); err != nil {
				slog.Warn("failed to turn off lighting before sleep", "err", err)
			}
		}
		return
	}
	slog.Info("system resumed from sleep, restoring volatile state")
	d.restoreVolatileState()
}

// lidOpened reports whether a UPower PropertiesChanged signal indicates the
// lid transitioned to open (LidIsClosed became false).
// onBatteryChanged reports whether a UPower PropertiesChanged signal carries
// OnBattery, i.e. the power source may have changed.
func onBatteryChanged(sig *dbus.Signal) bool {
	if len(sig.Body) < 2 {
		return false
	}
	changed, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		return false
	}
	_, ok = changed["OnBattery"]
	return ok
}

func lidOpened(sig *dbus.Signal) bool {
	if len(sig.Body) < 2 {
		return false
	}
	changed, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		return false
	}
	v, ok := changed["LidIsClosed"]
	if !ok {
		return false
	}
	closed, ok := v.Value().(bool)
	return ok && !closed
}

// restoreVolatileState reapplies all settings that are lost on sleep/resume:
// lighting, fan curves, TDP, and Curve Optimizer offsets.
func (d *Daemon) restoreVolatileState() {
	d.mu.Lock()
	state := d.state
	d.mu.Unlock()
	defer d.requestDisplayReconcile()

	// Restore lighting (lost on sleep regardless of profile). restoreLighting
	// takes applyMu then d.mu around the device use, matching the dispatch
	// handlers' lock order.
	d.restoreLighting()

	if state.ActivePreset != "" {
		if err := d.applyPreset(state.ActivePreset); err != nil {
			slog.Warn("resume: failed to restore active preset", "preset", state.ActivePreset, "err", err)
		} else {
			slog.Info("resume: active preset restored", "preset", state.ActivePreset)
		}
		return
	}

	d.applyMu.Lock()
	defer d.applyMu.Unlock()
	if err := applyTuning(state, false); err != nil {
		slog.Warn("resume: failed to restore tuning", "err", err)
	} else {
		slog.Info("resume: tuning restored", "profile", state.Profile)
	}
}

// restoreLighting reapplies the current lighting config. Used on lid-open when
// the system locks but does not sleep: the Aura device loses its config while
// the lid is closed, so it must be re-pushed when the lid reopens.
func (d *Daemon) restoreLighting() {
	d.applyMu.Lock()
	defer d.applyMu.Unlock()
	defer d.requestDisplayReconcile()
	// d.mu guards d.dev against the hotplug reopen; applyLightingState must
	// not take d.mu itself (reopenAndRestore calls it with d.mu held).
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.dev == nil {
		return
	}
	if err := d.applyLightingState(); err != nil {
		slog.Warn("lid: failed to restore lighting", "err", err)
		return
	}
	slog.Info("lid: lighting restored")
}
