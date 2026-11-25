package daemon

// hotplug.go — detachable-keyboard reattach watcher.
//
// The 2025 ROG Flow Z13 keyboard is detachable and its RGB lighting is lost when
// it is removed. On reattach the firmware does not restore the previous effect, so
// the daemon polls for the keyboard and re-applies the saved lighting state via
// reopenAndRestore whenever the handle it holds does not match what is plugged in.

import (
	"context"
	"time"

	"github.com/aic0d3r/z13ctl-plus/internal/hid"
)

const (
	// hotplugPollInterval is how often watchHotplug checks the keyboard.
	hotplugPollInterval = 2 * time.Second
	// hotplugMaxBackoff caps the retry interval while reopening keeps failing.
	hotplugMaxBackoff = time.Minute
)

// watchHotplug runs until ctx is done. It compares the keyboard that is plugged
// in with the handle the daemon holds, so it also catches a keyboard that
// could not be opened at startup and one that re-enumerated between two polls.
// Failed reopens (e.g. udev has not applied hidraw permissions yet) are retried
// with backoff.
func (d *Daemon) watchHotplug(ctx context.Context) {
	wait := hotplugPollInterval
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if !d.keyboardHandleStale() {
			wait = hotplugPollInterval
			continue
		}
		// Success means the keyboard is now held: the lightbar alone opening
		// (keyboard node not yet accessible) still needs a retry.
		if d.reopenAndRestore() && !d.keyboardHandleStale() {
			wait = hotplugPollInterval
		} else {
			wait = min(wait*2, hotplugMaxBackoff)
		}
	}
}

func (d *Daemon) keyboardHandleStale() bool {
	d.mu.Lock()
	held, live := false, false
	if d.dev != nil {
		held, live = d.dev.Holds("keyboard")
	}
	d.mu.Unlock()
	return hotplugNeedsReopen(hid.HasDevice("keyboard"), held, live)
}

// hotplugNeedsReopen reports whether the held HID handle is out of date: the
// keyboard is plugged in but not held through a live node (never opened, or
// re-enumerated), or it is gone but its dead node is still held, which would
// fail every write and starve the lightbar.
func hotplugNeedsReopen(present, held, live bool) bool {
	if present {
		return !live
	}
	return held
}
