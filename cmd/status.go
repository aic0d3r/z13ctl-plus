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
	"github.com/dahui/z13ctl/api"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show system status (temperature, fans, profile, TDP, battery)",
	Long: `Display a summary of all system metrics in a single view.

Shows daemon health (active preset, automation, overrides and whether they
are applied), APU temperature, fan speed and mode, performance profile, TDP
power limits, undervolt, and battery charge level and limit.`,
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return runStatus()
	},
}

func runStatus() error {
	handled, state, stateErr := api.SendGetState()
	switch {
	case stateErr != nil:
		fmt.Printf("Daemon:  unreachable (%v)\n", stateErr)
		state = nil
	case !handled:
		fmt.Println("Daemon:  not running (changes are not restored after sleep or reboot)")
		state = nil
	default:
		printDaemonStatus(state)
	}

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

	// Undervolt: the daemon's probe proves the ryzen_smu fork works; module
	// presence alone does not (the CLI must not probe: it writes a CO command).
	switch {
	case state != nil && state.UndervoltAvailable && state.UndervoltActive && state.Undervolt != nil:
		fmt.Printf("UV:      %d (active)\n", state.Undervolt.CPUCO)
	case state != nil && state.UndervoltAvailable:
		fmt.Println("UV:      available, not set")
	case state != nil && cli.SMUAvailable():
		fmt.Println("UV:      unavailable (ryzen_smu rejects commands or no access; see 'z13ctl-plus doctor')")
	case cli.SMUAvailable():
		fmt.Println("UV:      ryzen_smu loaded (not verified: daemon not running)")
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

// printDaemonStatus shows what the daemon manages: preset, automation and
// overrides, and whether they are actually applied.
func printDaemonStatus(s *api.State) {
	fmt.Println("Daemon:  running")
	if p := s.PowerPolicy; p != nil && p.Enabled {
		fmt.Printf("Auto:    on (AC=%s, Battery=%s); manual changes last until the next plug or unplug\n", p.ACPreset, p.BatteryPreset)
	} else {
		fmt.Println("Auto:    off")
	}
	if s.PowerSource != "" {
		fmt.Printf("Power:   %s\n", s.PowerSource)
	}
	preset := s.ActivePreset
	if preset == "" {
		preset = "none (manual settings)"
	}
	fmt.Printf("Preset:  %s\n", preset)
	var overrides []string
	if s.FanCurveActive {
		overrides = append(overrides, "fan curve")
	}
	if s.TDPActive {
		overrides = append(overrides, "TDP")
	}
	if s.UndervoltActive {
		overrides = append(overrides, "undervolt")
	}
	if len(overrides) == 0 {
		overrides = []string{"none"}
	}
	fmt.Printf("Overrides: %s\n", strings.Join(overrides, ", "))
	if s.TuningError != "" {
		fmt.Printf("WARNING: overrides are NOT applied (daemon retrying): %s\n", s.TuningError)
	}
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
