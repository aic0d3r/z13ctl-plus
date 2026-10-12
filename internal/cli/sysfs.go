package cli

// sysfs.go — sysfs path discovery helpers shared by cmd/ and internal/daemon/.

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// FindProfilePath returns the writable sysfs path for the ASUS platform-profile attribute.
// It prefers the device whose choices file contains "quiet" (the asus-wmi device), falling
// back to the first device with a profile file, then the ACPI alias.
func FindProfilePath() string {
	const dir = "/sys/class/platform-profile"
	entries, err := os.ReadDir(dir)
	if err == nil {
		// First pass: prefer the ASUS device (choices includes "quiet").
		for _, e := range entries {
			base := dir + "/" + e.Name()
			if profileDeviceSupports(base, "quiet") {
				p := base + "/profile"
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
		}
		// Fallback: first device with a profile file.
		for _, e := range entries {
			p := dir + "/" + e.Name() + "/profile"
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return "/sys/firmware/acpi/platform_profile"
}

// SetProfile writes the given ASUS profile (quiet/balanced/performance) to all
// platform_profile sysfs devices, mapping "quiet" to "low-power" for devices that
// do not support that name. Returns an error only if the primary ASUS device write
// fails; secondary device errors are ignored.
func SetProfile(profile string) error {
	const dir = "/sys/class/platform-profile"
	primaryPath := FindProfilePath()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return os.WriteFile(primaryPath, []byte(profile+"\n"), 0o644)
	}
	var primaryErr error
	attempted := 0
	for _, e := range entries {
		base := dir + "/" + e.Name()
		p := base + "/profile"
		if _, err := os.Stat(p); err != nil {
			continue
		}
		attempted++
		name := profileNameForDevice(base, profile)
		werr := os.WriteFile(p, []byte(name+"\n"), 0o644)
		if p == primaryPath {
			primaryErr = werr
		}
	}
	if attempted == 0 {
		// The class directory exists but no entry exposes a profile file;
		// fall back to the ACPI path and surface its result instead of
		// silently reporting success.
		return os.WriteFile(primaryPath, []byte(profile+"\n"), 0o644)
	}
	if primaryErr == nil {
		setPPD(profile)
	}
	return primaryErr
}

// setPPD updates the power-profiles-daemon active profile to match the given
// ASUS profile. No-op if powerprofilesctl is not installed. Silently ignores
// errors if PPD is installed but not currently running.
func setPPD(asusProfile string) {
	ppd := map[string]string{
		"quiet":       "power-saver",
		"balanced":    "balanced",
		"performance": "performance",
	}[asusProfile]
	if ppd == "" {
		return
	}
	bin, err := exec.LookPath("powerprofilesctl")
	if err != nil {
		return
	}
	if out, err := exec.Command(bin, "set", ppd).CombinedOutput(); err != nil {
		// PPD's amd-pstate driver restores profiles by writing per-policy
		// boost files, which the kernel rejects (EINVAL) while the global
		// boost file is 0 — z13ctl writes the global one. PPD then sticks on
		// its previous profile, so governor/EPP stop following profile
		// switches until boost is re-enabled. The sysfs profile write above
		// already succeeded, so warn instead of failing the switch.
		slog.Warn("powerprofilesctl set failed; PPD may stay stuck until CPU boost is re-enabled",
			"profile", ppd, "err", err, "output", strings.TrimSpace(string(out)))
	}
}

// profileDeviceSupports reports whether the platform_profile device at base
// lists name in its choices file.
func profileDeviceSupports(base, name string) bool {
	data, err := os.ReadFile(base + "/choices")
	return err == nil && slices.Contains(strings.Fields(string(data)), name)
}

// profileNameForDevice returns the appropriate profile name for a given device,
// mapping the ASUS-specific "quiet" to "low-power" for devices that don't support it.
func profileNameForDevice(base, asusProfile string) string {
	if asusProfile != "quiet" {
		return asusProfile // "balanced" and "performance" are universal
	}
	if profileDeviceSupports(base, "quiet") {
		return "quiet"
	}
	if profileDeviceSupports(base, "low-power") {
		return "low-power"
	}
	return asusProfile
}

// FindBatteryThresholdPath returns the writable sysfs path for the battery charge
// end threshold. It globs BAT* to avoid hardcoding BAT0 vs BAT1.
func FindBatteryThresholdPath() string {
	const glob = "/sys/class/power_supply/BAT*/charge_control_end_threshold"
	matches, _ := filepath.Glob(glob)
	if len(matches) > 0 {
		return matches[0]
	}
	return "/sys/class/power_supply/BAT0/charge_control_end_threshold"
}

// FindBootSoundPath returns the sysfs path for the boot sound firmware attribute.
func FindBootSoundPath() string {
	return "/sys/class/firmware-attributes/asus-armoury/attributes/boot_sound/current_value"
}

// FindPanelOverdrivePath returns the sysfs path for the panel overdrive firmware attribute.
func FindPanelOverdrivePath() string {
	return "/sys/class/firmware-attributes/asus-armoury/attributes/panel_overdrive/current_value"
}

// SetBootSound writes the given boot sound value (0 or 1) to the firmware attribute.
func SetBootSound(value int) error {
	return os.WriteFile(FindBootSoundPath(), []byte(strconv.Itoa(value)+"\n"), 0o644)
}

// SetPanelOverdrive writes the given panel overdrive value (0 or 1) to the firmware attribute.
func SetPanelOverdrive(value int) error {
	return os.WriteFile(FindPanelOverdrivePath(), []byte(strconv.Itoa(value)+"\n"), 0o644)
}

// ReadAPUTemperature reads the current APU die temperature from k10temp.
// The k10temp driver reports millidegrees; this function converts to whole degrees.
func ReadAPUTemperature() (int, error) {
	dir := FindFanHwmonPath("k10temp")
	if dir == "" {
		return 0, fmt.Errorf("k10temp hwmon device not found")
	}
	milli, err := readIntFile(dir + "/temp1_input")
	if err != nil {
		return 0, fmt.Errorf("reading APU temperature: %w", err)
	}
	return milli / 1000, nil
}

// FindBatteryCapacityPath returns the sysfs path for the current battery charge level.
func FindBatteryCapacityPath() string {
	const glob = "/sys/class/power_supply/BAT*/capacity"
	matches, _ := filepath.Glob(glob)
	if len(matches) > 0 {
		return matches[0]
	}
	return "/sys/class/power_supply/BAT0/capacity"
}
