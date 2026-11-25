package cli

// battery.go — read-only battery telemetry from power_supply sysfs.
// All values come from /sys/class/power_supply/BAT0/. Writes (charge limit)
// live in sysfs.go (FindBatteryThresholdPath). This file only reads.

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/dahui/z13ctl/api"
)

const (
	// µW → W conversion divisor. power_supply energy and power files use
	// microwatts (per kernel Documentation/ABI/testing/sysfs-class-power-supply).
	microwattsPerWatt = 1_000_000
)

// ReadBatteryState returns the live battery telemetry. Returns nil and an
// error only if BAT0 cannot be found (no battery present).
func ReadBatteryState() (*api.BatteryState, error) {
	matches, _ := filepath.Glob(powerSupplyDir + "/BAT*")
	if len(matches) == 0 {
		return nil, fmt.Errorf("no battery found in %s", powerSupplyDir)
	}
	batPath := matches[0]
	capacity, _ := readIntFile(batPath + "/capacity")
	status, _ := ReadTrimmedFile(batPath + "/status")
	threshold := readIntSafe(batPath + "/charge_control_end_threshold")
	powerMicroW := readIntSafe(batPath + "/power_now")
	voltageMicroV := readIntSafe(batPath + "/voltage_now")
	energyNow := readIntSafe(batPath + "/energy_now")
	energyFull := readIntSafe(batPath + "/energy_full")
	energyDesign := readIntSafe(batPath + "/energy_full_design")

	// Health: energy_full / energy_full_design × 100. Falls back to 100 when
	// design capacity is missing or zero (firmware did not expose it).
	health := 100
	if energyDesign > 0 {
		health = energyFull * 100 / energyDesign
		if health > 100 {
			health = 100
		}
	}

	return &api.BatteryState{
		CapacityPct:    capacity,
		Status:         status,
		HealthPct:      health,
		PowerWatts:     formatMicro(powerMicroW),
		VoltageVolts:   formatMicro(voltageMicroV),
		ThresholdPct:   threshold,
		Charging:       status == "Charging",
		EnergyNowWh:    float64(energyNow) / microwattsPerWatt,
		EnergyFullWh:   float64(energyFull) / microwattsPerWatt,
		EnergyDesignWh: float64(energyDesign) / microwattsPerWatt,
	}, nil
}

func readIntSafe(path string) int {
	v, _ := readIntFile(path)
	return v
}

// formatMicro converts a value in millionths to one decimal place.
func formatMicro(value int) string {
	if value <= 0 {
		return "0"
	}
	return strconv.FormatFloat(float64(value)/microwattsPerWatt, 'f', 1, 64)
}
