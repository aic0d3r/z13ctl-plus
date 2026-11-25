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
	"os"
	"time"

	"github.com/dahui/z13ctl/api"
	"github.com/godbus/dbus/v5"

	"github.com/aic0d3r/z13ctl-plus/internal/aura"
	"github.com/aic0d3r/z13ctl-plus/internal/cli"
)

const resumeReconnectMax = time.Minute

// watchResume listens for sleep, resume, lid and power-source signals on the
// system bus until ctx is cancelled, reconnecting with backoff when the bus
// connection drops so resume restore never silently stops.
func (d *Daemon) watchResume(ctx context.Context) {
	backoff := time.Second
	for {
		started := time.Now()
		d.watchResumeOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if time.Since(started) > resumeReconnectMax {
			backoff = time.Second
		}
		slog.Warn("system DBus watcher stopped; reconnecting", "in", backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, resumeReconnectMax)
	}
}

// takeSleepDelayLock asks logind to delay sleep until the returned file is
// closed, so the lightbar-off write finishes before the system suspends.
// logind caps the delay (InhibitDelayMaxSec, 5 s by default).
func takeSleepDelayLock(conn *dbus.Conn) *os.File {
	var fd dbus.UnixFD
	err := conn.Object("org.freedesktop.login1", "/org/freedesktop/login1").Call(
		"org.freedesktop.login1.Manager.Inhibit", 0,
		"sleep", "z13ctl-plus", "Turn off the lightbar before sleep", "delay",
	).Store(&fd)
	if err != nil {
		slog.Warn("cannot take logind sleep delay lock; lightbar may stay on during sleep", "err", err)
		return nil
	}
	return os.NewFile(uintptr(fd), "logind-sleep-delay")
}

// watchResumeOnce runs one system bus connection until it drops or ctx ends.
func (d *Daemon) watchResumeOnce(ctx context.Context) {
	// A private connection: closing it here must not affect other bus users.
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		slog.Warn("cannot connect to system DBus for resume watcher", "err", err)
		return
	}
	defer func() { _ = conn.Close() }()
	lock := takeSleepDelayLock(conn)
	defer func() {
		if lock != nil {
			_ = lock.Close()
		}
	}()

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
	// OnBattery changes may have been missed while disconnected.
	d.requestPowerCheck()

	for {
		select {
		case <-ctx.Done():
			conn.RemoveSignal(ch)
			return
		case sig, ok := <-ch:
			if !ok {
				slog.Warn("system DBus connection closed; resume watcher stopped")
				return
			}
			if sig == nil {
				continue
			}
			switch sig.Name {
			case "org.freedesktop.login1.Manager.PrepareForSleep":
				sleeping, ok := prepareForSleepArg(sig)
				if !ok {
					continue
				}
				if sleeping {
					d.prepareForSleep()
					if lock != nil {
						_ = lock.Close() // lets the suspend proceed
						lock = nil
					}
					continue
				}
				if lock == nil {
					lock = takeSleepDelayLock(conn)
				}
				slog.Info("system resumed from sleep, restoring volatile state")
				d.restoreVolatileState()
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

// prepareForSleepArg returns PrepareForSleep's argument: true before sleep,
// false after resume.
func prepareForSleepArg(sig *dbus.Signal) (sleeping, ok bool) {
	if len(sig.Body) < 1 {
		return false, false
	}
	sleeping, ok = sig.Body[0].(bool)
	return sleeping, ok
}

// prepareForSleep turns the lighting off; the keyboard goes dark by itself
// but the lightbar does not.
func (d *Daemon) prepareForSleep() {
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
// lighting, fan curves, TDP, Curve Optimizer offsets, and a camera turned off.
func (d *Daemon) restoreVolatileState() {
	d.mu.Lock()
	state := d.state
	prevSource := d.powerSource
	d.mu.Unlock()
	defer d.requestDisplayReconcile()

	// The power source may have changed while asleep (e.g. unplugged); restore
	// the preset assigned to the current source rather than briefly applying
	// the stale one before the policy watcher catches up.
	if source, err := cli.ReadPowerSource(); err == nil {
		state.ActivePreset = resumePreset(state, prevSource, source)
	}

	// Restore lighting (lost on sleep regardless of profile). restoreLighting
	// takes applyMu then d.mu around the device use, matching the dispatch
	// handlers' lock order.
	d.restoreLighting()

	d.applyMu.Lock()
	restoreCamera(state.CameraOff, "resume")
	d.applyMu.Unlock()

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
	err := applyTuning(state, false)
	d.recordTuning(err)
	if err != nil {
		slog.Warn("resume: failed to restore tuning", "err", err)
	} else {
		slog.Info("resume: tuning restored", "profile", state.Profile)
	}
}

// resumePreset returns the preset to restore after resume: the policy's
// preset for the current source if the source changed while asleep and power
// automation is on, otherwise the preset that was active before sleep.
func resumePreset(state api.State, prevSource, source string) string {
	p := state.PowerPolicy
	if source == prevSource || p == nil || !p.Enabled {
		return state.ActivePreset
	}
	if source == cli.PowerSourceAC {
		return p.ACPreset
	}
	return p.BatteryPreset
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
