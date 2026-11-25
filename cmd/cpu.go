package cmd

// cpu.go — "cpu" subcommand: CPU minimum frequency, energy preference (EPP)
// and boost via amd-pstate cpufreq. Also "refresh" (internal panel refresh
// rate) and "reset-overrides" (drop fan, TDP and undervolt overrides).

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aic0d3r/z13ctl-plus/internal/cli"
	"github.com/dahui/z13ctl/api"

	"github.com/spf13/cobra"
)

var (
	cpuGetFlag   bool
	cpuMinFlag   int
	cpuEPPFlag   string
	cpuBoostFlag string
)

var cpuCmd = &cobra.Command{
	Use:   "cpu",
	Short: "Get or set CPU minimum frequency, energy preference and boost",
	Long: `Get or set the amd-pstate CPU controls that presets also manage.

  --min KHZ       minimum frequency in kHz (see --get for the allowed range)
  --epp VALUE     energy performance preference (see --get for the choices)
  --boost on|off  CPU boost`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		set := cmd.Flags().Changed("min") || cpuEPPFlag != "" || cpuBoostFlag != ""
		if !set && !cpuGetFlag {
			return cmd.Help()
		}
		if set {
			if err := runCPUSet(cmd.Flags().Changed("min")); err != nil {
				return err
			}
		}
		if cpuGetFlag {
			cpu, err := cli.ReadCPUPower()
			if err != nil {
				return fmt.Errorf("reading CPU power controls: %w", err)
			}
			fmt.Printf("Minimum: %d kHz (range %d-%d)\n", cpu.MinFrequencyKHz, cpu.MinLimitKHz, cpu.MaxLimitKHz)
			fmt.Printf("EPP:     %s (choices: %s)\n", cpu.EPP, strings.Join(cpu.EPPChoices, ", "))
			fmt.Printf("Boost:   %s\n", onOff(cpu.Boost))
		}
		return nil
	},
}

func runCPUSet(setMin bool) error {
	var boost *bool
	if cpuBoostFlag != "" {
		v, err := parseOnOff(cpuBoostFlag)
		if err != nil {
			return fmt.Errorf("--boost: %w", err)
		}
		boost = &v
	}
	if dryRunFlag {
		if setMin {
			fmt.Printf("Would set CPU minimum to %d kHz\n", cpuMinFlag)
		}
		if cpuEPPFlag != "" {
			fmt.Printf("Would set CPU EPP to %s\n", cpuEPPFlag)
		}
		if boost != nil {
			fmt.Printf("Would turn CPU boost %s\n", onOff(*boost))
		}
		return nil
	}
	steps := []struct {
		run    bool
		label  string
		send   func() (bool, error)
		direct func() error
	}{
		{setMin, fmt.Sprintf("minimum %d kHz", cpuMinFlag),
			func() (bool, error) { return api.SendCPUMinFrequencySet(cpuMinFlag) },
			func() error { return cli.SetCPUMinFrequency(cpuMinFlag) }},
		{cpuEPPFlag != "", "EPP " + cpuEPPFlag,
			func() (bool, error) { return api.SendCPUEPPSet(cpuEPPFlag) },
			func() error { return cli.SetCPUEPP(cpuEPPFlag) }},
		{boost != nil, "boost " + cpuBoostFlag,
			func() (bool, error) { return api.SendCPUBoostSet(boost != nil && *boost) },
			func() error { return cli.SetCPUBoost(boost != nil && *boost) }},
	}
	direct := false
	for _, s := range steps {
		if !s.run {
			continue
		}
		handled, err := s.send()
		if !handled && err == nil {
			direct = true
			err = s.direct()
		}
		if err != nil {
			return fmt.Errorf("CPU %s: %w", s.label, err)
		}
		fmt.Printf("CPU %s set\n", s.label)
	}
	if direct {
		noteDirect()
	} else {
		noteAutomation()
	}
	return nil
}

var (
	refreshGetFlag bool
	refreshSetFlag int
)

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Get or set the built-in panel refresh rate (60 or 180 Hz)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if !refreshGetFlag && refreshSetFlag == 0 {
			return cmd.Help()
		}
		if refreshSetFlag != 0 {
			if dryRunFlag {
				fmt.Printf("Would set the built-in panel to %d Hz\n", refreshSetFlag)
				return nil
			}
			handled, err := api.SendRefreshRateSet(refreshSetFlag)
			if !handled && err == nil {
				err = cli.ApplyInternalDisplay(0, "", refreshSetFlag)
				defer noteDirect()
			} else if err == nil {
				defer noteAutomation()
			}
			if err != nil {
				return err
			}
			fmt.Printf("Refresh rate set to %d Hz\n", refreshSetFlag)
		}
		if refreshGetFlag {
			if hz := cli.ReadRefreshRate(); hz != 0 {
				fmt.Printf("%d Hz\n", hz)
			} else {
				fmt.Println("unknown (panel off or kscreen-doctor unavailable)")
			}
		}
		return nil
	},
}

var resetOverridesCmd = &cobra.Command{
	Use:   "reset-overrides",
	Short: "Drop the fan curve, TDP and undervolt overrides and return to the profile defaults",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		if dryRunFlag {
			fmt.Println("Would reset fan curve, TDP and undervolt overrides")
			return nil
		}
		handled, err := api.SendTuningReset()
		if err != nil {
			return err
		}
		if !handled {
			return fmt.Errorf("reset-overrides requires the z13ctl-plus daemon; use fancurve/tdp/undervolt --reset instead")
		}
		fmt.Println("Fan curve, TDP and undervolt overrides reset")
		return nil
	},
}

func parseOnOff(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "on", "1", "true":
		return true, nil
	case "off", "0", "false":
		return false, nil
	}
	return strconv.ParseBool(s)
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// noteAutomation warns that a manual change only lasts until the next power
// source change when AC/battery automation is on.
func noteAutomation() {
	handled, state, err := api.SendGetState()
	if !handled || err != nil || state.PowerPolicy == nil || !state.PowerPolicy.Enabled {
		return
	}
	fmt.Println("note: automatic AC/battery presets are on; this change lasts until the charger is plugged in or removed.")
}

func init() {
	cpuCmd.Flags().BoolVar(&cpuGetFlag, "get", false, "Show the current CPU controls")
	cpuCmd.Flags().IntVar(&cpuMinFlag, "min", 0, "Minimum frequency in kHz")
	cpuCmd.Flags().StringVar(&cpuEPPFlag, "epp", "", "Energy performance preference")
	cpuCmd.Flags().StringVar(&cpuBoostFlag, "boost", "", "CPU boost: on or off")
	refreshCmd.Flags().BoolVar(&refreshGetFlag, "get", false, "Show the current refresh rate")
	refreshCmd.Flags().IntVar(&refreshSetFlag, "set", 0, "Refresh rate in Hz (60 or 180)")
	rootCmd.AddCommand(cpuCmd, refreshCmd, resetOverridesCmd)
}
