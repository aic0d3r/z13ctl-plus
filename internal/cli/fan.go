package cli

// fan.go — hwmon sysfs path discovery and I/O helpers for ASUS fan curves.
// Discovers hwmon devices by name attribute (not by number, which is unstable).
//
// The 2025 ROG Flow Z13 has an APU with two physical fans but no discrete GPU.
// Both fans cool the same chip, so the same curve is always applied to both.

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dahui/z13ctl/api"
)

const (
	hwmonDir = "/sys/class/hwmon"

	// hwmon device names exposed by the asus-wmi kernel driver.
	hwmonNameReadings = "asus"                  // fan RPM + pwm_enable
	hwmonNameCurves   = "asus_custom_fan_curve" // 8-point curves + pwm_enable

	fanCurvePoints = 8
	fanCount       = 2 // fan 1 (pwm1) and fan 2 (pwm2)

	// HighTDPMinPWM is the minimum PWM value (50% of 255) enforced when
	// sustained TDP exceeds TDPMaxSafe. Users can set fans higher but not lower.
	HighTDPMinPWM = 127
)

// fanIndices are the hwmon fan indexes.
var fanIndices = [fanCount]int{1, 2}

// FindFanHwmonPath returns the sysfs hwmon directory whose name attribute
// matches the given value. Returns "" if not found. hwmon numbers are
// unstable across reboots, so discovery by name is required.
func FindFanHwmonPath(name string) string {
	entries, err := os.ReadDir(hwmonDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		p := hwmonDir + "/" + e.Name() + "/name"
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) == name {
			return hwmonDir + "/" + e.Name()
		}
	}
	return ""
}

// FindFanReadingsHwmonPath returns the hwmon dir for fan RPM and mode readings.
func FindFanReadingsHwmonPath() string {
	return FindFanHwmonPath(hwmonNameReadings)
}

// FindFanCurveHwmonPath returns the hwmon dir for custom fan curve points.
func FindFanCurveHwmonPath() string {
	return FindFanHwmonPath(hwmonNameCurves)
}

// ReadBothFanRPM reads the current RPM for both fans.
// Returns [2]int with fan1 and fan2 RPM values.
func ReadBothFanRPM() ([fanCount]int, error) {
	dir := FindFanReadingsHwmonPath()
	if dir == "" {
		return [fanCount]int{}, fmt.Errorf("hwmon device %q not found", hwmonNameReadings)
	}
	var rpms [fanCount]int
	for i, f := range fanIndices {
		v, err := readIntFile(dir + "/" + fmt.Sprintf("fan%d_input", f))
		if err != nil {
			return rpms, fmt.Errorf("reading fan%d RPM: %w", f, err)
		}
		rpms[i] = v
	}
	return rpms, nil
}

// ReadBothFanModes reads the pwm_enable value for both fans from the curve
// hwmon device. Returns 0 (full-speed), 1 (custom), or 2 (auto/firmware).
func ReadBothFanModes() ([fanCount]int, error) {
	dir := FindFanCurveHwmonPath()
	if dir == "" {
		return [fanCount]int{}, fmt.Errorf("hwmon device %q not found", hwmonNameCurves)
	}
	var modes [fanCount]int
	for i, f := range fanIndices {
		v, err := readIntFile(dir + "/" + fmt.Sprintf("pwm%d_enable", f))
		if err != nil {
			return modes, fmt.Errorf("reading fan%d mode: %w", f, err)
		}
		modes[i] = v
	}
	return modes, nil
}

// ReadBothFanCurves reads the 8-point fan curve for both fans.
func ReadBothFanCurves() ([fanCount][]api.FanCurvePoint, error) {
	dir := FindFanCurveHwmonPath()
	if dir == "" {
		return [fanCount][]api.FanCurvePoint{}, fmt.Errorf("hwmon device %q not found", hwmonNameCurves)
	}
	var curves [fanCount][]api.FanCurvePoint
	for fi, f := range fanIndices {
		points := make([]api.FanCurvePoint, fanCurvePoints)
		for i := range fanCurvePoints {
			temp, err := readIntFile(dir + "/" + fmt.Sprintf("pwm%d_auto_point%d_temp", f, i+1))
			if err != nil {
				return curves, fmt.Errorf("reading fan%d curve point %d temp: %w", f, i+1, err)
			}
			pwm, err := readIntFile(dir + "/" + fmt.Sprintf("pwm%d_auto_point%d_pwm", f, i+1))
			if err != nil {
				return curves, fmt.Errorf("reading fan%d curve point %d pwm: %w", f, i+1, err)
			}
			points[i] = api.FanCurvePoint{Temp: temp, PWM: pwm}
		}
		curves[fi] = points
	}
	return curves, nil
}

// SetBothFanCurves writes the same 8-point fan curve to both fans and enables
// custom mode (pwm_enable=1) on both.
func SetBothFanCurves(points []api.FanCurvePoint) error {
	if len(points) != fanCurvePoints {
		return fmt.Errorf("fan curve must have exactly %d points, got %d", fanCurvePoints, len(points))
	}
	dir := FindFanCurveHwmonPath()
	if dir == "" {
		return fmt.Errorf("hwmon device %q not found", hwmonNameCurves)
	}
	for _, f := range fanIndices {
		for i, p := range points {
			if err := writeIntFile(dir+"/"+fmt.Sprintf("pwm%d_auto_point%d_temp", f, i+1), p.Temp); err != nil {
				return fmt.Errorf("writing fan%d curve point %d temp: %w", f, i+1, err)
			}
			if err := writeIntFile(dir+"/"+fmt.Sprintf("pwm%d_auto_point%d_pwm", f, i+1), p.PWM); err != nil {
				return fmt.Errorf("writing fan%d curve point %d pwm: %w", f, i+1, err)
			}
		}
	}
	return setAllFanModes(1) // enable custom mode on both
}

// setAllFanModes writes custom or auto mode to the curve device for both fans,
// then syncs the readings device when available.
func setAllFanModes(mode int) error {
	curveDir := FindFanCurveHwmonPath()
	if curveDir == "" {
		return fmt.Errorf("hwmon device %q not found", hwmonNameCurves)
	}
	readDir := FindFanReadingsHwmonPath()
	for _, idx := range fanIndices {
		file := fmt.Sprintf("pwm%d_enable", idx)
		if err := writeIntFile(curveDir+"/"+file, mode); err != nil {
			return fmt.Errorf("setting fan mode on %s: %w", hwmonNameCurves, err)
		}
		if readDir != "" {
			_ = writeIntFile(readDir+"/"+file, mode)
		}
	}
	return nil
}

// ResetAllFanCurves restores firmware auto mode for both fans.
func ResetAllFanCurves() error {
	return setAllFanModes(2) // auto/firmware
}

// HighTDPFanCurve returns an 8-point fan curve with a minimum PWM of 50%,
// suitable for sustained TDP above 75W. Users can replace this with a custom
// curve as long as all PWM values stay at or above HighTDPMinPWM.
// The curve ramps up with temperature and reaches full speed at 80°C.
func HighTDPFanCurve() []api.FanCurvePoint {
	return []api.FanCurvePoint{
		{Temp: 30, PWM: HighTDPMinPWM},
		{Temp: 40, PWM: HighTDPMinPWM},
		{Temp: 50, PWM: 140},
		{Temp: 60, PWM: 165},
		{Temp: 65, PWM: 190},
		{Temp: 70, PWM: 215},
		{Temp: 75, PWM: 235},
		{Temp: 80, PWM: 255},
	}
}

// FanCurveEquals reports whether two fan curves have identical points.
func FanCurveEquals(a, b []api.FanCurvePoint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// LiftHighTDPFanCurve resets both fans to firmware auto mode if the current
// custom curve is exactly the high-TDP safety curve. Used by the daemonless
// CLI path, which keeps no record of the user's desired curve.
func LiftHighTDPFanCurve() {
	modes, modeErr := ReadBothFanModes()
	curves, curveErr := ReadBothFanCurves()
	if modeErr != nil || curveErr != nil {
		return
	}
	safety := HighTDPFanCurve()
	for i := range fanIndices {
		if modes[i] != 1 || !FanCurveEquals(curves[i], safety) {
			return
		}
	}
	_ = ResetAllFanCurves()
}

// ParseFanCurve parses a fan curve string "temp:pwm,temp:pwm,..." into a
// slice of FanCurvePoint. Requires exactly 8 points. Temps must be
// monotonically increasing (0–120°C). PWM values must be monotonically
// non-decreasing (0–255). PWM values may use a % suffix for percentage
// (0–100%), which is converted to PWM (e.g. 80% = 204). Both formats
// can be mixed in the same curve string.
func ParseFanCurve(s string) ([]api.FanCurvePoint, error) {
	parts := strings.Split(s, ",")
	if len(parts) != fanCurvePoints {
		return nil, fmt.Errorf("fan curve must have exactly %d points, got %d", fanCurvePoints, len(parts))
	}
	points := make([]api.FanCurvePoint, fanCurvePoints)
	for i, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), ":", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("invalid curve point %q: expected temp:pwm or temp:pct%%", part)
		}
		temp, err := strconv.Atoi(strings.TrimSpace(kv[0]))
		if err != nil {
			return nil, fmt.Errorf("invalid temp in point %d: %w", i+1, err)
		}
		pwmStr := strings.TrimSpace(kv[1])
		isPercent := strings.HasSuffix(pwmStr, "%")
		if isPercent {
			pwmStr = strings.TrimSuffix(pwmStr, "%")
		}
		pwm, err := strconv.Atoi(pwmStr)
		if err != nil {
			return nil, fmt.Errorf("invalid pwm in point %d: %w", i+1, err)
		}
		if temp < 0 || temp > 120 {
			return nil, fmt.Errorf("temp %d in point %d out of range 0–120", temp, i+1)
		}
		if isPercent {
			if pwm < 0 || pwm > 100 {
				return nil, fmt.Errorf("percentage %d in point %d out of range 0–100", pwm, i+1)
			}
			pwm = pwm * 255 / 100
		} else if pwm < 0 || pwm > 255 {
			return nil, fmt.Errorf("pwm %d in point %d out of range 0–255", pwm, i+1)
		}
		if i > 0 && temp <= points[i-1].Temp {
			return nil, fmt.Errorf("temps must be monotonically increasing: point %d (%d) <= point %d (%d)", i+1, temp, i, points[i-1].Temp)
		}
		if i > 0 && pwm < points[i-1].PWM {
			return nil, fmt.Errorf("pwm values must be non-decreasing: point %d (%d) < point %d (%d)", i+1, pwm, i, points[i-1].PWM)
		}
		points[i] = api.FanCurvePoint{Temp: temp, PWM: pwm}
	}
	return points, nil
}

// FanModeName returns a human-readable name for a pwm_enable value.
func FanModeName(mode int) string {
	switch mode {
	case 0:
		return "full-speed"
	case 1:
		return "custom"
	case 2:
		return "auto"
	default:
		return fmt.Sprintf("unknown(%d)", mode)
	}
}

// readIntFile reads a sysfs file and parses its content as an integer.
func readIntFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// writeIntFile writes an integer value to a sysfs file.
func writeIntFile(path string, value int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(value)+"\n"), 0o644)
}
