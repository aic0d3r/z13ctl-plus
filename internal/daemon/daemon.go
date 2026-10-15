// Package daemon implements the z13ctl-plus long-running daemon: Unix socket server,
// hardware device management, state persistence, and Armoury Crate button watcher.
//
// Designed as a systemd user service using two units:
//   - z13ctl-plus.socket  — systemd manages the socket fd (socket activation)
//   - z13ctl-plus.service — Type=notify, Restart=on-failure
//
// Can also be run directly for development: z13ctl-plus daemon.
//
// The daemon socket client (Send*, Subscribe, SocketPath) lives in the public
// api package: github.com/dahui/z13ctl/api.
package daemon

// daemon.go — Daemon struct, Run function, socket listener, and subscriber management.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/coreos/go-systemd/v22/activation"
	sddaemon "github.com/coreos/go-systemd/v22/daemon"

	"github.com/aic0d3r/z13ctl-plus/internal/aura"
	"github.com/aic0d3r/z13ctl-plus/internal/cli"
	"github.com/aic0d3r/z13ctl-plus/internal/hid"
	"github.com/dahui/z13ctl/api"
)

// Daemon holds the runtime state for the long-running z13ctl-plus process.
type Daemon struct {
	mu          sync.Mutex
	applyMu     sync.Mutex
	dev         *hid.Device // nil if no HID device was found at startup
	state       api.State
	powerSource string
	policyCh    chan struct{}
	powerCh     chan struct{} // UPower OnBattery changed
	displayMu   sync.Mutex
	display     displayIntent
	displayCh   chan struct{}
	// Guarded by applyMu. shortcut: in-memory only, so a restart forgets
	// per-posture scales; persist in state if users want them across logins.
	scalePosture  string
	postureScales map[string]float64

	subMu       sync.Mutex
	subscribers []net.Conn // long-lived connections subscribed to events

	buttonCh chan struct{}
}

// Run starts the daemon and blocks until ctx is cancelled. It opens HID devices,
// restores the last-saved state, starts the button watcher, and serves the
// Unix socket.
func Run(ctx context.Context, watchBtn bool) error {
	d := &Daemon{
		buttonCh:  make(chan struct{}, 4),
		policyCh:  make(chan struct{}, 1),
		powerCh:   make(chan struct{}, 1),
		displayCh: make(chan struct{}, 1),
	}

	var fresh bool
	var err error
	d.state, fresh, err = loadState()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	if projectErr := projectTabletSettings(d.state.TabletSettings); projectErr != nil {
		return fmt.Errorf("project tablet settings: %w", projectErr)
	}
	if seedErr := d.seedDefaultPresets(fresh); seedErr != nil {
		return fmt.Errorf("initialize power defaults: %w", seedErr)
	}
	policyManaged := d.applyInitialPowerPolicy()

	dev, err := hid.FindDevice("")
	if err != nil {
		slog.Warn("HID device not found; lighting commands will be unavailable", "err", err)
	} else {
		d.dev = dev
		// reopenAndRestore may replace d.dev on keyboard hotplug, so close
		// whatever the current device is at shutdown rather than the original.
		defer func() {
			d.mu.Lock()
			if d.dev != nil {
				d.dev.Close()
			}
			d.mu.Unlock()
		}()
		d.mu.Lock()
		applyErr := d.applyLightingState()
		d.mu.Unlock()
		if applyErr != nil {
			slog.Warn("failed to restore lighting state", "err", applyErr)
		}
	}

	// Avoid a redundant profile write because it briefly stops firmware fan
	// control, but always restore each independently active override.
	if !policyManaged && isPhysicalProfile(d.state.Profile) {
		current, _ := cli.ReadTrimmedFile(cli.FindProfilePath())
		if tuningErr := applyTuning(d.state, current != d.state.Profile); tuningErr != nil {
			slog.Warn("failed to restore profile tuning", "err", tuningErr)
		} else {
			slog.Info("profile tuning restored", "profile", d.state.Profile)
		}
	}

	// Restore battery charge limit if saved.
	if d.state.Battery > 0 {
		path := cli.FindBatteryThresholdPath()
		if batErr := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", d.state.Battery)), 0o644); batErr != nil {
			slog.Warn("failed to restore battery limit", "err", batErr)
		} else {
			slog.Info("battery limit restored", "limit", d.state.Battery)
		}
	}

	// Restore panel overdrive if enabled (firmware may not persist this across reboot).
	if d.state.PanelOverdrive != 0 {
		if poErr := cli.SetPanelOverdrive(d.state.PanelOverdrive); poErr != nil {
			slog.Warn("failed to restore panel overdrive", "err", poErr)
		} else {
			slog.Info("panel overdrive restored", "value", d.state.PanelOverdrive)
		}
	}

	if watchBtn {
		go watchButton(ctx, d.buttonCh)
	} else {
		slog.Info("Armoury Crate button watcher disabled")
	}

	go d.watchResume(ctx)

	go d.watchPowerPolicy(ctx)

	go d.watchDisplay(ctx)

	go d.watchHotplug(ctx)

	ln, err := d.getListener()
	if err != nil {
		return fmt.Errorf("socket: %w", err)
	}
	defer func() { _ = ln.Close() }()

	if _, err := sddaemon.SdNotify(false, sddaemon.SdNotifyReady); err != nil {
		slog.Warn("sd_notify READY failed", "err", err)
	}
	slog.Info("z13ctl-plus daemon ready", "socket", ln.Addr())

	go d.broadcastLoop(ctx)

	go func() {
		<-ctx.Done()
		_, _ = sddaemon.SdNotify(false, sddaemon.SdNotifyStopping)
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		go d.handleConn(conn)
	}
}

// getListener returns a net.Listener from systemd socket activation if available,
// otherwise creates a new Unix socket at socketPath().
func (d *Daemon) getListener() (net.Listener, error) {
	listeners, err := activation.Listeners()
	if err == nil && len(listeners) > 0 && listeners[0] != nil {
		slog.Info("using systemd socket activation")
		return listeners[0], nil
	}

	sock := api.SocketPath()
	if mkdirErr := os.MkdirAll(filepath.Dir(sock), 0o750); mkdirErr != nil {
		return nil, mkdirErr
	}
	if !ownedByMe(filepath.Dir(sock)) {
		return nil, fmt.Errorf("socket directory %s is not owned by the current user", filepath.Dir(sock))
	}
	// Never delete a socket another daemon is still serving.
	if c, dialErr := net.DialTimeout("unix", sock, time.Second); dialErr == nil {
		_ = c.Close()
		return nil, fmt.Errorf("daemon already running on %s", sock)
	}
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return nil, err
	}
	slog.Info("listening on Unix socket", "path", sock)
	return ln, nil
}

// broadcastLoop forwards Armoury Crate button presses to all subscribers
// until ctx is done, then closes all subscriber connections.
func (d *Daemon) broadcastLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			d.subMu.Lock()
			for _, c := range d.subscribers {
				_ = c.Close()
			}
			d.subscribers = nil
			d.subMu.Unlock()
			return
		case <-d.buttonCh:
			d.broadcast(response{Event: "gui-toggle"})
		}
	}
}

func (d *Daemon) broadcast(r response) {
	data, _ := json.Marshal(r)
	data = append(data, '\n')

	d.subMu.Lock()
	var alive []net.Conn
	for _, c := range d.subscribers {
		// A subscriber that stopped reading would otherwise block event
		// delivery for everyone; drop it when the write cannot complete.
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if _, err := c.Write(data); err == nil {
			alive = append(alive, c)
		} else {
			_ = c.Close()
		}
	}
	d.subscribers = alive
	d.subMu.Unlock()
}

func (d *Daemon) addSubscriber(conn net.Conn) {
	d.subMu.Lock()
	d.subscribers = append(d.subscribers, conn)
	d.subMu.Unlock()
}

// applyZone applies a LightingState to a specific HID device or zone.
func applyZone(dev *hid.Device, ls api.LightingState) error {
	if !ls.Enabled {
		return aura.TurnOff(dev)
	}
	mode, err := aura.ModeFromString(ls.Mode)
	if err != nil {
		return err
	}
	speed, err := aura.SpeedFromString(ls.Speed)
	if err != nil {
		return err
	}
	r, g, b, err := cli.ParseColor(ls.Color)
	if err != nil {
		return err
	}
	r2, g2, b2, err := cli.ParseColor(ls.Color2)
	if err != nil {
		return err
	}
	return aura.Apply(dev, mode, r, g, b, r2, g2, b2, speed, uint8(ls.Brightness))
}

// reopenAndRestore re-enumerates the HID device and re-applies the saved lighting
// state. It is called after the detachable keyboard is reattached, where the
// keyboard appears as a new hidraw node that the old d.dev no longer references.
// Returns true on success; false (with a logged warning) if the device cannot be
// reopened yet — e.g. udev has not finished applying hidraw permissions — so the
// caller can retry.
func (d *Daemon) reopenAndRestore() bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	dev, err := hid.FindDevice("")
	if err != nil {
		slog.Warn("hotplug: failed to reopen HID device", "err", err)
		return false
	}
	if d.dev != nil {
		d.dev.Close()
	}
	d.dev = dev
	if err := d.applyLightingState(); err != nil {
		slog.Warn("hotplug: failed to restore lighting", "err", err)
		return false
	}
	slog.Info("keyboard reattached; lighting restored")
	return true
}

// applyLightingState restores lighting from the saved state. d.dev must be non-nil.
// If per-device states are saved (d.state.Devices), each zone is restored independently;
// otherwise the all-device state (d.state.Lighting) is applied to all zones.
func (d *Daemon) applyLightingState() error {
	if len(d.state.Devices) > 0 {
		for _, name := range []string{"keyboard", "lightbar"} {
			ls := d.state.Lighting
			if dl, ok := d.state.Devices[name]; ok {
				ls = dl
			}
			target, ferr := d.dev.FilteredView(name)
			if ferr != nil {
				continue // zone not present on this system
			}
			if err := applyZone(target, ls); err != nil {
				return err
			}
			if ls.Enabled {
				slog.Info("lighting restored", "zone", name, "mode", ls.Mode, "brightness", ls.Brightness)
			} else {
				slog.Info("lighting restored (off)", "zone", name)
			}
		}
		return nil
	}
	if err := applyZone(d.dev, d.state.Lighting); err != nil {
		return err
	}
	if d.state.Lighting.Enabled {
		slog.Info("lighting restored", "zone", "all", "mode", d.state.Lighting.Mode, "brightness", d.state.Lighting.Brightness)
	} else {
		slog.Info("lighting restored (off)", "zone", "all")
	}
	return nil
}

// ownedByMe reports whether dir is owned by the current user, so the /tmp
// fallback cannot be pre-created by another local user.
func ownedByMe(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil {
		return false
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}
