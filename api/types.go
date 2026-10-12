// Package api provides the public client interface for the z13ctl-plus daemon.
// It contains the shared protocol types and socket client functions used by
// CLI commands, GUI frontends, and any other tool that communicates with the
// z13ctl-plus daemon over its Unix socket.
package api

// StateSchemaVersion is the current persisted state schema.
const StateSchemaVersion = 2

// State holds the last-applied settings for all controllable subsystems.
// It is returned by SendGetState and broadcast as part of daemon responses.
type State struct {
	SchemaVersion      int                      `json:"schema_version,omitempty"`
	Lighting           LightingState            `json:"lighting"`
	Devices            map[string]LightingState `json:"devices,omitempty"`         // per-device overrides keyed by name
	DevicePresence     map[string]bool          `json:"device_presence,omitempty"` // live HID presence keyed by name
	Profile            string                   `json:"profile,omitempty"`
	Battery            int                      `json:"battery_limit,omitempty"`
	BootSound          int                      `json:"boot_sound,omitempty"`
	PanelOverdrive     int                      `json:"panel_overdrive,omitempty"`
	FanCurve           *FanCurveState           `json:"fan_curve,omitempty"`
	TDP                *TDPState                `json:"tdp,omitempty"`
	Undervolt          *UndervoltState          `json:"undervolt,omitempty"`
	FanCurveActive     bool                     `json:"fan_curve_active,omitempty"`
	TDPActive          bool                     `json:"tdp_active,omitempty"`
	UndervoltActive    bool                     `json:"undervolt_active,omitempty"`
	FanSafetyActive    bool                     `json:"fan_safety_active,omitempty"` // live: high-TDP curve overrides desired fan state
	Presets            map[string]Preset        `json:"presets,omitempty"`
	PowerPolicy        *PowerPolicy             `json:"power_policy,omitempty"`
	ActivePreset       string                   `json:"active_preset,omitempty"`
	PreAutomation      *Preset                  `json:"pre_automation,omitempty"` // settings restored when power automation is turned off
	PowerSource        string                   `json:"power_source,omitempty"`   // live: "ac", "battery", or ""
	UndervoltAvailable bool                     `json:"undervolt_available"`      // true if ryzen_smu is loaded
	CPUPower           *CPUPowerState           `json:"cpu_power,omitempty"`
	RefreshRate        int                      `json:"refresh_rate,omitempty"` // eDP-1 refresh rate in Hz (0 = unknown)
	BatteryDetail      *BatteryState            `json:"battery_detail,omitempty"`
	Temperature        int                      `json:"temperature,omitempty"` // APU temp, degrees Celsius (legacy; also in Telemetry)
	FanRPM             int                      `json:"fan_rpm,omitempty"`     // fan1 speed in RPM (legacy; also in Telemetry)
	Telemetry          *TelemetryState          `json:"telemetry,omitempty"`   // live sensor readings
	TabletSettings     TabletSettings           `json:"tablet_settings"`
	TabletIntegration  *TabletIntegration       `json:"tablet_integration,omitempty"` // live companion status; never persisted
}

// TabletSettings configures the optional z13-tablet-kit companion.
type TabletSettings struct {
	DisableTouchscreenInDesktop bool `json:"disable_touchscreen_in_desktop"`
	TwoFingerHoldContextMenu    bool `json:"two_finger_hold_context_menu"`
	TouchpadEnabled             bool `json:"touchpad_enabled"`
	ScrollSensitivity           int  `json:"scroll_sensitivity"`
	ScrollSpeed                 int  `json:"scroll_speed"`
	// KeepDisplayScale stops posture changes from touching the panel scale.
	KeepDisplayScale bool `json:"keep_display_scale"`
}

// TabletIntegration is the latest live status reported by z13-tablet-kit.
type TabletIntegration struct {
	Posture            string `json:"posture"`
	FolioPresent       bool   `json:"folio_present"`
	Healthy            bool   `json:"healthy"`
	Error              string `json:"error"`
	LastSeen           int64  `json:"last_seen"`
	TouchscreenEnabled bool   `json:"touchscreen_enabled"`
	TouchScrollEnabled bool   `json:"touch_scroll_enabled"`
	TouchpadEnabled    bool   `json:"touchpad_enabled"`
}

// TelemetryState captures live sensor readings from the AMD Strix Halo SoC.
// CPU values come from k10temp + cpufreq + /proc/stat. GPU values come from
// the amdgpu driver sysfs (hwmon + drm device). NPU values come from the
// amdxdna DRM accel driver via ioctl (power, column utilization, clock).
// VRAM reflects the iGPU carveout in unified memory. Memory reflects Linux
// system memory, with used computed as MemTotal - MemAvailable. All values are
// sampled at read time; the daemon does not average or smooth them.
//
// CPUUtil is computed as the delta of /proc/stat jiffies between successive
// calls, so the first call after daemon start may report 0. Subsequent calls
// reflect the utilisation over the interval since the previous call.
//
// Numeric fields use zero when unavailable. Availability flags distinguish
// sensors where zero is also a valid live value, such as stopped fans or an
// idle NPU.
type TelemetryState struct {
	CPUTemp           int     `json:"cpu_temp,omitempty"`            // APU die temp, °C (k10temp Tctl)
	GPUTemp           int     `json:"gpu_temp,omitempty"`            // iGPU edge temp, °C (amdgpu temp1)
	CPUFanRPM         int     `json:"cpu_fan_rpm,omitempty"`         // fan1_input (CPU-side heatsink)
	GPUFanRPM         int     `json:"gpu_fan_rpm,omitempty"`         // fan2_input (GPU-side heatsink)
	FansAvailable     bool    `json:"fans_available,omitempty"`      // true when both fan sensors were read
	CPUUtil           int     `json:"cpu_util,omitempty"`            // 0–100, /proc/stat jiffie delta
	GPUUtil           int     `json:"gpu_util,omitempty"`            // 0–100, amdgpu gpu_busy_percent
	NPUUtil           int     `json:"npu_util,omitempty"`            // 0–100, amdxdna column utilization (avg)
	NPUPowerW         float64 `json:"npu_power_w,omitempty"`         // amdxdna NPU power draw, W (decimal, e.g. 1.85)
	NPUClockMHz       int     `json:"npu_clock_mhz,omitempty"`       // amdxdna live npuclk from current DPM level
	NPUAvailable      bool    `json:"npu_available,omitempty"`       // true when amdxdna telemetry was read
	CPUClockMHz       int     `json:"cpu_clock_mhz,omitempty"`       // average scaling_cur_freq across cores
	GPUClockMHz       int     `json:"gpu_clock_mhz,omitempty"`       // amdgpu freq1_input (sclk) or pp_dpm_sclk active
	APUPowerW         float64 `json:"apu_power_w,omitempty"`         // amdgpu PPT package power, W
	APUPowerAvailable bool    `json:"apu_power_available,omitempty"` // true when amdgpu power telemetry was read
	GPUPowerW         float64 `json:"gpu_power_w,omitempty"`         // amdgpu gpu_metrics GFX domain power, W
	GPUPowerAvailable bool    `json:"gpu_power_available,omitempty"` // true when gpu_metrics exposed GFX power
	VRAMUsedMB        int     `json:"vram_used_mb,omitempty"`        // amdgpu mem_info_vram_used
	VRAMTotalMB       int     `json:"vram_total_mb,omitempty"`       // amdgpu mem_info_vram_total
	MemoryUsedMB      int     `json:"memory_used_mb,omitempty"`      // MemTotal - MemAvailable
	MemoryTotalMB     int     `json:"memory_total_mb,omitempty"`     // Linux MemTotal
	MemClockMHz       int     `json:"mem_clock_mhz,omitempty"`       // amdgpu gpu_metrics time-filtered UCLK
	MemClockAvailable bool    `json:"mem_clock_available,omitempty"` // true when gpu_metrics exposed UCLK
}

// BatteryState captures live battery telemetry from power_supply sysfs.
// Health % is computed from energy_full / energy_full_design. Power is
// instantaneous (positive = discharging, 0 = idle or charging at threshold).
type BatteryState struct {
	CapacityPct    int     `json:"capacity_pct"`     // 0–100, current charge relative to threshold
	Status         string  `json:"status"`           // "Charging" / "Discharging" / "Not charging" / "Full"
	HealthPct      int     `json:"health_pct"`       // energy_full / energy_full_design × 100
	PowerWatts     string  `json:"power_watts"`      // instantaneous draw in watts ("12.3" or "0")
	VoltageVolts   string  `json:"voltage_volts"`    // terminal voltage in volts
	ThresholdPct   int     `json:"threshold_pct"`    // charge_control_end_threshold (40–100)
	Charging       bool    `json:"charging"`         // true when Status == "Charging"
	WhCurrent      int     `json:"wh_current"`       // energy_now in Wh (last full or current)
	WhDesign       int     `json:"wh_design"`        // energy_full_design in Wh
	EnergyNowWh    float64 `json:"energy_now_wh"`    // remaining energy_now in Wh
	EnergyFullWh   float64 `json:"energy_full_wh"`   // current full-charge capacity in Wh
	EnergyDesignWh float64 `json:"energy_design_wh"` // original energy_full_design in Wh
}

// CPUPowerState captures the live AMD P-State controls exposed by cpufreq.
type CPUPowerState struct {
	MinFrequencyKHz int      `json:"min_frequency_khz"`
	MinLimitKHz     int      `json:"min_limit_khz"`
	MaxLimitKHz     int      `json:"max_limit_khz"`
	EPP             string   `json:"epp"`
	EPPChoices      []string `json:"epp_choices"`
	Boost           bool     `json:"boost"`
}

// CPUPowerPreset captures the CPU controls managed by a named preset.
type CPUPowerPreset struct {
	MinFrequencyKHz int    `json:"min_frequency_khz"`
	EPP             string `json:"epp"`
	Boost           bool   `json:"boost"`
}

// Preset is a complete named snapshot of performance and display settings.
// A nil tuning field resets that override when the preset is applied.
type Preset struct {
	Profile        string          `json:"profile"`
	BaseProfile    string          `json:"base_profile,omitempty"` // legacy custom-profile migration only
	FanCurve       *FanCurveState  `json:"fan_curve,omitempty"`
	TDP            *TDPState       `json:"tdp,omitempty"`
	Undervolt      *UndervoltState `json:"undervolt,omitempty"`
	CPUPower       *CPUPowerPreset `json:"cpu_power,omitempty"`
	RefreshRate    int             `json:"refresh_rate,omitempty"`
	PanelOverdrive *int            `json:"panel_overdrive,omitempty"`
}

// PowerPolicy assigns named presets to AC and battery power sources.
type PowerPolicy struct {
	Enabled       bool   `json:"enabled"`
	ACPreset      string `json:"ac_preset"`
	BatteryPreset string `json:"battery_preset"`
}

// LightingState captures all parameters needed to reproduce one lighting zone.
type LightingState struct {
	Enabled    bool   `json:"enabled"`
	Mode       string `json:"mode"`
	Color      string `json:"color"`  // "RRGGBB" hex
	Color2     string `json:"color2"` // "RRGGBB" hex
	Speed      string `json:"speed"`
	Brightness int    `json:"brightness"` // 0–3
}

// FanCurvePoint represents one point on an 8-point fan curve.
type FanCurvePoint struct {
	Temp int `json:"temp"` // degrees Celsius
	PWM  int `json:"pwm"`  // 0–255 duty cycle
}

// FanCurveState captures the fan curve and mode applied to both fans.
type FanCurveState struct {
	Mode   int             `json:"mode"`   // pwm_enable: 0=full-speed, 1=custom, 2=auto
	Points []FanCurvePoint `json:"points"` // 8 points
}

// UndervoltState captures the AMD Curve Optimizer offset applied to the CPU.
// Values are non-positive integers (0 = stock, negative = undervolt).
// Active indicates whether the offset is currently applied to hardware.
type UndervoltState struct {
	CPUCO  int  `json:"cpu_co"` // all-core CPU Curve Optimizer offset
	Active bool `json:"active"` // true when CO is applied to hardware
}

// TDPState captures all PPT (Package Power Tracking) values in watts.
type TDPState struct {
	PL1SPL       int `json:"pl1_spl"`       // Sustained Power Limit
	PL2SPPT      int `json:"pl2_sppt"`      // Short Boost
	FPPT         int `json:"fppt"`          // Fast Boost
	APUSPPT      int `json:"apu_sppt"`      // APU Short PPT
	PlatformSPPT int `json:"platform_sppt"` // Platform Short PPT
}
