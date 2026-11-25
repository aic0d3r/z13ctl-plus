package cmd

// off.go — "off" subcommand: turn all lighting off.

import (
	"fmt"

	"github.com/aic0d3r/z13ctl-plus/internal/aura"
	"github.com/aic0d3r/z13ctl-plus/internal/cli"
	"github.com/aic0d3r/z13ctl-plus/internal/hid"
	"github.com/dahui/z13ctl/api"

	"github.com/spf13/cobra"
)

var offCmd = &cobra.Command{
	Use:   "off",
	Short: "Turn all lighting off",
	RunE: func(_ *cobra.Command, _ []string) error {
		if dryRunFlag {
			cli.DryRunOff()
			return nil
		}

		if handled, err := api.SendOff(deviceFlag); handled {
			if err != nil {
				return err
			}
			fmt.Println("Lighting off.")
			return nil
		}

		dev, err := hid.FindDevice(deviceFlag)
		if err != nil {
			return err
		}
		defer dev.Close()

		if err := aura.TurnOff(dev); err != nil {
			return err
		}
		fmt.Println("Lighting off.")
		noteDirect()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(offCmd)
}
