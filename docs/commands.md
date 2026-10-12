# Commands

!!! tip "Beginners: consider Z13GUI+ instead"
    This page is the complete CLI reference. Driving the more advanced features
    (custom fan curves, TDP, undervolting) from the command line makes sense for
    Linux veterans and scripting, but if you're newer to Linux you'll likely
    have an easier time with **[Z13GUI+](https://github.com/aic0d3r/z13gui-plus)**, the
    touch-friendly graphical frontend that exposes all of these commands as
    point-and-tap controls.

## Global Flags

These flags apply to every command.

| Flag | Description |
|------|-------------|
| `--device <name\|path>` | Target a single device: `keyboard`, `lightbar`, or a `/dev/hidrawN` path. Without this flag all matching devices are targeted. |
| `--dry-run` | Preview what would be sent or written without making any changes. Works for all commands including `setup`. |
| `--no-button` | Disable the Armoury Crate button watcher (daemon only). Use when another tool needs exclusive access to the button device. |

---

## apply

Apply a lighting effect to the keyboard backlight, the edge lightbar, or both.

```
z13ctl-plus apply [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--color` | `FF0000` | Primary color: 6-digit hex (`RRGGBB`) or a [named color](#colors) |
| `--color2` | `000000` | Secondary color for `breathe` mode: 6-digit hex or name |
| `--mode` | `static` | Lighting mode (see table below) |
| `--speed` | `normal` | Animation speed: `slow`, `normal`, `fast` |
| `--brightness` | `high` | Brightness level: `off`, `low`, `medium`, `high` |
| `--list-colors` | | Print all named colors with swatches and exit |

**Modes:**

| Mode | Description | `--color` | `--color2` | `--speed` |
|------|-------------|:---------:|:----------:|:---------:|
| `static` | Solid color | yes | — | — |
| `breathe` | Fade between two colors | yes | yes | yes |
| `cycle` | Auto-cycle full spectrum | — | — | yes |
| `rainbow` | Rainbow wave across zones | — | — | yes |
| `strobe` | Rapid flash | yes | — | yes |

All modes accept `--brightness`.

```sh
z13ctl-plus apply --color cyan --brightness high
z13ctl-plus apply --mode rainbow --speed slow
z13ctl-plus apply --mode breathe --color hotpink --color2 blue --speed slow
z13ctl-plus apply --list-colors
```

---

## brightness

Set the brightness level without changing the current lighting mode or color.

```
z13ctl-plus brightness <level>
```

`<level>` is one of: `off`, `low`, `medium`, `high`

```sh
z13ctl-plus brightness medium
z13ctl-plus brightness off
```

---

## off

Turn off all lighting zones (or a specific zone with `--device`).

```
z13ctl-plus off
```

```sh
z13ctl-plus off
z13ctl-plus off --device lightbar
```

---

## profile

Get or set the system performance profile via the asus-wmi `platform_profile`
sysfs interface. Root or group access required; see [setup](#setup).

```
z13ctl-plus profile [flags]
```

| Flag | Description |
|------|-------------|
| `--get` | Print the active performance profile |
| `--set <profile>` | Set the performance profile |

Valid profiles: `quiet`, `balanced`, `performance`. Profile changes preserve and
reapply active fan, TDP, and undervolt overrides when the daemon is running.

```sh
z13ctl-plus profile --get
z13ctl-plus profile --set performance
z13ctl-plus profile --set balanced
```

!!! note
    When the daemon is running, setting a profile also updates
    `power-profiles-daemon` (if installed) to the equivalent PPD profile.

---

## preset

Save, apply, and automatically select named performance presets. Presets capture
the physical profile, fan/TDP/undervolt overrides, CPU minimum/EPP/boost,
panel overdrive, and display refresh rate. The daemon must be running.

```sh
z13ctl-plus preset list
z13ctl-plus preset save Gaming
z13ctl-plus preset apply Gaming
z13ctl-plus preset delete Gaming
z13ctl-plus preset restore-recommended
z13ctl-plus preset policy --enable --ac "Plugged In" --battery "On Battery"
z13ctl-plus preset policy --disable
```

The first daemon start on a fresh installation sets an 80% charge limit and
creates two deterministic presets. Existing state files are migrated as needed
but are never replaced by fresh defaults:

| Preset | Defaults |
|--------|----------|
| `Plugged In` | Balanced profile, hardware-minimum CPU frequency, `balance_performance` EPP, boost on, panel overdrive on, 180 Hz |
| `On Battery` | Balanced profile, hardware-minimum CPU frequency, `balance_power` EPP, boost off, panel overdrive off, 60 Hz |

`preset restore-recommended` creates or updates only these two presets, assigns
them to AC and battery, and applies the preset for the current source. Other
presets and the configured battery charge limit are preserved.

Automatic switching runs in the user daemon, not a root udev rule. It changes
the KDE display mode through KWin/KScreen and only acts at startup or an actual
AC transition. A manually applied preset remains active until the next
power-source transition.

Saving an existing name updates that preset. A preset assigned to AC or battery
cannot be deleted until another preset is assigned in its place.

---

## display

Publish posture and optional explicit rotation intent to the daemon's
internal-panel coordinator. `z13-tablet-kit` publishes posture; these commands
require the daemon and never fall back to direct KScreen writes.

```sh
z13ctl-plus display posture desktop
z13ctl-plus display posture laptop
z13ctl-plus display posture tablet
z13ctl-plus display rotation none
z13ctl-plus display rotation left
```

Scale changes only when the posture changes, restoring the scale last used in that
posture (defaults: 1.5 desktop/laptop, 1.75 tablet); a scale set in System Settings
is kept. Desktop and laptop use normal rotation; tablet preserves Plasma's current rotation unless an explicit rotation intent was sent.
The coordinator combines requested refresh, scale, and rotation changes into one
transaction for `eDP-1`, never changes external outputs, waits for dock topology
to settle, and skips all writes while the internal panel is disabled.

---

## tablet

Configure and report the optional `z13-tablet-kit` companion through the daemon:

```sh
# Inspect the authoritative settings as JSON.
z13ctl-plus tablet settings

# Replace all settings atomically (all six flags are required).
z13ctl-plus tablet settings set \
  --disable-touchscreen-in-desktop=true \
  --two-finger-hold-context-menu=false \
  --touchpad-enabled=false \
  --scroll-sensitivity=3 \
  --scroll-speed=2 \
  --keep-display-scale=false

# Publish one live status/heartbeat (all eight flags are required).
z13ctl-plus tablet report \
  --posture=tablet \
  --folio-present=false \
  --healthy=true \
  --error="" \
  --last-seen=1785800000 \
  --touchscreen-enabled=true \
  --touch-scroll-enabled=true \
  --touchpad-enabled=false
```

Posture must be `desktop`, `laptop`, or `tablet`; both scroll levels must be
1-5. `last-seen` is a Unix timestamp in seconds. These commands require the
daemon and return nonzero if it cannot be reached or rejects the report.

Defaults are touchscreen disabled in desktop posture, two-finger hold context
menu disabled, touchpad disabled, scroll sensitivity 3 (the kit's current step
90), and scroll speed 2. When enabled, two stationary fingers held for about
650 ms and released emit the context-menu key (KEY_MENU); moving either finger
cancels the hold into scrolling.

---

## batterylimit

Get or set the battery charge limit via the Linux ACPI `power_supply` sysfs
interface. Root or group access required; see [setup](#setup).

```
z13ctl-plus batterylimit [flags]
```

| Flag | Description |
|------|-------------|
| `--get` | Print the current battery charge limit (percentage) |
| `--set <percent>` | Set the battery charge limit (40–100) |

Writing `100` removes any limit (charges to full).

```sh
z13ctl-plus batterylimit --get
z13ctl-plus batterylimit --set 80
```

---

## bootsound

Get or set the POST boot sound via the `asus-armoury` firmware-attributes sysfs
interface. Root or group access required; see [setup](#setup).

```
z13ctl-plus bootsound [flags]
```

| Flag | Description |
|------|-------------|
| `--get` | Print the current boot sound setting (`0` or `1`) |
| `--set <value>` | Set boot sound: `0` = off, `1` = on |

```sh
z13ctl-plus bootsound --get
z13ctl-plus bootsound --set 0
```

---

## paneloverdrive

Get or set display panel refresh overdrive via the `asus-armoury`
firmware-attributes sysfs interface. Root or group access required; see
[setup](#setup).

```
z13ctl-plus paneloverdrive [flags]
```

| Flag | Description |
|------|-------------|
| `--get` | Print the current panel overdrive setting (`0` or `1`) |
| `--set <value>` | Set panel overdrive: `0` = off, `1` = on |

```sh
z13ctl-plus paneloverdrive --get
z13ctl-plus paneloverdrive --set 1
```

---

## fancurve

Get, set, or reset custom fan curves via the asus-wmi hwmon sysfs interface.
Both physical fans cool the same APU, so the same curve is always applied to
both fans simultaneously. Root or group access required; see [setup](#setup).

```
z13ctl-plus fancurve [flags]
```

| Flag | Description |
|------|-------------|
| `--get` | Print the current fan curve, mode, RPM, and APU temperature |
| `--set <curve>` | Set a custom 8-point fan curve (applied to both fans) |
| `--reset` | Reset both fans to firmware auto mode |

**Curve format:** 8 comma-separated `temp:speed` pairs. Speed can be a PWM
value (0–255) or a percentage with a `%` suffix (0–100%). Both formats can be
mixed in the same curve.

```
"48:2,53:22,57:30,60:43,63:56,65:68,70:89,76:102"   # PWM values
"48:1%,53:9%,57:12%,60:17%,63:22%,65:27%,70:35%,76:40%"  # percentages
```

**Validation rules:**

- Exactly 8 points required
- Temperatures must be monotonically increasing (0–120 &deg;C)
- Speed values must be non-decreasing (0–255 PWM or 0–100%)

```sh
# Read current fan curves
z13ctl-plus fancurve --get

# Set a custom fan curve using PWM values (both fans)
z13ctl-plus fancurve --set "48:2,53:22,57:30,60:43,63:56,65:68,70:89,76:102"

# Set a custom fan curve using percentages
z13ctl-plus fancurve --set "48:1%,53:9%,57:12%,60:17%,63:22%,65:27%,70:35%,76:40%"

# Reset both fans to auto mode
z13ctl-plus fancurve --reset
```

---

## tdp

Get, set, or reset TDP (Thermal Design Power) limits via the asus-nb-wmi PPT
(Package Power Tracking) sysfs attributes. Root or group access required; see
[setup](#setup).

```
z13ctl-plus tdp [flags]
```

| Flag | Description |
|------|-------------|
| `--get` | Print current PPT values |
| `--set <watts>` | Set all PPT limits to the specified wattage |
| `--reset` | Switch to balanced profile (firmware manages PPT and fan curves) |
| `--pl1 <watts>` | Override PL1/SPL independently |
| `--pl2 <watts>` | Override PL2/sPPT independently |
| `--pl3 <watts>` | Override PL3/fPPT independently |
| `--force` | Allow sustained TDP (PL1) above 75W (up to 93W). Burst limits (PL2/PL3) are allowed up to 93W without `--force`. When PL1 exceeds 75W, fans are set to a 50% minimum curve; custom curves must keep all PWM values at or above 127 (50%). |

**PPT attributes:**

| Attribute | Limit | Description |
|-----------|-------|-------------|
| `ppt_pl1_spl` | PL1 — Sustained | Continuous power budget the APU can draw indefinitely. This is your effective base TDP. |
| `ppt_pl2_sppt` | PL2 — Short-term boost | Power the APU can draw for several seconds before throttling to PL1. |
| `ppt_fppt` | PL3 — Fast boost | Maximum instantaneous power for millisecond-scale spikes. |
| `ppt_apu_sppt` | APU short-term | APU-specific short-term limit; automatically mirrors PL2. |
| `ppt_platform_sppt` | Platform short-term | Platform-level short-term limit; automatically mirrors PL2. |

With `--set`, all three limits default to the same value. Use `--pl1`, `--pl2`,
and `--pl3` to set them independently — a stepped configuration like
`--set 45 --pl2 55 --pl3 65` sustains 45W with short bursts to 55W and
instantaneous peaks to 65W.

Without an active TDP override, the physical profile controls PPT dynamically.
Setting TDP keeps that physical profile and activates an independent override.

!!! note "PPT readback values"
    The values shown by `--get` are the kernel driver's cached values, which may
    not reflect the actual EC limits (especially after a fresh boot or profile
    change). Use `ryzenadj -i` if you need ground-truth PPT readings.

**Safety:**

- Default range: 5–75W
- `--force` extends the range to 5–93W
- When sustained PL1 exceeds 75W, both fans use a temporary 50%+ safety curve
  before TDP is written. The desired custom curve, or auto mode, returns when
  PL1 is safe again. If the fan write fails, TDP is not applied.

```sh
# Read current TDP values
z13ctl-plus tdp --get

# Set all PPT limits to 50W
z13ctl-plus tdp --set 50

# Set with individual PL overrides
z13ctl-plus tdp --set 45 --pl2 55 --pl3 60

# Force high TDP (fans use the temporary safety curve)
z13ctl-plus tdp --set 85 --force

# Reset TDP to the current profile's firmware defaults
z13ctl-plus tdp --reset
```

---

## undervolt

Get or set CPU Curve Optimizer (CO) offsets via the `ryzen_smu` kernel
module. Negative values reduce voltage (undervolt), improving efficiency and
thermals without reducing performance. Root or group access required; see
[setup](#setup).

```
z13ctl-plus undervolt [flags]
```

| Flag | Description |
|------|-------------|
| `--get` | Print current CO offset (from daemon state) |
| `--set <value>` | Set all-core CPU CO offset (0 to -40) |
| `--reset` | Reset CPU CO to stock (0) |

CO values have no sysfs readback, so `--get` returns daemon state. Undervolt is
an independent override and does not change the physical performance profile.

CO is volatile: values reset on reboot and sleep/resume. The daemon reapplies
it automatically on startup and resume while the undervolt override is active.

**Safety limits (matching G-Helper defaults):**

| Parameter | Range |
|-----------|-------|
| CPU CO | 0 to -40 |

**Requires:** `ryzen_smu` kernel module. Install via:

- **Arch/CachyOS:** `ryzen_smu-dkms-git` (AUR)
- **Other distros:** build from [amkillam/ryzen_smu](https://github.com/amkillam/ryzen_smu) source

!!! warning "Strix Halo requires the amkillam fork"
    The original `leogx9r/ryzen_smu` does not support Strix Halo. Use the
    [amkillam/ryzen_smu](https://github.com/amkillam/ryzen_smu) fork instead.

If the module is not installed, undervolt commands return a helpful error.

```sh
# Read current CO value
z13ctl-plus undervolt --get

# Set CPU CO to -20
z13ctl-plus undervolt --set -20

# Reset to stock voltage
z13ctl-plus undervolt --reset

# Preview without applying
z13ctl-plus --dry-run undervolt --set -20
```

---

## status

Display a summary of all system metrics in a single view: APU temperature, fan
speed and mode, performance profile, TDP power limits, undervolt status, and
battery charge level with charge limit.

```
z13ctl-plus status
```

This command is read-only and takes no flags. All values are read directly from
sysfs (except undervolt, which has no sysfs readback — shows `ryzen_smu`
module availability).

```sh
z13ctl-plus status
# APU:       62°C
# Fans:      4200 RPM, mode: auto
# Profile:   balanced
# TDP:       52W (PL1) / 71W (PL2) / 70W (PL3)
# Undervolt: available (ryzen_smu loaded)
# Battery:   74% (limit: 80%)
```

---

## list

List all matching hidraw devices and show whether each has Aura support.

```
z13ctl-plus list
```

Useful for diagnosing missing devices or verifying that `setup` worked. Does
not require the daemon to be running.

---

## setup

Install udev rules and a boot service granting a group read/write access to
the ASUS HID devices, performance profile, battery charge limit, firmware
attributes (boot sound, panel overdrive), hwmon fan curve attributes,
asus-nb-wmi PPT power limit attributes for TDP control, and ryzen_smu sysfs
files for undervolting (if the module is loaded).

```
sudo z13ctl-plus setup [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--group` | `users` | Group to grant device access to |

Use `--dry-run` to preview exactly what would be written — no root required:

```sh
z13ctl-plus --dry-run setup    # preview (no root needed)
sudo z13ctl-plus setup         # apply
```

After running setup, log out and back in (or run `newgrp <group>`) for the
group membership to take effect in your current session.

For a detailed explanation of what `setup` installs and why the battery limit
requires a separate systemd service, see [Installation](installation.md#what-setup-does).

---

## migrate-state

Validate and copy state from the legacy v1 `z13ctl` namespace into the v2 Plus
namespace:

```sh
z13ctl-plus --dry-run migrate-state
z13ctl-plus migrate-state
```

The command copies only when the Plus state file is absent. It does not remove
the source or other legacy artifacts and does not change any services. See
[Migrate from v1 to v2](installation.md#migrate-from-v1-to-v2) for the required
manual service switch and provenance warning.

---

## daemon

Start the z13ctl-plus daemon. Normally started automatically via the systemd socket
unit — see [Daemon](daemon.md). You can also start it directly for testing.

```
z13ctl-plus daemon
```

```sh
z13ctl-plus daemon               # with Armoury Crate button watcher
z13ctl-plus --no-button daemon   # without button watcher
```

When the daemon is running, all other commands (`apply`, `brightness`, `off`,
`profile`, `batterylimit`, `bootsound`, `paneloverdrive`, `fancurve`, `tdp`,
`undervolt`, `status`)
route through the daemon socket automatically. If the daemon is not running
they fall back to direct hardware or sysfs access.

---

## Colors

Named colors accepted by `--color` and `--color2`. Any 6-digit hex value
(`RRGGBB`, without `#`) is also accepted.

Run `z13ctl-plus apply --list-colors` to see ANSI true-color swatches in your terminal.

| Name | Hex | Name | Hex |
|------|-----|------|-----|
| `red` | `FF0000` | `blue` | `0000FF` |
| `crimson` | `DC143C` | `navy` | `000080` |
| `orangered` | `FF4500` | `indigo` | `4B0082` |
| `coral` | `FF7F50` | `blueviolet` | `8A2BE2` |
| `orange` | `FF8000` | `purple` | `800080` |
| `gold` | `FFD700` | `magenta` | `FF00FF` |
| `yellow` | `FFFF00` | `deeppink` | `FF1493` |
| `chartreuse` | `7FFF00` | `hotpink` | `FF69B4` |
| `green` | `00FF00` | `violet` | `EE82EE` |
| `springgreen` | `00FF7F` | `turquoise` | `40E0D0` |
| `aquamarine` | `7FFFD4` | `brown` | `A52A2A` |
| `teal` | `008080` | `white` | `FFFFFF` |
| `cyan` | `00FFFF` | `deepskyblue` | `00BFFF` |
| `dodgerblue` | `1E90FF` | `royalblue` | `4169E1` |
