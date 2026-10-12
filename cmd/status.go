package cmd

// status.go — "status" subcommand: display a summary of all system metrics.
// Read-only command, no flags. Aggregates APU temperature, fan RPM, profile,
// TDP, and battery information into a single dashboard view.

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aic0d3r/z13ctl-plus/internal/cli"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show system status (temperature, fans, profile, TDP, battery)",
	Long: `Display a summary of all system metrics in a single view.

Shows APU temperature, fan speed and mode, performance profile, TDP power
limits, and battery charge level and limit. All values are read directly
from sysfs.`,
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return runStatus()
	},
}

func runStatus() error {
	// APU temperature.
	if temp, err := cli.ReadAPUTemperature(); err == nil {
		fmt.Printf("APU:     %d°C\n", temp)
	} else {
		fmt.Println("APU:     N/A")
	}

	// Fan RPM and mode.
	rpms, rpmErr := cli.ReadBothFanRPM()
	modes, modeErr := cli.ReadBothFanModes()
	cpuFanStr, gpuFanStr := "N/A", "N/A"
	if rpmErr == nil {
		cpuFanStr = fmt.Sprintf("%d RPM", rpms[0])
		gpuFanStr = fmt.Sprintf("%d RPM", rpms[1])
	}
	modeStr := ""
	if modeErr == nil {
		modeStr = ", mode: " + cli.FanModeName(modes[0])
	}
	fmt.Printf("CPU Fan: %s%s\n", cpuFanStr, modeStr)
	fmt.Printf("GPU Fan: %s\n", gpuFanStr)

	// Performance profile.
	profile := readCurrentProfile()
	fmt.Printf("Profile: %s\n", profile)

	// TDP power limits.
	tdp, tdpErr := readTDP(profile)
	if tdpErr == nil {
		fmt.Printf("TDP:     %dW (PL1) / %dW (PL2) / %dW (PL3)\n",
			tdp.PL1SPL, tdp.PL2SPPT, tdp.FPPT)
	} else {
		fmt.Println("TDP:     N/A")
	}

	// Undervolt (Curve Optimizer) — only shown if ryzen_smu is available.
	// CO values have no sysfs readback; this would need daemon state.
	// For status, just indicate availability.
	if cli.SMUAvailable() {
		fmt.Println("UV:      available (use 'undervolt --get' via daemon for current values)")
	}

	// Battery: current charge level and charge limit.
	capStr := "N/A"
	if data, err := os.ReadFile(cli.FindBatteryCapacityPath()); err == nil {
		capStr = strings.TrimSpace(string(data)) + "%"
	}
	limitStr := ""
	if data, err := os.ReadFile(cli.FindBatteryThresholdPath()); err == nil {
		limitStr = " (limit: " + strings.TrimSpace(string(data)) + "%)"
	}
	fmt.Printf("Battery: %s%s\n", capStr, limitStr)

	// Live telemetry (CPU/GPU temp+util, clocks, VRAM, mem clock). CPU
	// utilisation is a delta since the previous read, so prime the snapshot
	// and let a short window elapse before sampling.
	_, _ = cli.ReadCPUUtilization()
	time.Sleep(200 * time.Millisecond)
	t := cli.ReadTelemetry()
	fmt.Println("Telemetry:")
	fmt.Printf("  CPU:    %d°C, %d%%, %s, %d RPM\n", t.CPUTemp, t.CPUUtil, formatClock(t.CPUClockMHz), t.CPUFanRPM)
	fmt.Printf("  GPU:    %d°C, %d%%, %s, %d RPM\n", t.GPUTemp, t.GPUUtil, formatClock(t.GPUClockMHz), t.GPUFanRPM)
	if t.APUPowerAvailable {
		fmt.Printf("  APU:    %.1f W PPT\n", t.APUPowerW)
	}
	if t.GPUPowerAvailable {
		fmt.Printf("  GPU:    %.1f W GFX\n", t.GPUPowerW)
	}
	if t.NPUUtil != 0 || t.NPUPowerW != 0 || t.NPUClockMHz != 0 {
		fmt.Printf("  NPU:    %d%%, %.2fW, %s\n", t.NPUUtil, t.NPUPowerW, formatClock(t.NPUClockMHz))
	}
	fmt.Printf("  VRAM:   %d / %d MB\n", t.VRAMUsedMB, t.VRAMTotalMB)
	fmt.Printf("  Memory: %d / %d MB\n", t.MemoryUsedMB, t.MemoryTotalMB)
	fmt.Printf("  Mem clk: %s\n", formatClock(t.MemClockMHz))

	return nil
}

// formatClock renders a clock value in MHz or GHz depending on magnitude.
func formatClock(mhz int) string {
	if mhz <= 0 {
		return "N/A"
	}
	if mhz < 1000 {
		return fmt.Sprintf("%d MHz", mhz)
	}
	return fmt.Sprintf("%.2f GHz", float64(mhz)/1000.0)
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
