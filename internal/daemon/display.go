package daemon

// display.go - serialized internal-panel intent coordinator for KDE/KScreen.

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/aic0d3r/z13ctl-plus/internal/cli"
)

const (
	displayRetryInterval = 2 * time.Second
	dockSettleInterval   = 10 * time.Second
	displayMaxBackoff    = time.Minute // no KScreen (GNOME, Gamescope) or no eDP-1
)

type displayIntent struct {
	posture   string
	rotation  string
	refresh   int
	notBefore time.Time
}

func (d *Daemon) setDisplayPosture(posture string) error {
	switch posture {
	case "desktop", "laptop", "tablet":
	default:
		return fmt.Errorf("invalid posture %q (valid: desktop, laptop, tablet)", posture)
	}
	d.displayMu.Lock()
	d.display.posture = posture
	if posture != "tablet" {
		d.display.rotation = "none"
	}
	d.display.notBefore = time.Now().Add(displayRetryInterval)
	if posture == "desktop" {
		d.display.notBefore = time.Now().Add(dockSettleInterval)
	}
	d.displayMu.Unlock()
	d.requestDisplayReconcile()
	return nil
}

func (d *Daemon) setDisplayRotation(rotation string) error {
	switch rotation {
	case "none", "left", "right", "inverted":
	default:
		return fmt.Errorf("invalid rotation %q (valid: none, left, right, inverted)", rotation)
	}
	d.displayMu.Lock()
	d.display.rotation = rotation
	d.displayMu.Unlock()
	d.requestDisplayReconcile()
	return nil
}

func (d *Daemon) setDisplayRefresh(hz int) error {
	if !slices.Contains(cli.SupportedRefreshRates(), hz) {
		return fmt.Errorf("unsupported refresh rate %d", hz)
	}
	d.displayMu.Lock()
	d.display.refresh = hz
	d.displayMu.Unlock()
	d.requestDisplayReconcile()
	return nil
}

func (d *Daemon) desiredDisplayRefresh() int {
	d.displayMu.Lock()
	refresh := d.display.refresh
	d.displayMu.Unlock()
	if refresh != 0 {
		return refresh
	}
	return cli.ReadRefreshRate()
}

func (d *Daemon) requestDisplayReconcile() {
	select {
	case d.displayCh <- struct{}{}:
	default:
	}
}

func (d *Daemon) watchDisplay(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	dirty := false
	var backoff time.Duration
	var nextTry time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.displayCh:
			dirty = true
			backoff, nextTry = 0, time.Time{} // a fresh intent tries now
		case <-ticker.C:
			if !dirty || time.Now().Before(nextTry) {
				continue
			}
			displayed, retry, err := d.reconcileDisplay()
			backoff, nextTry = nextDisplayBackoff(backoff, err)
			if err != nil && backoff == time.Second {
				slog.Warn("display reconciliation failed; retrying with backoff", "err", err)
			}
			if displayed {
				slog.Info("display intent applied")
			}
			dirty = retry
		}
	}
}

// nextDisplayBackoff doubles the retry delay after each failed reconcile (1 s
// up to displayMaxBackoff) and clears it on success.
func nextDisplayBackoff(backoff time.Duration, err error) (time.Duration, time.Time) {
	if err == nil {
		return 0, time.Time{}
	}
	backoff = min(max(2*backoff, time.Second), displayMaxBackoff)
	return backoff, time.Now().Add(backoff)
}

// reconcileDisplay returns whether it changed KScreen state and whether it
// should retry. A disabled internal panel is intentionally left untouched.
func (d *Daemon) reconcileDisplay() (changed, retry bool, err error) {
	d.displayMu.Lock()
	intent := d.display
	d.displayMu.Unlock()
	if intent.posture == "" && intent.refresh == 0 {
		return false, false, nil
	}
	if time.Now().Before(intent.notBefore) {
		return false, true, nil
	}

	d.applyMu.Lock()
	defer d.applyMu.Unlock()
	current, err := cli.ReadInternalDisplay()
	if err != nil {
		return false, true, err
	}
	if !current.Enabled {
		return false, false, nil
	}

	scale := 0.0
	rotation := ""
	switch intent.posture {
	case "desktop", "laptop":
		rotation = "none"
	case "tablet":
		rotation = intent.rotation
	default:
		// No posture reported (tablet kit absent): scale and rotation are not
		// ours to set, but refresh intents still apply.
	}
	// Scale changes only on a posture transition, so a scale the user picks in
	// System Settings sticks and is restored when they return to that posture.
	d.mu.Lock()
	keepScale := d.state.TabletSettings.KeepDisplayScale
	d.mu.Unlock()
	if !keepScale && intent.posture != "" {
		scale = d.postureScale(intent.posture, current.Scale)
	}
	refresh := intent.refresh
	if scale != 0 && math.Abs(current.Scale-scale) < 0.01 {
		scale = 0
	}
	if current.Rotation == rotation {
		rotation = ""
	}
	if current.Refresh == refresh {
		refresh = 0
	}
	if scale != 0 || rotation != "" || refresh != 0 {
		if err := cli.ApplyInternalDisplay(scale, rotation, refresh); err != nil {
			return false, true, err
		}
		changed = true
	}
	if intent.posture != "" {
		d.scalePosture = intent.posture
	}
	return changed, false, nil
}

// postureScale remembers current as the scale of the posture last reconciled
// and returns the scale to apply for posture (0 leaves the panel alone).
// Remembered scales are persisted, so booting straight into a posture or a
// daemon restart still restores them. Without a remembered scale, tablet
// defaults to 1.75; the startup posture otherwise keeps KDE's scale and a
// later transition uses 1.5. Callers hold applyMu.
func (d *Daemon) postureScale(posture string, current float64) float64 {
	d.mu.Lock()
	scales := d.state.PostureScales
	changed := false
	if d.scalePosture != "" && math.Abs(scales[d.scalePosture]-current) >= 0.01 {
		scales = maps.Clone(scales) // get-state may still be marshaling the old map
		if scales == nil {
			scales = map[string]float64{}
		}
		scales[d.scalePosture] = current
		d.state.PostureScales = scales
		changed = true
	}
	target := 0.0
	if posture != d.scalePosture {
		target = scales[posture]
		switch {
		case target != 0:
		case posture == "tablet":
			target = 1.75
		case d.scalePosture != "":
			target = 1.5
		}
	}
	d.mu.Unlock()
	if changed {
		if err := d.persistState(); err != nil {
			slog.Warn("failed to save posture scale", "err", err)
		}
	}
	return target
}
