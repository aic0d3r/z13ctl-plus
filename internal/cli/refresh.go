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
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
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

// refreshCacheTTL bounds how stale a cached refresh-rate read may be.
const refreshCacheTTL = time.Second

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
// call. Results are cached for one second: callers poll this per get-state
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
	if out, err := kscreenCommand(ctx, "-o").CombinedOutput(); err == nil {
		summary = summarizeDisplays(out)
	}

	refreshCacheMu.Lock()
	defer refreshCacheMu.Unlock()
	refreshCacheAt = time.Now()
	refreshCacheSummary = summary
	return summary
}

// summarizeDisplays parses kscreen-doctor -o output into a DisplaySummary.
func summarizeDisplays(out []byte) DisplaySummary {
	var summary DisplaySummary
	if panel, ok := parseInternalDisplay(out); ok {
		if panel.Enabled {
			summary.Refresh = panel.Refresh
		} else {
			summary.PanelOff = true
		}
	}
	out = ansiRe.ReplaceAll(out, nil)
	for _, block := range outputRe.Split(string(out), -1)[1:] {
		// A block starts with "<id> <name> <uuid>".
		if fields := strings.Fields(block); len(fields) > 1 && fields[1] != outputName && enabledRe.MatchString(block) {
			summary.External = true
		}
	}
	return summary
}

var (
	ansiRe    = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	outputRe  = regexp.MustCompile(`(?m)^Output:`)
	enabledRe = regexp.MustCompile(`(?m)^\s*enabled\s*$`)
)

// ReadInternalDisplay returns the enabled, mode, scale, and rotation state of eDP-1.
func ReadInternalDisplay() (InternalDisplayState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
	defer cancel()
	out, err := kscreenCommand(ctx, "-o").CombinedOutput()
	if err != nil {
		return InternalDisplayState{}, err
	}
	state, ok := parseInternalDisplay(out)
	if !ok {
		return InternalDisplayState{}, fmt.Errorf("%s output not found", outputName)
	}
	return state, nil
}

// parseInternalDisplay extracts the eDP-1 state and current mode.
func parseInternalDisplay(out []byte) (InternalDisplayState, bool) {
	out = ansiRe.ReplaceAll(out, nil)
	// kscreen-doctor marks the active mode with a trailing "*". Example:
	//   Modes:  1:2560x1600@180.00*  2:2560x1600@60.00  ...
	// We look for "@<digits>.<digits>*" inside a Modes: block.
	// To stay robust if multiple outputs are connected, restrict to the
	// eDP-1 output block.
	re := regexp.MustCompile(`(?m)^Output:.*\b` + outputName + `\b.*\n(?s:.*?)(?:\nOutput:|\z)`)
	block := re.Find(out)
	if block == nil {
		return InternalDisplayState{}, false
	}
	state := InternalDisplayState{Enabled: enabledRe.Match(block)}
	rateRe := regexp.MustCompile(`@(\d+)\.\d+\*`)
	m := rateRe.FindSubmatch(block)
	if m != nil {
		state.Refresh, _ = strconv.Atoi(string(m[1]))
	}
	if m := regexp.MustCompile(`(?m)^\s*Scale:\s*([0-9.]+)\s*$`).FindSubmatch(block); m != nil {
		state.Scale, _ = strconv.ParseFloat(string(m[1]), 64)
	}
	if m := regexp.MustCompile(`(?m)^\s*Rotation:\s*(\d+)\s*$`).FindSubmatch(block); m != nil {
		state.Rotation = rotationName(string(m[1]))
	}
	return state, true
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

func rotationName(value string) string {
	switch value {
	case "1":
		return "none"
	case "2":
		return "left"
	case "4":
		return "inverted"
	case "8":
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
