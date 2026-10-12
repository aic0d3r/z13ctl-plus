package api

// client.go — socket client for communicating with the z13ctl-plus daemon.
//
// Each Send* function connects to the daemon's Unix socket, sends one JSON
// command, and reads one JSON response. If the daemon is not running
// (connection refused), functions return (false, nil) so callers can fall
// back to direct hardware access.
//
// Subscribe opens a long-lived connection and returns a channel that receives
// event name strings streamed by the daemon.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

func runtimePath(name string) string {
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	if runtime == "" {
		runtime = "/tmp"
	}
	return runtime + "/z13ctl-plus/" + name
}

// SocketPath returns the runtime path for the daemon's Unix socket.
func SocketPath() string {
	return runtimePath("z13ctl-plus.sock")
}

// TabletSettingsPath returns the runtime path projected for z13-tablet-kit.
func TabletSettingsPath() string {
	return runtimePath("tablet-settings.json")
}

// request is a command sent to the daemon over the socket.
type request struct {
	Cmd               string             `json:"cmd"`
	Mode              string             `json:"mode,omitempty"`
	Color             string             `json:"color,omitempty"`  // "RRGGBB" hex
	Color2            string             `json:"color2,omitempty"` // "RRGGBB" hex
	Speed             string             `json:"speed,omitempty"`
	Brightness        int                `json:"brightness,omitempty"`
	Set               string             `json:"set,omitempty"`
	Device            string             `json:"device,omitempty"` // "keyboard", "lightbar", /dev/hidrawN; empty = all
	Events            []string           `json:"events,omitempty"`
	PL1               string             `json:"pl1,omitempty"`
	PL2               string             `json:"pl2,omitempty"`
	PL3               string             `json:"pl3,omitempty"`
	Force             bool               `json:"force,omitempty"`
	Name              string             `json:"name,omitempty"`
	Enabled           *bool              `json:"enabled,omitempty"`
	ACPreset          string             `json:"ac_preset,omitempty"`
	BatPreset         string             `json:"battery_preset,omitempty"`
	Posture           string             `json:"posture,omitempty"`
	Rotation          string             `json:"rotation,omitempty"`
	TabletSettings    *TabletSettings    `json:"tablet_settings,omitempty"`
	TabletIntegration *TabletIntegration `json:"tablet_integration,omitempty"`
	Preset            *Preset            `json:"preset,omitempty"`
}

// response is the reply to a command or a streamed event notification.
type response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Value string `json:"value,omitempty"`
	State *State `json:"state,omitempty"`
	Event string `json:"event,omitempty"`
}

// sendCommand connects to the daemon and sends req, returning the response.
// Returns (false, nil, nil) if the daemon is not running.
func sendCommand(req request) (bool, *response, error) {
	conn, err := net.DialTimeout("unix", SocketPath(), time.Second)
	if err != nil {
		return false, nil, nil // daemon not running
	}
	defer func() { _ = conn.Close() }()
	// Bound the whole exchange: a daemon stuck on a hardware write must not
	// hang the caller forever.
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	data, err := json.Marshal(req)
	if err != nil {
		return true, nil, err
	}
	if _, err := fmt.Fprintf(conn, "%s\n", data); err != nil {
		return true, nil, err
	}

	var resp response
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return true, nil, fmt.Errorf("no response from daemon")
	}
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return true, nil, err
	}
	return true, &resp, nil
}

func sendRequest(req request) (bool, error) {
	handled, resp, err := sendCommand(req)
	if !handled || err != nil {
		return handled, err
	}
	if !resp.OK {
		return true, fmt.Errorf("%s", resp.Error)
	}
	return true, nil
}

func sendValue(command string) (bool, string, error) {
	handled, resp, err := sendCommand(request{Cmd: command})
	if !handled || err != nil {
		return handled, "", err
	}
	if !resp.OK {
		return true, "", fmt.Errorf("%s", resp.Error)
	}
	return true, resp.Value, nil
}

func sendIntValue(command, label string) (handled bool, value int, err error) {
	handled, raw, err := sendValue(command)
	if !handled || err != nil {
		return handled, 0, err
	}
	if _, scanErr := fmt.Sscan(raw, &value); scanErr != nil {
		return true, 0, fmt.Errorf("invalid %s value %q: %w", label, raw, scanErr)
	}
	return true, value, nil
}

// SendApply sends an apply command to the daemon. color and color2 must be
// "RRGGBB" hex strings. device may be "keyboard", "lightbar", a /dev/hidrawN
// path, or "" to target all devices. Returns (true, nil) on success, (false,
// nil) if the daemon is not running (caller should fall back to direct HID access).
func SendApply(device, color, color2, mode, speed string, brightness int) (bool, error) {
	return sendRequest(request{
		Cmd:        "apply",
		Mode:       mode,
		Color:      color,
		Color2:     color2,
		Speed:      speed,
		Brightness: brightness,
		Device:     device,
	})
}

// SendOff sends an off command to the daemon. device may be "keyboard",
// "lightbar", a /dev/hidrawN path, or "" to target all devices.
func SendOff(device string) (bool, error) {
	return sendRequest(request{Cmd: "off", Device: device})
}

// SendBrightness sends a brightness-only command to the daemon. device may be
// "keyboard", "lightbar", a /dev/hidrawN path, or "" to target all devices.
func SendBrightness(device string, level int) (bool, error) {
	return sendRequest(request{Cmd: "brightness", Brightness: level, Device: device})
}

// SendProfileGet queries the daemon for the current performance profile by
// reading sysfs (not cached daemon state). Intended for GUI/plugin callers.
// Returns (false, "", nil) if the daemon is not running.
func SendProfileGet() (handled bool, profile string, err error) {
	return sendValue("profile-get")
}

// SendBatteryLimitGet queries the daemon for the current battery charge limit by
// reading sysfs (not cached daemon state). Intended for GUI/plugin callers.
// Returns (false, 0, nil) if the daemon is not running.
func SendBatteryLimitGet() (handled bool, limit int, err error) {
	return sendIntValue("batterylimit-get", "battery limit")
}

// SendProfileSet sends a profile set command to the daemon.
func SendProfileSet(profile string) (bool, error) {
	return sendRequest(request{Cmd: "profile", Set: profile})
}

// SendBatteryLimitSet sends a battery limit set command to the daemon.
func SendBatteryLimitSet(limit int) (bool, error) {
	return sendRequest(request{Cmd: "batterylimit", Set: fmt.Sprintf("%d", limit)})
}

// SendBootSoundSet sends a boot sound set command to the daemon.
func SendBootSoundSet(value int) (bool, error) {
	return sendRequest(request{Cmd: "bootsound", Set: fmt.Sprintf("%d", value)})
}

// SendPanelOverdriveSet sends a panel overdrive set command to the daemon.
func SendPanelOverdriveSet(value int) (bool, error) {
	return sendRequest(request{Cmd: "paneloverdrive", Set: fmt.Sprintf("%d", value)})
}

// SendCPUMinFrequencySet sets the minimum CPU frequency for every cpufreq policy.
func SendCPUMinFrequencySet(khz int) (bool, error) {
	return sendRequest(request{Cmd: "cpu-min", Set: strconv.Itoa(khz)})
}

// SendCPUEPPSet sets the AMD energy performance preference for every CPU policy.
func SendCPUEPPSet(value string) (bool, error) {
	return sendRequest(request{Cmd: "cpu-epp", Set: value})
}

// SendCPUBoostSet enables or disables CPU boost.
func SendCPUBoostSet(enabled bool) (bool, error) {
	value := "0"
	if enabled {
		value = "1"
	}
	return sendRequest(request{Cmd: "cpu-boost", Set: value})
}

// SendRefreshRateSet publishes an eDP-1 refresh request to the display
// coordinator. Application is asynchronous and waits for current posture.
func SendRefreshRateSet(hz int) (bool, error) {
	return sendRequest(request{Cmd: "refresh-rate", Set: strconv.Itoa(hz)})
}

// SendRefreshRateGet queries the daemon for the current eDP-1 refresh rate.
// Returns (false, 0, nil) if the daemon is not running.
func SendRefreshRateGet() (handled bool, hz int, err error) {
	return sendIntValue("refresh-rate-get", "refresh rate")
}

// SendBootSoundGet queries the daemon for the current boot sound setting by
// reading sysfs. Returns (false, 0, nil) if the daemon is not running.
func SendBootSoundGet() (handled bool, value int, err error) {
	return sendIntValue("bootsound-get", "boot sound")
}

// SendPanelOverdriveGet queries the daemon for the current panel overdrive
// setting by reading sysfs. Returns (false, 0, nil) if the daemon is not running.
func SendPanelOverdriveGet() (handled bool, value int, err error) {
	return sendIntValue("paneloverdrive-get", "panel overdrive")
}

// SendFanCurveGet queries the daemon for the current fan curve data.
// Returns JSON value string with both fans' curve and mode.
func SendFanCurveGet() (handled bool, value string, err error) {
	return sendValue("fancurve-get")
}

// SendFanCurveSet sends a fan curve set command to the daemon.
// The curve is applied to both fans simultaneously.
func SendFanCurveSet(curve string) (bool, error) {
	return sendRequest(request{Cmd: "fancurve", Set: curve})
}

// SendFanCurveReset resets both fans to firmware auto mode.
func SendFanCurveReset() (bool, error) {
	return sendRequest(request{Cmd: "fancurve-reset"})
}

// SendTdpGet queries the daemon for current TDP/PPT values.
// Returns JSON value string.
func SendTdpGet() (handled bool, value string, err error) {
	return sendValue("tdp-get")
}

// SendTdpSet sends a TDP set command to the daemon.
func SendTdpSet(watts string, pl1, pl2, pl3 string, force bool) (bool, error) {
	return sendRequest(request{
		Cmd:   "tdp",
		Set:   watts,
		PL1:   pl1,
		PL2:   pl2,
		PL3:   pl3,
		Force: force,
	})
}

// SendTdpReset sends a TDP reset command to the daemon.
func SendTdpReset() (bool, error) {
	return sendRequest(request{Cmd: "tdp-reset"})
}

// SendUndervoltGet queries the daemon for the current Curve Optimizer offsets.
// Returns JSON value string. Returns (false, "", nil) if the daemon is not running.
func SendUndervoltGet() (handled bool, value string, err error) {
	return sendValue("undervolt-get")
}

// SendUndervoltSet sends a Curve Optimizer set command to the daemon.
// cpu is a string representation of the CO offset (e.g. "-20").
func SendUndervoltSet(cpu string) (bool, error) {
	return sendRequest(request{Cmd: "undervolt", Set: cpu})
}

// SendUndervoltReset resets Curve Optimizer to stock (0).
func SendUndervoltReset() (bool, error) {
	return sendRequest(request{Cmd: "undervolt-reset"})
}

// SendTuningReset resets fan, TDP, and undervolt overrides while preserving
// the physical profile, CPU power, display, and battery settings.
func SendTuningReset() (bool, error) {
	return sendRequest(request{Cmd: "tuning-reset"})
}

// SendPresetSave saves the daemon's current performance and display state as name.
func SendPresetSave(name string) (bool, error) {
	return sendRequest(request{Cmd: "preset-save", Name: name})
}

// SendPresetSet creates or replaces name with explicit preset values. Fan
// curves are ignored. If name is the active preset it is re-applied.
func SendPresetSet(name string, preset Preset) (bool, error) {
	return sendRequest(request{Cmd: "preset-set", Name: name, Preset: &preset})
}

// SendPresetRename renames a preset, keeping its power-policy assignments.
func SendPresetRename(from, to string) (bool, error) {
	return sendRequest(request{Cmd: "preset-rename", Name: from, Set: to})
}

// SendPresetApply applies a named preset.
func SendPresetApply(name string) (bool, error) {
	return sendRequest(request{Cmd: "preset-apply", Name: name})
}

// SendPresetDelete deletes a named preset that is not assigned by the power policy.
func SendPresetDelete(name string) (bool, error) {
	return sendRequest(request{Cmd: "preset-delete", Name: name})
}

// SendRestoreRecommended restores and applies the recommended power presets.
func SendRestoreRecommended() (bool, error) {
	return sendRequest(request{Cmd: "restore-recommended"})
}

// SendPowerPolicySet configures automatic AC/battery preset switching. Empty
// preset names preserve the current assignments (or select recommended defaults).
func SendPowerPolicySet(enabled bool, acPreset, batteryPreset string) (bool, error) {
	return sendRequest(request{
		Cmd:       "power-policy-set",
		Enabled:   &enabled,
		ACPreset:  acPreset,
		BatPreset: batteryPreset,
	})
}

// SendDisplayPosture publishes the current desktop, laptop, or tablet posture
// to the daemon's display coordinator.
func SendDisplayPosture(posture string) (bool, error) {
	return sendRequest(request{Cmd: "display-posture", Posture: posture})
}

// SendDisplayRotation publishes the desired tablet rotation to the daemon's
// display coordinator.
func SendDisplayRotation(rotation string) (bool, error) {
	return sendRequest(request{Cmd: "display-rotation", Rotation: rotation})
}

// SendTabletSettingsSet atomically replaces all persisted tablet settings.
func SendTabletSettingsSet(settings TabletSettings) (bool, error) {
	return sendRequest(request{Cmd: "tablet-settings-set", TabletSettings: &settings})
}

// SendTabletIntegrationReport publishes ephemeral z13-tablet-kit status.
func SendTabletIntegrationReport(status TabletIntegration) (bool, error) {
	return sendRequest(request{Cmd: "tablet-integration-report", TabletIntegration: &status})
}

// SendGetState fetches the daemon's full cached state for GUI initialization.
// Returns (false, nil, nil) if the daemon is not running.
func SendGetState() (bool, *State, error) {
	handled, resp, err := sendCommand(request{Cmd: "get-state"})
	if !handled || err != nil {
		return handled, nil, err
	}
	if !resp.OK {
		return true, nil, fmt.Errorf("%s", resp.Error)
	}
	return true, resp.State, nil
}

// Subscribe opens a long-lived subscription to the daemon and returns a channel
// that receives event name strings (e.g. "gui-toggle") as they are streamed.
// The returned cancel func closes the underlying connection and stops the
// goroutine; the channel is closed when the connection drops or cancel is called.
// Returns (nil, nil, nil) if the daemon is not running.
func Subscribe(events []string) (<-chan string, func(), error) {
	conn, err := net.DialTimeout("unix", SocketPath(), time.Second)
	if err != nil {
		return nil, nil, nil // daemon not running
	}
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second)) // handshake only

	req := request{Cmd: "subscribe", Events: events}
	data, err := json.Marshal(req)
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	if _, err := fmt.Fprintf(conn, "%s\n", data); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	// Read the initial OK acknowledgement.
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("no response from daemon")
	}
	var ack response
	if err := json.Unmarshal(scanner.Bytes(), &ack); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	if !ack.OK {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("subscribe: %s", ack.Error)
	}
	// Long-lived connection: clear the handshake deadline.
	_ = conn.SetDeadline(time.Time{})

	ch := make(chan string, 8)
	go func() {
		defer close(ch)
		for scanner.Scan() {
			var ev response
			if err := json.Unmarshal(scanner.Bytes(), &ev); err == nil && ev.Event != "" {
				ch <- ev.Event
			}
		}
	}()

	cancel := func() { _ = conn.Close() }
	return ch, cancel, nil
}
