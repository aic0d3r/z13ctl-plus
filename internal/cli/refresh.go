package cli

// refresh.go — display refresh rate switching via kscreen-doctor.
//
// The Z13's eDP panel runs at 2560x1600 natively and supports 60 Hz and 180 Hz
// at that resolution. kscreen-doctor is the Plasma/Wayland tool for atomic
// mode setting; it is unprivileged and present on every Plasma 6 system.
//
// Mode specification uses resolution@rate (mode IDs are unstable across
// kscreen reconfigurations). We hardcode 2560x1600 because it is the only
// native resolution worth switching refresh rate for — lower resolutions are
// GPU-scaled and have no battery benefit.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	nativeMode = "2560x1600"
	kscreenBin = "kscreen-doctor"
	outputName = "eDP-1"

	refreshTimeout = 3 * time.Second

	systemdBusName = "org.freedesktop.systemd1"
	systemdPath    = dbus.ObjectPath("/org/freedesktop/systemd1")
	systemdManager = "org.freedesktop.systemd1.Manager"
)

// supportedRefreshRates is the whitelist we expose in the UI. The panel
// advertises many modes; only these two are useful (60 = battery saver,
// 180 = full speed). Order defines UI order.
var supportedRefreshRates = []int{60, 180}

// refreshCacheTTL bounds how stale a cached refresh-rate read may be. It must
// exceed the drawer's 1 s state poll or nearly every poll spawns kscreen-doctor;
// our own changes invalidate it, so only changes made elsewhere lag.
const refreshCacheTTL = 5 * time.Second

var (
	refreshCacheMu      sync.Mutex
	refreshCacheSummary DisplaySummary
	refreshCacheAt      time.Time
)

// DisplaySummary is what the drawer needs to label its refresh-rate control.
type DisplaySummary struct {
	Refresh  int  // live eDP-1 rate in Hz; 0 when unknown or the panel is off
	PanelOff bool // eDP-1 was read and is disabled (not set when the read failed)
	External bool // at least one output other than eDP-1 is enabled
}

// InternalDisplayState is the current KScreen state for the built-in panel.
type InternalDisplayState struct {
	Enabled  bool
	Refresh  int
	Scale    float64
	Rotation string
}

// SupportedRefreshRates returns the refresh rates selectable in the UI.
func SupportedRefreshRates() []int {
	return append([]int(nil), supportedRefreshRates...)
}

// ReadRefreshRate returns the current eDP-1 refresh rate in Hz (integer).
// Returns 0 if kscreen-doctor is missing or eDP-1 is not connected or enabled.
func ReadRefreshRate() int {
	return ReadDisplaySummary().Refresh
}

// ReadDisplaySummary reads eDP-1 and the other outputs in one kscreen-doctor
// call. Results are cached for refreshCacheTTL: callers poll this per get-state
// request and each miss would spawn a kscreen-doctor process.
func ReadDisplaySummary() DisplaySummary {
	refreshCacheMu.Lock()
	if time.Since(refreshCacheAt) < refreshCacheTTL {
		summary := refreshCacheSummary
		refreshCacheMu.Unlock()
		return summary
	}
	refreshCacheMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()
	var summary DisplaySummary
	if out, err := kscreenCommand(ctx, "-j").Output(); err == nil {
		summary = summarizeDisplays(out)
	}

	refreshCacheMu.Lock()
	defer refreshCacheMu.Unlock()
	refreshCacheAt = time.Now()
	refreshCacheSummary = summary
	return summary
}

// kscreenOutput is the part of one `kscreen-doctor -j` output we use.
type kscreenOutput struct {
	Name          string  `json:"name"`
	Enabled       bool    `json:"enabled"`
	CurrentModeID string  `json:"currentModeId"`
	Scale         float64 `json:"scale"`
	Rotation      int     `json:"rotation"`
	Modes         []struct {
		ID          string  `json:"id"`
		RefreshRate float64 `json:"refreshRate"`
	} `json:"modes"`
}

func parseKScreen(out []byte) []kscreenOutput {
	var config struct {
		Outputs []kscreenOutput `json:"outputs"`
	}
	if json.Unmarshal(out, &config) != nil {
		return nil
	}
	return config.Outputs
}

// summarizeDisplays parses kscreen-doctor -j output into a DisplaySummary.
func summarizeDisplays(out []byte) DisplaySummary {
	var summary DisplaySummary
	if panel, ok := parseInternalDisplay(out); ok {
		if panel.Enabled {
			summary.Refresh = panel.Refresh
		} else {
			summary.PanelOff = true
		}
	}
	for _, o := range parseKScreen(out) {
		if o.Name != outputName && o.Enabled {
			summary.External = true
		}
	}
	return summary
}

// ReadInternalDisplay returns the enabled, mode, scale, and rotation state of eDP-1.
func ReadInternalDisplay() (InternalDisplayState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()
	out, err := kscreenCommand(ctx, "-j").Output()
	if err != nil {
		return InternalDisplayState{}, err
	}
	state, ok := parseInternalDisplay(out)
	if !ok {
		return InternalDisplayState{}, fmt.Errorf("%s output not found", outputName)
	}
	return state, nil
}

// parseInternalDisplay extracts the eDP-1 state and current mode from
// kscreen-doctor -j output.
func parseInternalDisplay(out []byte) (InternalDisplayState, bool) {
	for _, o := range parseKScreen(out) {
		if o.Name != outputName {
			continue
		}
		state := InternalDisplayState{Enabled: o.Enabled, Scale: o.Scale, Rotation: rotationName(o.Rotation)}
		for _, m := range o.Modes {
			if m.ID == o.CurrentModeID {
				state.Refresh = int(math.Round(m.RefreshRate))
			}
		}
		return state, true
	}
	return InternalDisplayState{}, false
}

// ApplyInternalDisplay changes only the non-zero internal-panel properties in
// one KScreen transaction. External outputs are never included.
func ApplyInternalDisplay(scale float64, rotation string, hz int) error {
	args := make([]string, 0, 3)
	if scale != 0 {
		if scale < 1 || scale > 3 {
			return fmt.Errorf("unsupported display scale %g", scale)
		}
		args = append(args, fmt.Sprintf("output.%s.scale.%g", outputName, scale))
	}
	if rotation != "" {
		switch rotation {
		case "none", "left", "right", "inverted":
		default:
			return fmt.Errorf("unsupported display rotation %q", rotation)
		}
		args = append(args, "output."+outputName+".rotation."+rotation)
	}
	if hz != 0 {
		if !slices.Contains(supportedRefreshRates, hz) {
			return fmt.Errorf("unsupported refresh rate %d (valid: %v)", hz, supportedRefreshRates)
		}
		args = append(args, "output."+outputName+".mode."+fmt.Sprintf("%s@%d", nativeMode, hz))
	}
	if len(args) == 0 {
		return nil
	}
	defer invalidateDisplayCache()
	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()
	cmd := kscreenCommand(ctx, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kscreen-doctor %s: %w%s", strings.Join(args, " "), err, suffixNonEmpty(stderr.String()))
	}
	return nil
}

// invalidateDisplayCache makes the next ReadDisplaySummary read KScreen again.
func invalidateDisplayCache() {
	refreshCacheMu.Lock()
	refreshCacheAt = time.Time{}
	refreshCacheMu.Unlock()
}

// kscreenCommand adds graphical-session variables that may have reached the
// systemd user manager after socket activation started the daemon.
func kscreenCommand(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, kscreenBin, args...)
	cmd.Env = os.Environ()
	conn, err := dbus.SessionBus()
	if err != nil {
		return cmd
	}
	var environment dbus.Variant
	err = conn.Object(systemdBusName, systemdPath).
		CallWithContext(ctx, "org.freedesktop.DBus.Properties.Get", 0, systemdManager, "Environment").
		Store(&environment)
	if err != nil {
		return cmd
	}
	if values, ok := environment.Value().([]string); ok {
		cmd.Env = append(cmd.Env, values...)
	}
	return cmd
}

func rotationName(value int) string {
	switch value {
	case 1:
		return "none"
	case 2:
		return "left"
	case 4:
		return "inverted"
	case 8:
		return "right"
	default:
		return ""
	}
}

func suffixNonEmpty(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return ": " + s
}
