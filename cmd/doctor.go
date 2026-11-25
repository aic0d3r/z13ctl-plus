package cmd

// doctor.go — "doctor" subcommand: check the installation and print a fix for
// every problem (group, sysfs access, udev rules, boot service, daemon,
// upstream conflict, ryzen_smu, kernel features).

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"slices"
	"strconv"
	"strings"
	"syscall"

	"github.com/aic0d3r/z13ctl-plus/internal/cli"
	"github.com/dahui/z13ctl/api"

	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check permissions, services, the daemon and kernel support; print fixes",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		failed := runDoctor()
		if failed > 0 {
			return fmt.Errorf("%d problem(s) found", failed)
		}
		fmt.Println("\nEverything looks good.")
		return nil
	},
}

type doctor struct{ failed int }

func (d *doctor) ok(format string, args ...any) {
	fmt.Printf("  ok    "+format+"\n", args...)
}

func (d *doctor) warn(format string, args ...any) {
	fmt.Printf("  warn  "+format+"\n", args...)
}

// fail prints a problem and its fix.
func (d *doctor) fail(fix, format string, args ...any) {
	d.failed++
	fmt.Printf("  FAIL  "+format+"\n", args...)
	fmt.Printf("        fix: %s\n", fix)
}

func runDoctor() int {
	d := &doctor{}
	cfg, saved := loadSetupConfig()

	fmt.Println("Access")
	d.checkGroup(cfg.Group)
	d.checkWritable(cfg)
	if fileExists(rulesPath) || fileExists("/usr/lib/udev/rules.d/99-z13ctl-plus.rules") {
		d.ok("udev rules installed")
	} else {
		d.fail("sudo z13ctl-plus setup", "udev rules are not installed")
	}
	if out, _ := exec.Command("systemctl", "is-enabled", "z13ctl-plus-perms.service").Output(); strings.TrimSpace(string(out)) == "enabled" {
		d.ok("z13ctl-plus-perms.service enabled")
	} else {
		d.fail("sudo z13ctl-plus setup", "z13ctl-plus-perms.service is not enabled; permissions are lost at reboot")
	}
	if !saved {
		d.warn("no %s yet; run 'sudo z13ctl-plus setup' once to save the group", setupConfPath)
	}

	fmt.Println("Daemon")
	state := d.checkDaemon()

	fmt.Println("Hardware support")
	d.checkFeatures(cfg, state)

	return d.failed
}

func (d *doctor) checkGroup(group string) {
	g, err := user.LookupGroup(group)
	if err != nil {
		d.fail("sudo z13ctl-plus setup --group=<existing group>", "access group %q does not exist", group)
		return
	}
	me, err := user.Current()
	if err != nil {
		return
	}
	if me.Uid == "0" {
		d.warn("running as root; run doctor as your normal user to check its access")
		return
	}
	gids, _ := me.GroupIds()
	if !slices.Contains(gids, g.Gid) {
		d.fail(fmt.Sprintf("sudo usermod -aG %s %s, then log out and back in", group, me.Username),
			"user %s is not in group %s", me.Username, group)
		return
	}
	active, _ := os.Getgroups()
	gid, _ := strconv.Atoi(g.Gid)
	if !slices.Contains(active, gid) && os.Getgid() != gid {
		d.fail("log out and back in", "user %s was added to %s but this session predates it", me.Username, group)
		return
	}
	d.ok("user %s is in group %s", me.Username, group)
}

func (d *doctor) checkWritable(cfg setupConfig) {
	var denied []string
	targets := permissionTargets(cfg.Undervolt)
	for _, t := range targets {
		if syscall.Access(t.Path, 2 /* W_OK */) != nil {
			denied = append(denied, t.Path)
		}
	}
	if len(denied) == 0 {
		d.ok("all %d controlled files are writable", len(targets))
		return
	}
	shown := denied[:min(len(denied), 5)]
	more := ""
	if len(denied) > len(shown) {
		more = fmt.Sprintf(" (+%d more)", len(denied)-len(shown))
	}
	d.fail("sudo z13ctl-plus setup --perms-only (or log in again after a group change)",
		"%d file(s) not writable: %s%s", len(denied), strings.Join(shown, ", "), more)
}

func (d *doctor) checkDaemon() *api.State {
	if cli.UpstreamDaemonRunning() {
		d.fail("systemctl --user disable --now z13ctl.socket z13ctl.service",
			"the upstream z13ctl daemon is running; it fights z13ctl-plus over the hardware")
	}
	handled, state, err := api.SendGetState()
	switch {
	case err != nil:
		d.fail("check 'journalctl --user -u z13ctl-plus'", "daemon unreachable: %v", err)
		return nil
	case !handled:
		d.fail("systemctl --user enable --now z13ctl-plus.socket z13ctl-plus.service",
			"daemon not running; changes apply directly and are not restored after sleep or reboot")
		return nil
	}
	d.ok("daemon running (%s)", api.SocketPath())
	if state.TuningError != "" {
		d.fail("check 'journalctl --user -u z13ctl-plus'; the daemon keeps retrying",
			"saved overrides are not applied: %s", state.TuningError)
	}
	return state
}

func (d *doctor) checkFeatures(cfg setupConfig, state *api.State) {
	feature := func(present bool, name, fix string) {
		if present {
			d.ok("%s", name)
		} else {
			d.warn("%s missing (%s)", name, fix)
		}
	}
	feature(fileExists(cli.FindProfilePath()), "platform_profile", "asus-wmi not loaded?")
	feature(cli.FindFanCurveHwmonPath() != "", "custom fan curves (asus_custom_fan_curve hwmon)", "kernel without asus-wmi fan curve support")
	feature(fileExists("/sys/devices/platform/asus-nb-wmi/ppt_pl1_spl"), "TDP (asus-nb-wmi ppt_*)", "kernel without asus-nb-wmi PPT attributes")
	feature(fileExists(cli.FindPanelOverdrivePath()), "asus-armoury firmware attributes", "Linux 6.19+ or the asus-armoury module")
	feature(fileExists(cli.FindBatteryThresholdPath()), "battery charge limit", "no charge_control_end_threshold")

	switch {
	case !cli.SMUAvailable():
		d.warn("ryzen_smu not loaded; undervolt unavailable (install the amkillam ryzen_smu fork)")
	case !cfg.Undervolt:
		d.warn("ryzen_smu loaded but undervolt access not granted (sudo z13ctl-plus setup --undervolt)")
	case state == nil:
		// The probe writes a Curve Optimizer command, so only the daemon runs it.
		d.warn("ryzen_smu loaded; fork support is checked by the daemon (not running)")
	case state.UndervoltAvailable:
		d.ok("undervolt (ryzen_smu Curve Optimizer works)")
	default:
		d.fail("install the amkillam ryzen_smu fork (the leogx9r fork does not support Strix Halo)",
			"ryzen_smu is loaded but rejects Curve Optimizer commands")
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
