package cmd

// camera.go — "camera" subcommand: turn the built-in camera on or off through
// its USB "authorized" sysfs attribute. Off disconnects the webcam and the IR
// camera until it is turned back on; the daemon reapplies "off" after reboot.

import (
	"fmt"
	"strconv"

	"github.com/aic0d3r/z13ctl-plus/internal/cli"
	"github.com/dahui/z13ctl/api"

	"github.com/spf13/cobra"
)

var (
	cameraGetFlag bool
	cameraSetFlag string
)

var cameraCmd = &cobra.Command{
	Use:   "camera",
	Short: "Turn the built-in camera on or off",
	Long: `Turn the built-in camera (webcam and IR camera) on or off.

Off disconnects the USB camera device, so no application can open it until it
is turned back on. When the daemon is running it remembers the choice and
reapplies it after reboot and resume; without the daemon the kernel turns the
camera back on at the next boot.

Values:
  0 — off
  1 — on`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		if !cameraGetFlag && cameraSetFlag == "" {
			return cmd.Help()
		}

		if cameraSetFlag != "" {
			value, err := strconv.Atoi(cameraSetFlag)
			if err != nil || (value != 0 && value != 1) {
				return fmt.Errorf("invalid value %q: must be 0 or 1", cameraSetFlag)
			}

			if dryRunFlag {
				cli.DryRunCamera(value)
				return nil
			}

			if handled, err := api.SendCameraSet(value); handled {
				if err != nil {
					return err
				}
				fmt.Printf("Camera set to %d\n", value)
				return nil
			}

			if err := cli.SetCamera(value); err != nil {
				return fmt.Errorf("setting camera: %w\n  (run 'sudo z13ctl-plus setup' to enable non-root access)", err)
			}
			fmt.Printf("Camera set to %d (not remembered: the daemon is not running)\n", value)
			return nil
		}

		value, err := cli.ReadCamera()
		if err != nil {
			return fmt.Errorf("reading camera: %w", err)
		}
		fmt.Println(value)
		return nil
	},
}

func init() {
	cameraCmd.Flags().BoolVar(&cameraGetFlag, "get", false, "Print whether the camera is on (1) or off (0)")
	cameraCmd.Flags().StringVar(&cameraSetFlag, "set", "", "Turn the camera on (1) or off (0)")
	rootCmd.AddCommand(cameraCmd)
}
