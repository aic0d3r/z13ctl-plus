package cmd

// tdp.go — "tdp" subcommand: read or set TDP power limits via the Linux
// asus-nb-wmi PPT sysfs attributes. No HID access required.

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/aic0d3r/z13ctl-plus/internal/cli"
	"github.com/dahui/z13ctl/api"

	"github.com/spf13/cobra"
)

var (
	tdpGetFlag   bool
	tdpSetFlag   string
	tdpResetFlag bool
	tdpPL1Flag   string
	tdpPL2Flag   string
	tdpPL3Flag   string
	tdpForceFlag bool
)

var tdpCmd = &cobra.Command{
	Use:   "tdp",
	Short: "Get or set TDP power limits via asus-nb-wmi PPT",
	Long: `Get or set TDP power limits via the Linux asus-nb-wmi PPT sysfs attributes.

With --get, prints all current PPT (Package Power Tracking) values.

With --set, writes power limits in watts. By default, all PPT values are set to
the same value. Use --pl1, --pl2, --pl3 to override individual limits.

Safety: The sustained power limit (PL1) is capped at 75W by default. Use --force
to allow PL1 up to 93W (the absolute hardware maximum for the ROG Flow Z13
GZ302E). When PL1 exceeds 75W, fans are temporarily locked to a 50%+ curve for
thermal safety. Burst limits (PL2/PL3) are allowed up to 93W without --force
since short bursts are thermally safe.

With --reset, restores the current physical profile's firmware PPT defaults.
Other active overrides are preserved when the daemon is running.

PPT attributes:
  PL1/SPL          — Sustained Power Limit: the continuous power budget the APU
                     can draw indefinitely. This is your effective base TDP.
  PL2/sPPT         — Short-term boost: the APU can draw this much power for
                     several seconds before throttling back to PL1.
  PL3/fPPT         — Fast boost: the maximum instantaneous power the APU can
                     draw for millisecond-scale spikes (e.g. launching an app).
  APU sPPT         — APU-specific short-term limit (automatically set to PL2).
  Platform sPPT    — Platform-level short-term limit (automatically set to PL2).

When using --set, all three limits are set to the same value by default. Use
--pl1, --pl2, and --pl3 to set them independently — for example, --set 45
--pl2 55 --pl3 65 allows short bursts up to 65W while sustaining 45W.

The physical profile remains quiet, balanced, or performance while a custom TDP
override is active.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if !tdpGetFlag && tdpSetFlag == "" && !tdpResetFlag {
			return cmd.Help()
		}

		if tdpSetFlag != "" {
			return runTdpSet()
		}
		if tdpResetFlag {
			return runTdpReset()
		}
		return runTdpGet()
	},
}

func runTdpGet() error {
	tdp, err := readTDP(readCurrentProfile())
	if err != nil {
		return fmt.Errorf("reading TDP: %w", err)
	}

	fmt.Println("TDP Power Limits (watts):")
	fmt.Printf("  PL1 (SPL):          %d\n", tdp.PL1SPL)
	fmt.Printf("  PL2 (sPPT):         %d\n", tdp.PL2SPPT)
	fmt.Printf("  PL3 (fPPT):         %d\n", tdp.FPPT)
	fmt.Printf("  APU sPPT:           %d\n", tdp.APUSPPT)
	fmt.Printf("  Platform sPPT:      %d\n", tdp.PlatformSPPT)
	return nil
}

// readTDP prefers the daemon, which knows whether a TDP override is active;
// the raw sysfs cache keeps reporting the last write after a reset.
func readTDP(profile string) (api.TDPState, error) {
	if handled, value, err := api.SendTdpGet(); handled && err == nil {
		var tdp api.TDPState
		if err := json.Unmarshal([]byte(value), &tdp); err == nil {
			return tdp, nil
		}
	}
	return cli.ReadEffectivePPT(profile)
}

func readCurrentProfile() string {
	profile, err := cli.ReadTrimmedFile(cli.FindProfilePath())
	if err != nil || profile == "" {
		return "unknown"
	}
	return profile
}

func runTdpSet() error {
	watts, err := strconv.Atoi(tdpSetFlag)
	if err != nil {
		return fmt.Errorf("invalid TDP value %q: must be an integer", tdpSetFlag)
	}

	pl1, pl2, pl3, err := parsePLOverrides(watts)
	if err != nil {
		return err
	}

	// PL1 (sustained) requires --force above 75W. PL2/PL3 (burst) are allowed
	// up to the hardware max without --force since short bursts are thermally safe.
	if err := cli.ValidateTDP(pl1, pl2, pl3, tdpForceFlag); err != nil {
		return err
	}

	if dryRunFlag {
		cli.DryRunTdp(watts, pl1, pl2, pl3, tdpForceFlag)
		return nil
	}

	if handled, err := api.SendTdpSet(tdpSetFlag, tdpPL1Flag, tdpPL2Flag, tdpPL3Flag, tdpForceFlag); handled {
		if err != nil {
			return err
		}
		fmt.Printf("TDP set to %dW\n", watts)
		return nil
	}

	// Direct fallback has no daemon safety planner, so lock the fans before
	// writing a high sustained limit.
	if pl1 > cli.TDPMaxSafe {
		if err := cli.SetBothFanCurves(cli.HighTDPFanCurve()); err != nil {
			return fmt.Errorf("failed to set high-TDP fan curve: %w (refusing to apply unsafe TDP)", err)
		}
		fmt.Println("Fans set to 50%+ curve for thermal safety")
	}

	if err := cli.SetTDP(watts, pl1, pl2, pl3); err != nil {
		return fmt.Errorf("setting TDP: %w\n  (run 'sudo z13ctl-plus setup' to enable non-root access)", err)
	}
	// The daemonless path keeps no record of the desired curve: when
	// returning to a safe sustained limit, lift a previously forced safety
	// curve back to firmware auto control.
	if pl1 <= cli.TDPMaxSafe {
		cli.LiftHighTDPFanCurve()
	}
	fmt.Printf("TDP set to %dW\n", watts)
	return nil
}

func runTdpReset() error {
	if dryRunFlag {
		cli.DryRunTdpReset()
		return nil
	}

	if handled, err := api.SendTdpReset(); handled {
		if err != nil {
			return err
		}
		fmt.Println("TDP override reset to physical profile defaults")
		return nil
	}

	profile := readCurrentProfile()
	if err := cli.SetProfile(profile); err != nil {
		return fmt.Errorf("resetting TDP for %s profile: %w\n  (run 'sudo z13ctl-plus setup' to enable non-root access)", profile, err)
	}
	fmt.Printf("TDP reset to %s profile defaults\n", profile)
	return nil
}

// parsePLOverrides returns the effective PL1/PL2/PL3 values, applying
// per-PL flag overrides when set. Non-zero overrides replace the unified watts value.
func parsePLOverrides(watts int) (pl1, pl2, pl3 int, err error) {
	pl1, pl2, pl3 = watts, watts, watts
	if tdpPL1Flag != "" {
		pl1, err = strconv.Atoi(tdpPL1Flag)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid --pl1 value %q: must be an integer", tdpPL1Flag)
		}
	}
	if tdpPL2Flag != "" {
		pl2, err = strconv.Atoi(tdpPL2Flag)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid --pl2 value %q: must be an integer", tdpPL2Flag)
		}
	}
	if tdpPL3Flag != "" {
		pl3, err = strconv.Atoi(tdpPL3Flag)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid --pl3 value %q: must be an integer", tdpPL3Flag)
		}
	}
	return pl1, pl2, pl3, nil
}

func init() {
	tdpCmd.Flags().BoolVar(&tdpGetFlag, "get", false, "Print current TDP power limits")
	tdpCmd.Flags().StringVar(&tdpSetFlag, "set", "", "Set TDP power limit in watts")
	tdpCmd.Flags().BoolVar(&tdpResetFlag, "reset", false, "Reset TDP to physical profile defaults")
	tdpCmd.Flags().StringVar(&tdpPL1Flag, "pl1", "", "Override PL1/SPL (watts)")
	tdpCmd.Flags().StringVar(&tdpPL2Flag, "pl2", "", "Override PL2/sPPT (watts)")
	tdpCmd.Flags().StringVar(&tdpPL3Flag, "pl3", "", "Override PL3/fPPT (watts)")
	tdpCmd.Flags().BoolVar(&tdpForceFlag, "force", false, "Allow sustained TDP (PL1) above 75W (up to 93W)")
	rootCmd.AddCommand(tdpCmd)
}
