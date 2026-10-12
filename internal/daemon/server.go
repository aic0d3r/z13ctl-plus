package daemon

// server.go — incoming socket connection handler and command dispatch.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aic0d3r/z13ctl-plus/internal/aura"
	"github.com/aic0d3r/z13ctl-plus/internal/cli"
	"github.com/aic0d3r/z13ctl-plus/internal/hid"
	"github.com/dahui/z13ctl/api"
)

// request and response mirror the unexported types in api/client.go.
// They are intentionally duplicated: the api module is a separate Go module
// (stdlib-only), so the daemon cannot share types with it across the module
// boundary without making them public. Both sides must stay in sync with the
// JSON wire protocol.

// request is a command sent by a client over the socket.
// One JSON object per newline-terminated message.
type request struct {
	Cmd               string                 `json:"cmd"`
	Mode              string                 `json:"mode,omitempty"`
	Color             string                 `json:"color,omitempty"`  // "RRGGBB" hex
	Color2            string                 `json:"color2,omitempty"` // "RRGGBB" hex
	Speed             string                 `json:"speed,omitempty"`
	Brightness        int                    `json:"brightness,omitempty"`
	Set               string                 `json:"set,omitempty"`
	Device            string                 `json:"device,omitempty"` // "keyboard", "lightbar", /dev/hidrawN; empty = all
	Events            []string               `json:"events,omitempty"`
	PL1               string                 `json:"pl1,omitempty"`
	PL2               string                 `json:"pl2,omitempty"`
	PL3               string                 `json:"pl3,omitempty"`
	Force             bool                   `json:"force,omitempty"`
	Name              string                 `json:"name,omitempty"`
	Enabled           *bool                  `json:"enabled,omitempty"`
	ACPreset          string                 `json:"ac_preset,omitempty"`
	BatPreset         string                 `json:"battery_preset,omitempty"`
	Posture           string                 `json:"posture,omitempty"`
	Rotation          string                 `json:"rotation,omitempty"`
	TabletSettings    *api.TabletSettings    `json:"tablet_settings,omitempty"`
	TabletIntegration *api.TabletIntegration `json:"tablet_integration,omitempty"`
	Preset            *api.Preset            `json:"preset,omitempty"`
}

// response is the reply to a command or a streamed event notification.
type response struct {
	OK    bool       `json:"ok"`
	Error string     `json:"error,omitempty"`
	Value string     `json:"value,omitempty"`
	State *api.State `json:"state,omitempty"`
	Event string     `json:"event,omitempty"`
}

// handleConn reads one JSON request, dispatches it, and writes one JSON response.
// For "subscribe" requests the connection is kept open for event streaming.
func (d *Daemon) handleConn(conn net.Conn) {
	// Bound the time a silent or stuck client can hold a connection; the
	// subscribe path clears the deadline once the handshake completes.
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		_ = conn.Close()
		return
	}

	var req request
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		writeResponse(conn, response{OK: false, Error: "invalid JSON: " + err.Error()})
		_ = conn.Close()
		return
	}

	if req.Cmd == "subscribe" {
		// Acknowledge and keep connection open for event streaming.
		writeResponse(conn, response{OK: true})
		_ = conn.SetReadDeadline(time.Time{})
		d.addSubscriber(conn)
		return
	}

	resp := d.dispatch(req)
	if !resp.OK {
		slog.Warn("command failed", "cmd", req.Cmd, "err", resp.Error)
	}
	writeResponse(conn, resp)
	_ = conn.Close()
}

func (d *Daemon) dispatch(req request) response {
	if commandNeedsApplyLock(req.Cmd) {
		d.applyMu.Lock()
		defer d.applyMu.Unlock()
	}
	switch req.Cmd {
	case "apply":
		return d.handleApply(req)
	case "off":
		return d.handleOff(req)
	case "brightness":
		return d.handleBrightness(req)
	case "profile":
		return d.handleProfile(req)
	case "profile-get":
		return handleProfileGet()
	case "batterylimit":
		return d.handleBatteryLimit(req)
	case "batterylimit-get":
		return handleBatteryLimitGet()
	case "bootsound":
		return handleBootSound(req)
	case "bootsound-get":
		return handleBootSoundGet()
	case "paneloverdrive":
		return d.handlePanelOverdrive(req)
	case "paneloverdrive-get":
		return handlePanelOverdriveGet()
	case "cpu-min":
		return d.handleCPUMin(req)
	case "cpu-epp":
		return d.handleCPUEPP(req)
	case "cpu-boost":
		return d.handleCPUBoost(req)
	case "refresh-rate":
		return d.handleRefreshRate(req)
	case "refresh-rate-get":
		return handleRefreshRateGet()
	case "fancurve":
		return d.handleFanCurve(req)
	case "fancurve-get":
		return handleFanCurveGet()
	case "fancurve-reset":
		return d.handleFanCurveReset()
	case "tdp":
		return d.handleTDP(req)
	case "tdp-get":
		return d.handleTDPGet()
	case "tdp-reset":
		return d.handleTDPReset()
	case "undervolt":
		return d.handleUndervolt(req)
	case "undervolt-get":
		return d.handleUndervoltGet()
	case "undervolt-reset":
		return d.handleUndervoltReset()
	case "tuning-reset":
		return d.handleTuningReset()
	case "preset-save":
		return responseFromError(d.savePreset(req.Name))
	case "preset-set":
		if req.Preset == nil {
			return response{OK: false, Error: "preset-set requires a preset"}
		}
		return responseFromError(d.setPreset(req.Name, *req.Preset))
	case "preset-rename":
		return responseFromError(d.renamePreset(req.Name, req.Set))
	case "preset-apply":
		return responseFromError(d.applyPreset(req.Name))
	case "preset-delete":
		return responseFromError(d.deletePreset(req.Name))
	case "restore-recommended":
		return responseFromError(d.restoreRecommended())
	case "power-policy-set":
		if req.Enabled == nil {
			return response{OK: false, Error: "power policy requires enabled"}
		}
		return responseFromError(d.setPowerPolicy(*req.Enabled, req.ACPreset, req.BatPreset))
	case "display-posture":
		return responseFromError(d.setDisplayPosture(req.Posture))
	case "display-rotation":
		return responseFromError(d.setDisplayRotation(req.Rotation))
	case "tablet-settings-set":
		return d.handleTabletSettings(req)
	case "tablet-integration-report":
		return d.handleTabletIntegration(req)
	case "get-state":
		d.mu.Lock()
		s := d.state
		s.FanCurve = cloneFanCurve(d.state.FanCurve)
		s.TDP = clonePtr(d.state.TDP)
		s.Undervolt = clonePtr(d.state.Undervolt)
		s.Presets = clonePresetMap(d.state.Presets)
		s.Devices = maps.Clone(d.state.Devices)
		s.PowerPolicy = clonePtr(d.state.PowerPolicy)
		s.TabletIntegration = clonePtr(d.state.TabletIntegration)
		d.mu.Unlock()
		s.DevicePresence = map[string]bool{
			"keyboard": hid.HasDevice("keyboard"),
			"lightbar": hid.HasDevice("lightbar"),
		}
		// Populate firmware-managed fields from sysfs (not cached in daemon state).
		s.BootSound = readIntSysfs(cli.FindBootSoundPath())
		s.PanelOverdrive = readIntSysfs(cli.FindPanelOverdrivePath())
		if cpuPower, err := cli.ReadCPUPower(); err == nil {
			s.CPUPower = &cpuPower
		}
		if profile := readProfileFromSysfs(); isPhysicalProfile(profile) {
			s.Profile = profile
		}
		plan := planTuning(s)
		s.FanSafetyActive = plan.FanSafety
		if !s.FanCurveActive {
			s.FanCurve = readFanCurveFromSysfs()
		}
		if !s.TDPActive {
			if tdp, err := readStockOrEffectivePPT(s.Profile); err == nil {
				s.TDP = &tdp
			}
		}
		if s.Undervolt != nil {
			s.Undervolt.Active = s.UndervoltActive
		} else {
			s.Undervolt = &api.UndervoltState{}
		}
		// Indicate whether undervolt is available (ryzen_smu loaded + commands work).
		s.UndervoltAvailable = cli.SMUProbeUndervolt()
		// Populate APU temperature and fan RPM from sysfs.
		if temp, err := cli.ReadAPUTemperature(); err == nil {
			s.Temperature = temp
		}
		if rpms, err := cli.ReadBothFanRPM(); err == nil {
			s.FanRPM = rpms[0]
		}
		// Populate live telemetry (CPU/GPU temp, util, clocks, VRAM, mem clock).
		s.Telemetry = cli.ReadTelemetry()
		// Populate eDP-1 refresh rate from kscreen-doctor.
		s.RefreshRate = cli.ReadRefreshRate()
		// Populate battery detail (health, power draw, status).
		if bat, err := cli.ReadBatteryState(); err == nil {
			s.BatteryDetail = bat
		}
		d.mu.Lock()
		s.PowerSource = d.powerSource
		d.mu.Unlock()
		return response{OK: true, State: &s}
	default:
		return response{OK: false, Error: "unknown command: " + req.Cmd}
	}
}

func validateTabletSettings(settings api.TabletSettings) error {
	if settings.ScrollSensitivity < 1 || settings.ScrollSensitivity > 5 {
		return fmt.Errorf("scroll sensitivity must be between 1 and 5")
	}
	if settings.ScrollSpeed < 1 || settings.ScrollSpeed > 5 {
		return fmt.Errorf("scroll speed must be between 1 and 5")
	}
	return nil
}

func validateTabletIntegration(status api.TabletIntegration) error {
	switch status.Posture {
	case "desktop", "laptop", "tablet":
		return nil
	default:
		return fmt.Errorf("posture must be desktop, laptop, or tablet")
	}
}

func (d *Daemon) handleTabletSettings(req request) response {
	if req.TabletSettings == nil {
		return response{OK: false, Error: "tablet settings are required"}
	}
	settings := *req.TabletSettings
	if err := validateTabletSettings(settings); err != nil {
		return responseFromError(err)
	}

	d.mu.Lock()
	next := d.state
	next.TabletSettings = settings
	previous := d.state.TabletSettings
	if err := projectTabletSettings(settings); err != nil {
		d.mu.Unlock()
		return response{OK: false, Error: "project tablet settings: " + err.Error()}
	}
	if err := saveState(next); err != nil {
		_ = projectTabletSettings(previous)
		d.mu.Unlock()
		return response{OK: false, Error: "save tablet settings: " + err.Error()}
	}
	d.state.TabletSettings = settings
	d.mu.Unlock()

	d.broadcast(response{Event: "state-changed"})
	return response{OK: true}
}

func (d *Daemon) handleTabletIntegration(req request) response {
	if req.TabletIntegration == nil {
		return response{OK: false, Error: "tablet integration status is required"}
	}
	status := *req.TabletIntegration
	if err := validateTabletIntegration(status); err != nil {
		return responseFromError(err)
	}

	d.mu.Lock()
	d.state.TabletIntegration = &status
	d.mu.Unlock()
	d.broadcast(response{Event: "state-changed"})
	return response{OK: true}
}

func responseFromError(err error) response {
	if err != nil {
		return response{OK: false, Error: err.Error()}
	}
	return response{OK: true}
}

func commandNeedsApplyLock(cmd string) bool {
	switch cmd {
	case "apply", "off", "brightness", "profile", "batterylimit", "bootsound",
		"paneloverdrive", "cpu-min", "cpu-epp", "cpu-boost", "refresh-rate",
		"fancurve", "fancurve-reset", "tdp", "tdp-reset", "undervolt", "undervolt-reset",
		"tuning-reset", "preset-save":
		return true
	default:
		return false
	}
}

func (d *Daemon) handleCPUMin(req request) response {
	khz, err := strconv.Atoi(req.Set)
	if err != nil {
		return response{OK: false, Error: "CPU minimum frequency must be an integer in kHz"}
	}
	if err := cli.SetCPUMinFrequency(khz); err != nil {
		return response{OK: false, Error: err.Error()}
	}
	d.clearActivePreset()
	return response{OK: true}
}

func (d *Daemon) handleCPUEPP(req request) response {
	if err := cli.SetCPUEPP(req.Set); err != nil {
		return response{OK: false, Error: err.Error()}
	}
	d.clearActivePreset()
	return response{OK: true}
}

func (d *Daemon) handleCPUBoost(req request) response {
	value, err := strconv.Atoi(req.Set)
	if err != nil || (value != 0 && value != 1) {
		return response{OK: false, Error: "CPU boost must be 0 or 1"}
	}
	if err := cli.SetCPUBoost(value == 1); err != nil {
		return response{OK: false, Error: err.Error()}
	}
	d.clearActivePreset()
	return response{OK: true}
}

// handleRefreshRateGet reads the current eDP-1 refresh rate via kscreen-doctor.
// Returns the integer Hz value (60 or 180) or 0 if unavailable.
func handleRefreshRateGet() response {
	hz := cli.ReadRefreshRate()
	return response{OK: true, Value: strconv.Itoa(hz)}
}

// handleRefreshRate switches eDP-1 to the requested refresh rate.
func (d *Daemon) handleRefreshRate(req request) response {
	hz, err := strconv.Atoi(req.Set)
	if err != nil {
		return response{OK: false, Error: "refresh rate must be an integer in Hz"}
	}
	if err := d.setDisplayRefresh(hz); err != nil {
		return response{OK: false, Error: err.Error()}
	}
	slog.Info("refresh-rate", "hz", hz)
	d.clearActivePreset()
	return response{OK: true}
}

// handleProfileGet reads the current performance profile from sysfs.
// Reading from sysfs (not daemon state) ensures accurate values even if
// the profile was changed by another process.
func handleProfileGet() response {
	value, err := cli.ReadTrimmedFile(cli.FindProfilePath())
	if err != nil {
		return response{OK: false, Error: "reading profile: " + err.Error()}
	}
	return response{OK: true, Value: value}
}

// handleBatteryLimitGet reads the current battery charge limit from sysfs.
func handleBatteryLimitGet() response {
	value, err := cli.ReadTrimmedFile(cli.FindBatteryThresholdPath())
	if err != nil {
		return response{OK: false, Error: "reading battery limit: " + err.Error()}
	}
	return response{OK: true, Value: value}
}

func (d *Daemon) handleApply(req request) response {
	mode, err := aura.ModeFromString(req.Mode)
	if err != nil {
		return response{OK: false, Error: "mode: " + err.Error()}
	}
	speed, err := aura.SpeedFromString(req.Speed)
	if err != nil {
		return response{OK: false, Error: "speed: " + err.Error()}
	}
	r, g, b, err := cli.ParseColor(req.Color)
	if err != nil {
		return response{OK: false, Error: "color: " + err.Error()}
	}
	r2, g2, b2, err := cli.ParseColor(req.Color2)
	if err != nil {
		return response{OK: false, Error: "color2: " + err.Error()}
	}
	if req.Brightness < 0 || req.Brightness > 3 {
		return response{OK: false, Error: fmt.Sprintf("brightness %d out of range 0–3", req.Brightness)}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.dev == nil {
		return response{OK: false, Error: "no HID device available"}
	}
	target, err := d.dev.FilteredView(req.Device)
	if err != nil {
		return response{OK: false, Error: err.Error()}
	}
	if err := aura.Apply(target, mode, r, g, b, r2, g2, b2, speed, uint8(req.Brightness)); err != nil {
		return response{OK: false, Error: "apply: " + err.Error()}
	}
	device := req.Device
	if device == "" {
		device = "all"
	}
	slog.Info("apply", "device", device, "mode", req.Mode, "color", req.Color, "brightness", req.Brightness)
	ls := api.LightingState{
		Enabled:    true,
		Mode:       req.Mode,
		Color:      req.Color,
		Color2:     req.Color2,
		Speed:      req.Speed,
		Brightness: req.Brightness,
	}
	if req.Device == "" {
		// All-device apply: update canonical state and clear per-device overrides.
		d.state.Lighting = ls
		d.state.Devices = nil
	} else if !strings.HasPrefix(req.Device, "/") {
		// Named per-device apply (keyboard/lightbar): save as a per-device override.
		if d.state.Devices == nil {
			d.state.Devices = make(map[string]api.LightingState)
		}
		d.state.Devices[req.Device] = ls
	}
	// Raw /dev/hidrawN paths are transient; not persisted.
	if req.Device == "" || !strings.HasPrefix(req.Device, "/") {
		if err := saveState(d.state); err != nil {
			slog.Warn("failed to save state", "err", err)
		}
	}
	return response{OK: true}
}

func (d *Daemon) handleOff(req request) response {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.dev == nil {
		return response{OK: false, Error: "no HID device available"}
	}
	target, err := d.dev.FilteredView(req.Device)
	if err != nil {
		return response{OK: false, Error: err.Error()}
	}
	if err := aura.TurnOff(target); err != nil {
		return response{OK: false, Error: "off: " + err.Error()}
	}
	if req.Device != "" {
		slog.Info("off", "device", req.Device)
		if !strings.HasPrefix(req.Device, "/") {
			// Named per-device off: save disabled state for this zone.
			if d.state.Devices == nil {
				d.state.Devices = make(map[string]api.LightingState)
			}
			d.state.Devices[req.Device] = api.LightingState{Enabled: false}
			if err := saveState(d.state); err != nil {
				slog.Warn("failed to save state", "err", err)
			}
		}
	} else {
		slog.Info("off")
		d.state.Lighting.Enabled = false
		d.state.Devices = nil
		if err := saveState(d.state); err != nil {
			slog.Warn("failed to save state", "err", err)
		}
	}
	return response{OK: true}
}

func (d *Daemon) handleBrightness(req request) response {
	if req.Brightness < 0 || req.Brightness > 3 {
		return response{OK: false, Error: fmt.Sprintf("brightness %d out of range 0–3", req.Brightness)}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.dev == nil {
		return response{OK: false, Error: "no HID device available"}
	}
	target, err := d.dev.FilteredView(req.Device)
	if err != nil {
		return response{OK: false, Error: err.Error()}
	}
	if err := aura.Init(target); err != nil {
		return response{OK: false, Error: "init: " + err.Error()}
	}
	on := req.Brightness > 0
	if err := aura.SetPower(target, on); err != nil {
		return response{OK: false, Error: "setpower: " + err.Error()}
	}
	if err := aura.SetBrightness(target, uint8(req.Brightness)); err != nil {
		return response{OK: false, Error: "brightness: " + err.Error()}
	}
	logArgs := []any{"level", req.Brightness}
	if req.Device != "" {
		logArgs = append(logArgs, "device", req.Device)
	}
	slog.Info("brightness", logArgs...)
	if req.Device == "" {
		d.state.Lighting.Brightness = req.Brightness
		d.state.Lighting.Enabled = on
		// Update per-device overrides too; leaving them stale would
		// resurrect old settings on the next restore.
		for name, ls := range d.state.Devices {
			if ls.Mode == "" {
				ls = d.state.Lighting // pure-off entry: rebuild from the base state
			}
			ls.Brightness = req.Brightness
			ls.Enabled = on
			d.state.Devices[name] = ls
		}
		if err := saveState(d.state); err != nil {
			slog.Warn("failed to save state", "err", err)
		}
	} else if !strings.HasPrefix(req.Device, "/") {
		// Named per-device brightness: update or create entry, preserving other fields.
		if d.state.Devices == nil {
			d.state.Devices = make(map[string]api.LightingState)
		}
		ls := d.state.Lighting // base: fall back to all-device state
		if existing, ok := d.state.Devices[req.Device]; ok && existing.Mode != "" {
			ls = existing // a pure-off entry (Mode "") is not restorable; use the base
		}
		ls.Brightness = req.Brightness
		ls.Enabled = on
		d.state.Devices[req.Device] = ls
		if err := saveState(d.state); err != nil {
			slog.Warn("failed to save state", "err", err)
		}
	}
	return response{OK: true}
}

func (d *Daemon) handleProfile(req request) response {
	if req.Set == "" {
		return response{OK: false, Error: "profile requires a set field"}
	}

	profile := strings.ToLower(req.Set)
	if !isPhysicalProfile(profile) {
		return response{OK: false, Error: "profile must be quiet, balanced, or performance"}
	}

	d.mu.Lock()
	next := d.state
	d.mu.Unlock()
	next.Profile = profile
	if err := applyTuning(next, true); err != nil {
		if profileAppliedBeforeError(err) {
			_ = d.commitTuningState(next, "")
		}
		return response{OK: false, Error: err.Error()}
	}
	slog.Info("profile", "set", profile)
	if err := d.commitTuningState(next, ""); err != nil {
		slog.Warn("failed to save state", "err", err)
	}
	return response{OK: true}
}

func (d *Daemon) handleBatteryLimit(req request) response {
	limit, err := strconv.Atoi(req.Set)
	if err != nil || limit < 40 || limit > 100 {
		return response{OK: false, Error: "battery limit must be an integer 40–100"}
	}
	if err := os.WriteFile(cli.FindBatteryThresholdPath(), []byte(req.Set+"\n"), 0o644); err != nil {
		return response{OK: false, Error: "batterylimit: " + err.Error()}
	}
	slog.Info("batterylimit", "set", limit)
	d.mu.Lock()
	d.state.Battery = limit
	d.mu.Unlock()
	if err := d.persistState(); err != nil {
		slog.Warn("failed to save state", "err", err)
	}
	return response{OK: true}
}

func handleBootSoundGet() response {
	value, err := cli.ReadTrimmedFile(cli.FindBootSoundPath())
	if err != nil {
		return response{OK: false, Error: "reading boot sound: " + err.Error()}
	}
	return response{OK: true, Value: value}
}

func handleBootSound(req request) response {
	value, err := strconv.Atoi(req.Set)
	if err != nil || (value != 0 && value != 1) {
		return response{OK: false, Error: "boot sound must be 0 or 1"}
	}
	if err := cli.SetBootSound(value); err != nil {
		return response{OK: false, Error: "bootsound: " + err.Error()}
	}
	slog.Info("bootsound", "set", value)
	return response{OK: true}
}

func handlePanelOverdriveGet() response {
	value, err := cli.ReadTrimmedFile(cli.FindPanelOverdrivePath())
	if err != nil {
		return response{OK: false, Error: "reading panel overdrive: " + err.Error()}
	}
	return response{OK: true, Value: value}
}

func (d *Daemon) handlePanelOverdrive(req request) response {
	value, err := strconv.Atoi(req.Set)
	if err != nil || (value != 0 && value != 1) {
		return response{OK: false, Error: "panel overdrive must be 0 or 1"}
	}
	if err := cli.SetPanelOverdrive(value); err != nil {
		return response{OK: false, Error: "paneloverdrive: " + err.Error()}
	}
	slog.Info("paneloverdrive", "set", value)
	d.mu.Lock()
	d.state.PanelOverdrive = value
	d.state.ActivePreset = ""
	d.mu.Unlock()
	if err := d.persistState(); err != nil {
		slog.Warn("failed to save state", "err", err)
	}
	return response{OK: true}
}

// readIntSysfs parses a sysfs integer, returning 0 on any error.
func readIntSysfs(path string) int {
	value, _ := cli.ReadTrimmedFile(path)
	v, _ := strconv.Atoi(value)
	return v
}

// handleFanCurveGet reads the current fan curve from sysfs (both fans).
func handleFanCurveGet() response {
	fc := readFanCurveFromSysfs()
	if fc == nil {
		return response{OK: false, Error: "failed to read fan curve from sysfs"}
	}
	data, _ := json.Marshal(fc)
	return response{OK: true, Value: string(data)}
}

// readFanCurveFromSysfs reads the fan curve and mode from sysfs.
// Returns fan 1's curve (both fans share the same curve).
func readFanCurveFromSysfs() *api.FanCurveState {
	modes, modeErr := cli.ReadBothFanModes()
	curves, curveErr := cli.ReadBothFanCurves()
	if modeErr != nil && curveErr != nil {
		return nil
	}
	mode := 0
	if modeErr == nil {
		mode = modes[0]
	}
	var points []api.FanCurvePoint
	if curveErr == nil {
		points = curves[0]
	}
	return &api.FanCurveState{Mode: mode, Points: points}
}

func (d *Daemon) handleFanCurve(req request) response {
	points, err := cli.ParseFanCurve(req.Set)
	if err != nil {
		return response{OK: false, Error: "fancurve: " + err.Error()}
	}
	d.mu.Lock()
	next := d.state
	d.mu.Unlock()
	next.FanCurve = &api.FanCurveState{Mode: 1, Points: points}
	next.FanCurveActive = true
	if plan := planTuning(next); plan.Fan != nil {
		// plan.Fan is the user curve when it meets the high-TDP floor,
		// otherwise the safety curve — write whichever applies.
		if err := cli.SetBothFanCurves(plan.Fan.Points); err != nil {
			return response{OK: false, Error: "fancurve: " + err.Error()}
		}
	}
	slog.Info("fancurve", "fans", "both")
	d.mu.Lock()
	d.state.FanCurve = &api.FanCurveState{Mode: 1, Points: points}
	d.state.FanCurveActive = true
	d.mu.Unlock()
	if err := d.persistState(); err != nil {
		slog.Warn("failed to save state", "err", err)
	}
	return response{OK: true}
}

func (d *Daemon) handleFanCurveReset() response {
	d.mu.Lock()
	next := d.state
	next.FanCurve = nil
	next.FanCurveActive = false
	plan := planTuning(next)
	d.mu.Unlock()
	if !plan.FanSafety {
		var err error
		if plan.Fan != nil {
			err = cli.SetBothFanCurves(plan.Fan.Points)
		} else {
			err = cli.ResetAllFanCurves()
		}
		if err != nil {
			return response{OK: false, Error: "fancurve-reset: " + err.Error()}
		}
	}
	slog.Info("fancurve-reset", "fans", "both")
	d.mu.Lock()
	d.state.FanCurve = nil
	d.state.FanCurveActive = false
	s := d.state
	d.mu.Unlock()
	if err := saveState(s); err != nil {
		slog.Warn("failed to save state", "err", err)
	}
	return response{OK: true}
}

func (d *Daemon) handleTDPGet() response {
	d.mu.Lock()
	active := d.state.TDPActive
	d.mu.Unlock()
	profile := readProfileFromSysfs()
	read := cli.ReadEffectivePPT
	if !active {
		read = readStockOrEffectivePPT
	}
	tdp, err := read(profile)
	if err != nil {
		return response{OK: false, Error: "reading TDP: " + err.Error()}
	}
	data, _ := json.Marshal(tdp)
	return response{OK: true, Value: string(data)}
}

// readStockOrEffectivePPT reports firmware PPT when no TDP override is active.
// asus-nb-wmi ppt_* only cache the last write and keep it across profile
// writes and tdp-reset, so they would report a stale override as live.
func readStockOrEffectivePPT(profile string) (api.TDPState, error) {
	if stock, ok := cli.StockProfilePPT[profile]; ok {
		return stock, nil
	}
	return cli.ReadEffectivePPT(profile)
}

// readProfileFromSysfs reads the current platform_profile value.
func readProfileFromSysfs() string {
	value, _ := cli.ReadTrimmedFile(cli.FindProfilePath())
	return value
}

func (d *Daemon) handleTDP(req request) response {
	base := req.Set
	if base == "" {
		base = req.PL1
	}
	watts, err := strconv.Atoi(base)
	if err != nil {
		return response{OK: false, Error: "TDP value must be an integer"}
	}

	pl1, pl2, pl3 := watts, watts, watts
	if req.PL1 != "" {
		if pl1, err = strconv.Atoi(req.PL1); err != nil {
			return response{OK: false, Error: "invalid pl1 value"}
		}
	}
	if req.PL2 != "" {
		if pl2, err = strconv.Atoi(req.PL2); err != nil {
			return response{OK: false, Error: "invalid pl2 value"}
		}
	}
	if req.PL3 != "" {
		if pl3, err = strconv.Atoi(req.PL3); err != nil {
			return response{OK: false, Error: "invalid pl3 value"}
		}
	}

	// PL1 (sustained) requires force flag above 75W. PL2/PL3 (burst) allowed up to hardware max.
	if err := cli.ValidateTDP(pl1, pl2, pl3, req.Force); err != nil {
		return response{OK: false, Error: err.Error()}
	}

	d.mu.Lock()
	old := d.state
	next := old
	oldSafety := planTuning(old).FanSafety
	d.mu.Unlock()
	next.TDP = &api.TDPState{
		PL1SPL: pl1, PL2SPPT: pl2, FPPT: pl3, APUSPPT: pl2, PlatformSPPT: pl2,
	}
	next.TDPActive = true
	plan := planTuning(next)
	if plan.FanSafety {
		if err := cli.SetBothFanCurves(plan.Fan.Points); err != nil {
			return response{OK: false, Error: "cannot set high-TDP fan curve: " + err.Error()}
		}
	}
	if err := cli.SetTDP(watts, pl1, pl2, pl3); err != nil {
		if plan.FanSafety && !oldSafety {
			oldPlan := planTuning(old)
			if oldPlan.Fan != nil {
				_ = cli.SetBothFanCurves(oldPlan.Fan.Points)
			} else {
				_ = cli.ResetAllFanCurves()
			}
		}
		// Reapply the previous known power state: reading sysfs back would
		// return the kernel's stale 5 W cache.
		if old.TDPActive && old.TDP != nil {
			_ = cli.SetTDP(0, old.TDP.PL1SPL, old.TDP.PL2SPPT, old.TDP.FPPT)
		} else {
			_ = cli.SetProfile(readProfileFromSysfs())
		}
		return response{OK: false, Error: "tdp: " + err.Error()}
	}
	if oldSafety && !plan.FanSafety {
		if plan.Fan != nil {
			if err := cli.SetBothFanCurves(plan.Fan.Points); err != nil {
				return response{OK: false, Error: "TDP is safe but desired fan curve could not be restored: " + err.Error()}
			}
		} else if err := cli.ResetAllFanCurves(); err != nil {
			return response{OK: false, Error: "TDP is safe but automatic fan control could not be restored: " + err.Error()}
		}
	}
	d.mu.Lock()
	d.state.TDP = clonePtr(next.TDP)
	d.state.TDPActive = true
	d.state.ActivePreset = ""
	d.mu.Unlock()
	if err := d.persistState(); err != nil {
		slog.Warn("failed to save state", "err", err)
	}
	slog.Info("tdp", "pl1", pl1, "pl2", pl2, "pl3", pl3)

	return response{OK: true}
}

func (d *Daemon) handleTDPReset() response {
	d.mu.Lock()
	next := d.state
	d.mu.Unlock()
	next.TDP = nil
	next.TDPActive = false
	if err := applyTuning(next, true); err != nil {
		if profileAppliedBeforeError(err) {
			_ = d.commitTuningState(next, "")
		}
		return response{OK: false, Error: "tdp-reset: " + err.Error()}
	}
	slog.Info("tdp-reset", "profile", next.Profile)
	if err := d.commitTuningState(next, ""); err != nil {
		slog.Warn("failed to save state", "err", err)
	}
	return response{OK: true}
}

func (d *Daemon) handleUndervoltGet() response {
	if !cli.SMUProbeUndervolt() {
		return response{OK: false, Error: "Curve Optimizer not available — ryzen_smu module missing or does not support this platform"}
	}
	d.mu.Lock()
	uv := d.state.Undervolt
	active := d.state.UndervoltActive
	profile := d.state.Profile
	d.mu.Unlock()

	uvState := api.UndervoltState{}
	if uv != nil {
		uvState = *uv
	}
	uvState.Active = active
	data, _ := json.Marshal(struct {
		api.UndervoltState
		Profile string `json:"profile"`
	}{uvState, profile})
	return response{OK: true, Value: string(data)}
}

func (d *Daemon) handleUndervolt(req request) response {
	if !cli.SMUProbeUndervolt() {
		return response{OK: false, Error: "Curve Optimizer not available — ryzen_smu module missing or does not support this platform"}
	}

	cpuOffset := 0
	if req.Set != "" {
		v, err := strconv.Atoi(req.Set)
		if err != nil {
			return response{OK: false, Error: "invalid CPU undervolt value: must be an integer"}
		}
		cpuOffset = v
	}

	if err := cli.ValidateCOValues(cpuOffset); err != nil {
		return response{OK: false, Error: err.Error()}
	}

	if err := cli.SetCurveOptimizer(cpuOffset); err != nil {
		return response{OK: false, Error: "undervolt: " + err.Error()}
	}

	slog.Info("undervolt", "cpu", cpuOffset)
	d.mu.Lock()
	d.state.Undervolt = &api.UndervoltState{CPUCO: cpuOffset, Active: true}
	d.state.UndervoltActive = true
	d.state.ActivePreset = ""
	d.mu.Unlock()
	if err := d.persistState(); err != nil {
		slog.Warn("failed to save state", "err", err)
	}
	return response{OK: true}
}

func (d *Daemon) handleUndervoltReset() response {
	if cli.SMUProbeUndervolt() {
		if err := cli.ResetCurveOptimizer(); err != nil {
			return response{OK: false, Error: "undervolt-reset: " + err.Error()}
		}
	}

	slog.Info("undervolt-reset")
	d.mu.Lock()
	d.state.Undervolt = nil
	d.state.UndervoltActive = false
	d.state.ActivePreset = ""
	d.mu.Unlock()
	if err := d.persistState(); err != nil {
		slog.Warn("failed to save state", "err", err)
	}
	return response{OK: true}
}

func (d *Daemon) handleTuningReset() response {
	d.mu.Lock()
	next := d.state
	d.mu.Unlock()
	next.FanCurve = nil
	next.FanCurveActive = false
	next.TDP = nil
	next.TDPActive = false
	next.Undervolt = nil
	next.UndervoltActive = false
	if err := applyTuning(next, true); err != nil {
		if profileAppliedBeforeError(err) {
			_ = d.commitTuningState(next, "")
		}
		return response{OK: false, Error: "reset tuning overrides: " + err.Error()}
	}
	if err := d.commitTuningState(next, ""); err != nil {
		return response{OK: false, Error: err.Error()}
	}
	return response{OK: true}
}

func writeResponse(conn net.Conn, r response) {
	data, _ := json.Marshal(r)
	_, _ = fmt.Fprintf(conn, "%s\n", data)
}
