# Daemon

The z13ctl-plus daemon is a long-running background process that provides things
ordinary one-shot CLI invocations cannot:

- **State persistence** — saves your last-applied lighting, profile, battery,
  fan curve, TDP, and undervolt settings to
  `~/.local/state/z13ctl-plus/state.json`
  and restores them automatically at every boot.
- **Sleep/resume recovery** — watches for system resume events via D-Bus and
  reapplies lighting and volatile settings (fan curves, TDP, undervolt) that
  are lost during sleep.
- **Keyboard reattach recovery** — detects the detachable keyboard being removed
  and reattached, reopens the HID device, and re-applies the saved keyboard
  lighting (the firmware does not restore it on its own).
- **HID device ownership** — holds the hidraw devices open continuously so that
  commands arrive instantly rather than waiting to reopen the device each time.
- **Armoury Crate button events** — captures `KEY_PROG3` (the dedicated Armoury
  Crate button) and broadcasts a `gui-toggle` event to any connected subscribers
  (see [API](api.md)).

All CLI commands (`apply`, `brightness`, `off`, `profile`, `batterylimit`,
`bootsound`, `paneloverdrive`, `fancurve`, `tdp`, `undervolt`, `status`) automatically route through the
daemon socket when it is running. If the daemon is not running they fall back to direct hardware or
sysfs access transparently — there is no user-visible difference other than
persistence.

!!! warning "Run only one hardware daemon"
    z13ctl-plus uses an independent socket and can be co-installed with upstream,
    but both daemons control the same hardware. Enable only one. The Plus CLI
    checks only the Plus socket and falls back to direct hardware access when it
    is unavailable, so do not mix an upstream daemon with Plus direct-fallback
    commands.

---

## Systemd setup (recommended)

z13ctl-plus ships two independently named systemd user units that use socket
activation:

- **`z13ctl-plus.socket`** — systemd creates and manages the Unix socket. The daemon
  is started on first use and does not run if nothing has connected.
- **`z13ctl-plus.service`** — `Type=notify`, `Restart=on-failure`. The daemon sends
  `sd_notify READY` when it is listening.

The units target `graphical-session.target`, so they work in both desktop
environments (KDE, GNOME) and Steam Gaming Mode (gamescope session).

Install and enable:

```sh
install -Dm644 contrib/systemd/user/z13ctl-plus.socket \
    ~/.config/systemd/user/z13ctl-plus.socket
install -Dm644 contrib/systemd/user/z13ctl-plus.service \
    ~/.config/systemd/user/z13ctl-plus.service
systemctl --user daemon-reload
systemctl --user enable --now z13ctl-plus.socket z13ctl-plus.service
```

Or, if you built from source:

```sh
make install-service
```

---

## Managing the service

```sh
# Check status
systemctl --user status z13ctl-plus.socket
systemctl --user status z13ctl-plus.service

# View live logs
journalctl --user -u z13ctl-plus.service -f

# Restart the daemon (e.g., after a config change)
systemctl --user restart z13ctl-plus.service
```

### Remove the user service

```sh
systemctl --user disable --now z13ctl-plus.socket z13ctl-plus.service
rm -f ~/.config/systemd/user/z13ctl-plus.socket \
      ~/.config/systemd/user/z13ctl-plus.service
systemctl --user daemon-reload
```

---

## Running without systemd

Start the daemon directly for testing or on systems without systemd:

```sh
z13ctl-plus daemon
```

To disable the Armoury Crate button watcher (e.g., when another tool such as
a Steam controller mapper needs exclusive access to the button device):

```sh
z13ctl-plus --no-button daemon
```

The daemon listens on a Unix socket at:

```
$XDG_RUNTIME_DIR/z13ctl-plus/z13ctl-plus.sock
```

(falls back to `/tmp/z13ctl-plus/z13ctl-plus.sock` if `XDG_RUNTIME_DIR` is not
set).

---

## State file

The daemon persists state to:

```
$XDG_STATE_HOME/z13ctl-plus/state.json
```

When `XDG_STATE_HOME` is unset, this is normally
`~/.local/state/z13ctl-plus/state.json`. The file is written atomically after
every successful command. It stores:

- `lighting` — mode, color, color2, speed, brightness, enabled flag
- `devices` — per-device overrides (keyboard/lightbar can have independent state)
- `profile` — last-set performance profile
- `battery_limit` — last-set charge limit
- `fan_curve` — custom curve points and mode (applied to both fans)
- `tdp` — PL1, PL2, and PL3 power limits in watts
- `undervolt` — CPU Curve Optimizer offset and active flag (preserved across
  profile switches for recall; `undervolt-get` includes the current profile so
  clients can distinguish active vs saved values)
- `presets` — named performance/display snapshots
- `power_policy` — automatic AC and battery preset assignments
- `active_preset` — the last fully applied preset
- `tablet_settings` — authoritative settings for the optional tablet companion

On `get-state` requests the daemon also populates `temperature` (APU die
temperature in °C), `fan_rpm` (fan speed in RPM), `cpu_power` (live cpufreq
minimum, EPP, boost, and available limits), and `undervolt_available`
(whether the `ryzen_smu` kernel module is present) from live sysfs reads.
These are not persisted — they are real-time sensor values.

The latest `tablet_integration` heartbeat is likewise live-only and is empty
after a daemon restart. Tablet settings are also projected atomically with mode
`0600` to `$XDG_RUNTIME_DIR/z13ctl-plus/tablet-settings.json` (or the `/tmp`
fallback used by the socket) for read-only use by `z13-tablet-kit`:

```json
{
  "disable_touchscreen_in_desktop": true,
  "two_finger_hold_context_menu": false,
  "touchpad_enabled": false,
  "scroll_sensitivity": 3,
  "scroll_speed": 2,
  "keep_display_scale": false
}
```

The opt-in context-menu gesture emits the context-menu key after a quick
stationary two-finger hold (~650 ms); movement cancels it.

On startup the daemon reads this file and independently restores each active fan
curve, TDP, and undervolt override before accepting connections.

!!! note "Raw hidrawN paths are not persisted"
    Commands sent with `--device /dev/hidraw2` (a raw path) are applied but
    not saved — raw device numbers are transient and may change across reboots.
    Use `keyboard` or `lightbar` by name for persistent per-zone settings.

## AC/battery preset switching

The daemon reads external power state from `/sys/class/power_supply` and applies
the configured AC or battery preset after the source settles. It does not infer
AC state from battery charging status, so charge thresholds and full batteries
do not produce false transitions.

Display refresh is submitted as intent to the daemon's display coordinator.
The coordinator waits for posture from `z13-tablet-kit`, serializes internal
refresh, scale, and rotation changes, and combines pending values into one
`kscreen-doctor` transaction. External outputs are never included. Desktop
posture waits ten seconds for dock topology to settle, and a disabled `eDP-1`
is left untouched. KWin remains the output owner.

Manual preset changes are not continuously overwritten. On resume, an active
named preset is restored in full; otherwise the daemon restores the current
manual physical profile and active tuning overrides. Display posture intent is then
reconciled without replacing the manual choice with a source default.

---

## Sleep/resume recovery

Several hardware settings are volatile — they are lost when the system enters
sleep (suspend/hibernate) and must be reapplied on resume:

- **Lighting** — RGB lighting is turned off by the hardware on sleep
- **Fan curves** — custom PWM curves reset to firmware defaults on sleep
- **TDP (PPT)** — power limits revert to the firmware profile's defaults
- **Undervolt (Curve Optimizer)** — CO offsets reset to stock on every sleep cycle

The daemon monitors D-Bus for `org.freedesktop.login1.Manager.PrepareForSleep`
signals from systemd-logind. When the system resumes (`PrepareForSleep(false)`),
the daemon restores lighting (regardless of profile) and each independently
active volatile override from its saved state.

This happens transparently with no user intervention. You can verify it worked
by checking the daemon logs after a resume:

```sh
journalctl --user -u z13ctl-plus.service --since "5 minutes ago"
```

---

## Keyboard reattach recovery

The Z13's keyboard folio is detachable, and it is a separate USB HID device
(`0b05:1a30`) from the lightbar (`0b05:18c6`), which lives in the tablet body.
When you detach the keyboard it loses power and its RGB goes dark; when you
reattach it, the firmware brings it back **unlit** — the previously applied
effect is not restored.

The daemon opens the hidraw devices once at startup, so a reattached keyboard
appears as a brand-new device node that the original handle no longer references.
To recover, the daemon polls sysfs every couple of seconds for the keyboard
reappearing. On detecting it, the daemon reopens the HID devices and re-applies
the saved lighting state — honoring per-device overrides, so a keyboard-specific
color/mode is restored exactly as you last set it.

This requires no user intervention; the keyboard relights within a few seconds of
reattachment. If your `z13ctl-plus setup` udev rules are in place, the reattached
node is granted access automatically. You can verify it in the daemon logs:

```sh
journalctl --user -u z13ctl-plus.service -f
# On reattach: keyboard reattached; lighting restored
```

!!! note "Daemon required"
    This recovery only happens while the daemon is running. Without it, re-run
    your `apply` command (or press the Armoury Crate button in Z13GUI+) after
    reattaching the keyboard.

---

## Armoury Crate button

The daemon watches the ASUS WMI hotkeys input device for `KEY_PROG3` (key code
202) — the physical Armoury Crate button on the Z13. When pressed, it broadcasts
a `gui-toggle` event to all connected subscribers.

External tools can subscribe to this event via the API:

```go
ch, cancel, err := api.Subscribe([]string{"gui-toggle"})
```

See the [API](api.md) page for details.

### InputPlumber compatibility

On gaming distributions such as Bazzite and ChimeraOS, [InputPlumber](https://github.com/ShadowBlip/InputPlumber)
ships a built-in device profile for the ROG Flow Z13 (`50-rog_flow_z13.yaml`)
that grabs `"Asus WMI hotkeys"` as a managed source device. This creates an
exclusive evdev conflict: z13ctl-plus cannot open the device and will log:

```
button watcher stopped; retrying err="open /dev/input/eventN: permission denied"
```

**Workaround:** Create an override config that marks `"Asus WMI hotkeys"` as
`ignore: true`. This tells InputPlumber to leave that device unmanaged so
z13ctl-plus can grab it exclusively, while preserving all other InputPlumber
functionality (controller emulation, touchpad, etc.).

First, check whether the built-in config exists on your system:

```sh
cat /usr/share/inputplumber/devices/50-rog_flow_z13.yaml
```

If found, create the override directory if needed, then save the override file:

```sh
sudo mkdir -p /etc/inputplumber/devices.d
```

Create `/etc/inputplumber/devices.d/50-rog_flow_z13.yaml` with `ignore: true` added
to the keyboard source device:

```yaml
# /etc/inputplumber/devices.d/50-rog_flow_z13.yaml
version: 1
kind: CompositeDevice
name: ASUS ROG Flow Z13 (2025)
single_source: false

matches:
  - dmi_data:
      board_name: GZ302EA
      sys_vendor: ASUSTeK COMPUTER INC.

source_devices:
  - group: keyboard
    ignore: true        # allow z13ctl-plus to grab this device exclusively
    evdev:
      name: Asus WMI hotkeys
      handler: event*

options:
  auto_manage: true

target_devices:
  - xbox-elite
  - touchpad
  - keyboard

capability_map_id: flw1
```

!!! note
    Always place overrides under `/etc/inputplumber/devices.d/` — never edit files
    under `/usr/share/inputplumber/devices/` directly, as those are owned by the
    package and will be overwritten on upgrades.

    If your model's `board_name` differs from `GZ302EA`, verify it with:
    ```sh
    cat /sys/class/dmi/id/board_name
    ```

After saving the file, restart InputPlumber:

```sh
sudo systemctl restart inputplumber.service
```

Then restore device permissions that the previous InputPlumber instance may have
changed (InputPlumber restricts device node access while managing a device, and may
not fully restore permissions on shutdown):

```sh
sudo z13ctl-plus setup --perms-only
```

Then confirm z13ctl-plus can grab the button:

```sh
journalctl --user -u z13ctl-plus.service -f
# Should show: watching Armoury Crate button path=/dev/input/eventN
```

Alternatively, disable the button watcher entirely and let InputPlumber handle
the button exclusively:

```sh
z13ctl-plus --no-button daemon
```
