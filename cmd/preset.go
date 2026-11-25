package cmd

// preset.go - named performance presets and AC/battery assignments.

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/dahui/z13ctl/api"

	"github.com/spf13/cobra"
)

var errNeedDaemon = errors.New("preset management requires the z13ctl-plus daemon")

var presetCmd = &cobra.Command{
	Use:   "preset",
	Short: "Manage named performance presets and automatic power switching",
}

// daemonState returns the daemon's state or an error naming what needs it.
func daemonState() (*api.State, error) {
	handled, state, err := api.SendGetState()
	if err != nil {
		return nil, err
	}
	if !handled {
		return nil, errNeedDaemon
	}
	return state, nil
}

// describePreset renders every setting a preset applies.
func describePreset(p api.Preset) string {
	parts := []string{p.Profile}
	if p.TDP != nil {
		parts = append(parts, fmt.Sprintf("TDP %d/%d/%d W", p.TDP.PL1SPL, p.TDP.PL2SPPT, p.TDP.FPPT))
	} else {
		parts = append(parts, "TDP stock")
	}
	if p.Undervolt != nil {
		parts = append(parts, fmt.Sprintf("undervolt %d", p.Undervolt.CPUCO))
	}
	if c := p.CPUPower; c != nil {
		parts = append(parts, fmt.Sprintf("CPU min %d MHz, EPP %s, boost %s", c.MinFrequencyKHz/1000, c.EPP, onOff(c.Boost)))
	}
	if p.RefreshRate != 0 {
		parts = append(parts, fmt.Sprintf("%d Hz", p.RefreshRate))
	}
	if p.PanelOverdrive != nil {
		parts = append(parts, "overdrive "+onOff(*p.PanelOverdrive == 1))
	}
	return strings.Join(parts, ", ")
}

var presetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved presets and power assignments",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		state, err := daemonState()
		if err != nil {
			return err
		}
		for _, name := range slices.Sorted(maps.Keys(state.Presets)) {
			active := ""
			if name == state.ActivePreset {
				active = " (active)"
			}
			fmt.Printf("%s%s: %s\n", name, active, describePreset(state.Presets[name]))
		}
		if policy := state.PowerPolicy; policy != nil {
			fmt.Printf("Auto: %s; AC=%s; Battery=%s; source=%s\n",
				onOff(policy.Enabled), policy.ACPreset, policy.BatteryPreset, state.PowerSource)
		}
		return nil
	},
}

var presetRestoreRecommendedCmd = &cobra.Command{
	Use:   "restore-recommended",
	Short: "Restore and apply the recommended AC/battery presets",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		if dryRunFlag {
			fmt.Println("Would restore and apply the recommended power presets")
			return nil
		}
		handled, err := api.SendRestoreRecommended()
		if err != nil {
			return err
		}
		if !handled {
			return fmt.Errorf("restoring recommended presets requires the z13ctl-plus daemon")
		}
		fmt.Println("Recommended presets restored and applied; automatic AC/battery switching is on")
		return nil
	},
}

func presetAction(use, short, done string, send func(string) (bool, error)) *cobra.Command {
	return &cobra.Command{
		Use:   use + " NAME",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if dryRunFlag {
				fmt.Printf("Would %s preset %q\n", use, args[0])
				return nil
			}
			handled, err := send(args[0])
			if err != nil {
				return err
			}
			if !handled {
				return errNeedDaemon
			}
			fmt.Printf("Preset %q %s\n", args[0], done)
			return nil
		},
	}
}

var presetRenameCmd = &cobra.Command{
	Use:   "rename OLD NEW",
	Short: "Rename a preset, keeping its AC/battery assignment",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		if dryRunFlag {
			fmt.Printf("Would rename preset %q to %q\n", args[0], args[1])
			return nil
		}
		handled, err := api.SendPresetRename(args[0], args[1])
		if err != nil {
			return err
		}
		if !handled {
			return errNeedDaemon
		}
		fmt.Printf("Preset %q renamed to %q\n", args[0], args[1])
		return nil
	},
}

var presetEditFlags struct {
	profile, tdp, undervolt, epp, boost string
	cpuMin, refresh, overdrive         int
}

var presetEditCmd = &cobra.Command{
	Use:   "edit NAME",
	Short: "Change individual values of a saved preset",
	Long: `Change individual values of a saved preset; unspecified values are kept.
An active preset is applied again.

  --tdp PL1[:PL2[:PL3]] | none     sustained/short/fast power limits in W
  --undervolt N | none             Curve Optimizer offset (0 to -40)
  --refresh 60|180|0               0 leaves the refresh rate alone
  --panel-overdrive 0|1|-1         -1 leaves panel overdrive alone`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		state, err := daemonState()
		if err != nil {
			return err
		}
		p, ok := state.Presets[args[0]]
		if !ok {
			return fmt.Errorf("preset %q not found (create it with: preset save %s)", args[0], args[0])
		}
		if err := editPreset(cmd, &p); err != nil {
			return err
		}
		if dryRunFlag {
			fmt.Printf("Would set preset %q to: %s\n", args[0], describePreset(p))
			return nil
		}
		if _, err := api.SendPresetSet(args[0], p); err != nil {
			return err
		}
		fmt.Printf("Preset %q: %s\n", args[0], describePreset(p))
		return nil
	},
}

func editPreset(cmd *cobra.Command, p *api.Preset) error {
	f := presetEditFlags
	changed := cmd.Flags().Changed
	if changed("profile") {
		p.Profile = f.profile
	}
	if changed("tdp") {
		if f.tdp == "none" {
			p.TDP = nil
		} else {
			var w [3]int
			fields := strings.Split(f.tdp, ":")
			if len(fields) > 3 {
				return fmt.Errorf("--tdp: use PL1[:PL2[:PL3]]")
			}
			for i := range w {
				src := fields[min(i, len(fields)-1)]
				v, err := strconv.Atoi(src)
				if err != nil {
					return fmt.Errorf("--tdp: %q is not a number of watts", src)
				}
				w[i] = v
			}
			p.TDP = &api.TDPState{PL1SPL: w[0], PL2SPPT: w[1], FPPT: w[2], APUSPPT: w[1], PlatformSPPT: w[1]}
		}
	}
	if changed("undervolt") {
		if f.undervolt == "none" {
			p.Undervolt = nil
		} else {
			v, err := strconv.Atoi(f.undervolt)
			if err != nil {
				return fmt.Errorf("--undervolt: %q is not a number", f.undervolt)
			}
			p.Undervolt = &api.UndervoltState{CPUCO: v}
		}
	}
	if changed("cpu-min") || changed("epp") || changed("boost") {
		if p.CPUPower == nil {
			p.CPUPower = &api.CPUPowerPreset{}
			if cpu := currentCPUPreset(); cpu != nil {
				p.CPUPower = cpu
			}
		}
		if changed("cpu-min") {
			p.CPUPower.MinFrequencyKHz = f.cpuMin
		}
		if changed("epp") {
			p.CPUPower.EPP = f.epp
		}
		if changed("boost") {
			v, err := parseOnOff(f.boost)
			if err != nil {
				return fmt.Errorf("--boost: %w", err)
			}
			p.CPUPower.Boost = v
		}
	}
	if changed("refresh") {
		p.RefreshRate = f.refresh
	}
	if changed("panel-overdrive") {
		if f.overdrive < 0 {
			p.PanelOverdrive = nil
		} else {
			v := f.overdrive
			p.PanelOverdrive = &v
		}
	}
	return nil
}

func currentCPUPreset() *api.CPUPowerPreset {
	handled, state, err := api.SendGetState()
	if !handled || err != nil || state.CPUPower == nil {
		return nil
	}
	c := state.CPUPower
	return &api.CPUPowerPreset{MinFrequencyKHz: c.MinFrequencyKHz, EPP: c.EPP, Boost: c.Boost}
}

var (
	policyEnable  bool
	policyDisable bool
	policyAC      string
	policyBattery string
)

var presetPolicyCmd = &cobra.Command{
	Use:   "policy",
	Short: "Configure automatic AC/battery preset switching",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		if policyEnable == policyDisable {
			return fmt.Errorf("specify exactly one of --enable or --disable")
		}
		if dryRunFlag {
			fmt.Printf("Would set automatic preset switching: enabled=%t AC=%q Battery=%q\n", policyEnable, policyAC, policyBattery)
			return nil
		}
		if policyAC == "" || policyBattery == "" {
			handled, state, err := api.SendGetState()
			if err != nil {
				return err
			}
			if !handled || state.PowerPolicy == nil {
				return fmt.Errorf("specify both --ac and --battery")
			}
			if policyAC == "" {
				policyAC = state.PowerPolicy.ACPreset
			}
			if policyBattery == "" {
				policyBattery = state.PowerPolicy.BatteryPreset
			}
		}
		handled, err := api.SendPowerPolicySet(policyEnable, policyAC, policyBattery)
		if err != nil {
			return err
		}
		if !handled {
			return errNeedDaemon
		}
		if policyEnable {
			fmt.Printf("Automatic switching on: AC=%s, Battery=%s. Manual changes last until the next plug or unplug.\n", policyAC, policyBattery)
		} else {
			fmt.Println("Automatic switching off; settings from before it was turned on are restored")
		}
		return nil
	},
}

func init() {
	presetCmd.AddCommand(presetListCmd, presetRestoreRecommendedCmd, presetRenameCmd, presetEditCmd)
	presetCmd.AddCommand(presetAction("save", "Save current settings as a preset", "saved", api.SendPresetSave))
	presetCmd.AddCommand(presetAction("apply", "Apply a named preset", "applied", api.SendPresetApply))
	presetCmd.AddCommand(presetAction("delete", "Delete an unassigned preset", "deleted", api.SendPresetDelete))
	ef := presetEditCmd.Flags()
	ef.StringVar(&presetEditFlags.profile, "profile", "", "quiet, balanced, or performance")
	ef.StringVar(&presetEditFlags.tdp, "tdp", "", "PL1[:PL2[:PL3]] in W, or none")
	ef.StringVar(&presetEditFlags.undervolt, "undervolt", "", "Curve Optimizer offset, or none")
	ef.IntVar(&presetEditFlags.cpuMin, "cpu-min", 0, "CPU minimum frequency in kHz")
	ef.StringVar(&presetEditFlags.epp, "epp", "", "CPU energy performance preference")
	ef.StringVar(&presetEditFlags.boost, "boost", "", "CPU boost: on or off")
	ef.IntVar(&presetEditFlags.refresh, "refresh", 0, "Refresh rate: 60, 180, or 0 to leave alone")
	ef.IntVar(&presetEditFlags.overdrive, "panel-overdrive", -1, "Panel overdrive: 0, 1, or -1 to leave alone")
	presetPolicyCmd.Flags().BoolVar(&policyEnable, "enable", false, "Enable automatic switching")
	presetPolicyCmd.Flags().BoolVar(&policyDisable, "disable", false, "Disable automatic switching")
	presetPolicyCmd.Flags().StringVar(&policyAC, "ac", "", "Preset assigned to AC power")
	presetPolicyCmd.Flags().StringVar(&policyBattery, "battery", "", "Preset assigned to battery power")
	presetCmd.AddCommand(presetPolicyCmd)
	rootCmd.AddCommand(presetCmd)
}
