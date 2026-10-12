package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/dahui/z13ctl/api"
	"github.com/spf13/cobra"
)

var (
	tabletDisableTouchscreenInDesktop bool
	tabletTwoFingerHoldContextMenu    bool
	tabletSettingsTouchpadEnabled     bool
	tabletScrollSensitivity           int
	tabletScrollSpeed                 int
	tabletKeepDisplayScale            bool

	tabletPosture            string
	tabletFolioPresent       bool
	tabletHealthy            bool
	tabletError              string
	tabletLastSeen           int64
	tabletTouchscreenEnabled bool
	tabletTouchScrollEnabled bool
	tabletStatusTouchpad     bool
)

var tabletCmd = &cobra.Command{
	Use:   "tablet",
	Short: "Configure and monitor the optional tablet integration",
}

var tabletSettingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Print tablet settings as JSON",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		handled, state, err := api.SendGetState()
		if err != nil {
			return err
		}
		if !handled {
			return fmt.Errorf("tablet settings require the z13ctl-plus daemon")
		}
		if state == nil {
			return fmt.Errorf("daemon returned no state")
		}
		data, err := json.MarshalIndent(state.TabletSettings, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	},
}

var tabletSettingsSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Replace all tablet settings",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		settings := api.TabletSettings{
			DisableTouchscreenInDesktop: tabletDisableTouchscreenInDesktop,
			TwoFingerHoldContextMenu:    tabletTwoFingerHoldContextMenu,
			TouchpadEnabled:             tabletSettingsTouchpadEnabled,
			ScrollSensitivity:           tabletScrollSensitivity,
			ScrollSpeed:                 tabletScrollSpeed,
			KeepDisplayScale:            tabletKeepDisplayScale,
		}
		handled, err := api.SendTabletSettingsSet(settings)
		if err != nil {
			return err
		}
		if !handled {
			return fmt.Errorf("tablet settings require the z13ctl-plus daemon")
		}
		return nil
	},
}

var tabletReportCmd = &cobra.Command{
	Use:   "report",
	Short: "Report live tablet integration status",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		status := api.TabletIntegration{
			Posture:            tabletPosture,
			FolioPresent:       tabletFolioPresent,
			Healthy:            tabletHealthy,
			Error:              tabletError,
			LastSeen:           tabletLastSeen,
			TouchscreenEnabled: tabletTouchscreenEnabled,
			TouchScrollEnabled: tabletTouchScrollEnabled,
			TouchpadEnabled:    tabletStatusTouchpad,
		}
		handled, err := api.SendTabletIntegrationReport(status)
		if err != nil {
			return err
		}
		if !handled {
			return fmt.Errorf("tablet reporting requires the z13ctl-plus daemon")
		}
		return nil
	},
}

func init() {
	tabletSettingsSetCmd.Flags().BoolVar(&tabletDisableTouchscreenInDesktop, "disable-touchscreen-in-desktop", true, "Disable touchscreen in desktop posture")
	tabletSettingsSetCmd.Flags().BoolVar(&tabletTwoFingerHoldContextMenu, "two-finger-hold-context-menu", false, "Emit the context-menu key on a stationary two-finger hold")
	tabletSettingsSetCmd.Flags().BoolVar(&tabletSettingsTouchpadEnabled, "touchpad-enabled", false, "Enable the folio touchpad")
	tabletSettingsSetCmd.Flags().IntVar(&tabletScrollSensitivity, "scroll-sensitivity", 3, "Touch scroll sensitivity level (1-5)")
	tabletSettingsSetCmd.Flags().IntVar(&tabletScrollSpeed, "scroll-speed", 2, "Touch scroll speed level (1-5)")
	tabletSettingsSetCmd.Flags().BoolVar(&tabletKeepDisplayScale, "keep-display-scale", false, "Never change the display scale on posture changes")
	for _, name := range []string{"disable-touchscreen-in-desktop", "two-finger-hold-context-menu", "touchpad-enabled", "scroll-sensitivity", "scroll-speed", "keep-display-scale"} {
		_ = tabletSettingsSetCmd.MarkFlagRequired(name)
	}

	tabletReportCmd.Flags().StringVar(&tabletPosture, "posture", "", "Current posture (desktop, laptop, tablet)")
	tabletReportCmd.Flags().BoolVar(&tabletFolioPresent, "folio-present", false, "Whether the keyboard folio is present")
	tabletReportCmd.Flags().BoolVar(&tabletHealthy, "healthy", false, "Whether the integration is healthy")
	tabletReportCmd.Flags().StringVar(&tabletError, "error", "", "Current integration error, or an empty string")
	tabletReportCmd.Flags().Int64Var(&tabletLastSeen, "last-seen", 0, "Heartbeat Unix timestamp")
	tabletReportCmd.Flags().BoolVar(&tabletTouchscreenEnabled, "touchscreen-enabled", false, "Whether the touchscreen is enabled")
	tabletReportCmd.Flags().BoolVar(&tabletTouchScrollEnabled, "touch-scroll-enabled", false, "Whether touch scrolling is enabled")
	tabletReportCmd.Flags().BoolVar(&tabletStatusTouchpad, "touchpad-enabled", false, "Whether the touchpad is enabled")
	for _, name := range []string{"posture", "folio-present", "healthy", "error", "last-seen", "touchscreen-enabled", "touch-scroll-enabled", "touchpad-enabled"} {
		_ = tabletReportCmd.MarkFlagRequired(name)
	}

	tabletSettingsCmd.AddCommand(tabletSettingsSetCmd)
	tabletCmd.AddCommand(tabletSettingsCmd, tabletReportCmd)
	rootCmd.AddCommand(tabletCmd)
}
