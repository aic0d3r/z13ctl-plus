// Package cmd implements the z13ctl-plus command shipped by z13ctl-plus.
// Each file in this package defines exactly one subcommand.
// CLI support utilities (color parsing, dry-run display) live in internal/cli.
//
// root.go — Cobra root command for z13ctl-plus.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Version is the current release. Override at build time:
//
//	go build -ldflags "-X github.com/aic0d3r/z13ctl-plus/cmd.Version=1.2.3" .
var Version = "2.0.0"

var (
	deviceFlag   string
	dryRunFlag   bool
	noButtonFlag bool
)

var rootCmd = &cobra.Command{
	Use:     "z13ctl-plus",
	Version: Version,
	Short:   "System control for the ASUS ROG Flow Z13",
	Long: `z13ctl-plus - system control for the 2025 ASUS ROG Flow Z13

Controls keyboard and lightbar RGB via Linux hidraw, performance profile and
battery charge limit via asus-wmi sysfs, and boot sound and panel overdrive
via asus-armoury firmware-attributes.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRun: func(_ *cobra.Command, _ []string) {
		useInvokingUserRuntimeDir()
	},
}

// useInvokingUserRuntimeDir lets `sudo z13ctl-plus ...` reach the invoking
// user's daemon: sudo drops XDG_RUNTIME_DIR, so the socket would be looked up
// under /tmp and the command would silently bypass the daemon.
func useInvokingUserRuntimeDir() {
	uid := os.Getenv("SUDO_UID")
	if os.Geteuid() != 0 || uid == "" || os.Getenv("XDG_RUNTIME_DIR") != "" {
		return
	}
	if dir := "/run/user/" + uid; dirExists(dir) {
		_ = os.Setenv("XDG_RUNTIME_DIR", dir)
	}
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// noteDirect tells the user a change bypassed the daemon (not running), so it
// is neither saved nor restored after sleep or reboot.
func noteDirect() {
	fmt.Fprintln(os.Stderr, "note: z13ctl-plus daemon not running; applied directly. It will not be restored after sleep or reboot.")
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&deviceFlag, "device", "", "Target device: keyboard, lightbar, or a hidraw path (default: all)")
	rootCmd.PersistentFlags().BoolVar(&dryRunFlag, "dry-run", false, "Preview changes without applying them")
	rootCmd.PersistentFlags().BoolVar(&noButtonFlag, "no-button", false, "Disable the Armoury Crate button watcher (daemon only)")
}
