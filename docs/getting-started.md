# Quick Start

These examples assume you have [installed z13ctl-plus](installation.md) and run
`sudo z13ctl-plus setup`. Most direct hardware commands work without the daemon.
Named presets, automatic power-source switching, display intents, and persisted
state operations require it.

!!! tip "Prefer not to use the command line? Use Z13GUI+"
    **[Z13GUI+](https://github.com/aic0d3r/z13gui-plus)** provides touch-friendly
    controls for power automation, CPU policy, expanded telemetry, refresh rate,
    and tuning through the daemon. Display posture/rotation and scripting remain
    CLI/API workflows.

---

## Power automation

Create or update the recommended source-specific presets, assign them, and apply
the configuration for the current power source:

```sh
z13ctl-plus preset restore-recommended
z13ctl-plus preset policy --enable --ac "Plugged In" --battery "On Battery"
z13ctl-plus preset list
```

Apply any saved configuration manually without changing the assignments:

```sh
z13ctl-plus preset apply "On Battery"
```

The daemon preserves a manual selection until the next real AC/battery transition.

---

## Display coordination

Publish internal-panel posture and, when needed, an explicit rotation override
to the daemon's KDE display coordinator:

```sh
z13ctl-plus display posture tablet
z13ctl-plus display rotation right
```

The coordinator combines scale and preset refresh-rate changes without modifying
external displays. On a posture change it restores the scale last used in that
posture (defaults: 150% desktop/laptop, 175% tablet), so your own scale sticks. Set
`keep_display_scale` (GUI: "Keep my display scale") to never change it. Tablet
preserves Plasma's current rotation unless an explicit rotation intent was sent.

---

## Lighting

```sh
# Solid cyan at full brightness (keyboard + lightbar)
z13ctl-plus apply --color cyan --brightness high

# Breathing red, slow pulse
z13ctl-plus apply --mode breathe --color red --speed slow

# Breathing between two colors
z13ctl-plus apply --mode breathe --color hotpink --color2 blue

# Rainbow wave across both zones
z13ctl-plus apply --mode rainbow --speed normal

# Spectrum cycle, fast
z13ctl-plus apply --mode cycle --speed fast

# Strobe white
z13ctl-plus apply --mode strobe --color white

# Turn off all lighting
z13ctl-plus off
```

---

## Brightness

```sh
# Adjust brightness without changing mode or color
z13ctl-plus brightness high
z13ctl-plus brightness medium
z13ctl-plus brightness low
z13ctl-plus brightness off
```

---

## Performance profile

```sh
# Check current profile
z13ctl-plus profile --get

# Switch profiles
z13ctl-plus profile --set performance
z13ctl-plus profile --set balanced
z13ctl-plus profile --set quiet
```

---

## Battery charge limit

```sh
# Check current limit
z13ctl-plus batterylimit --get

# Cap charging at 80% (recommended for mostly-plugged-in use)
z13ctl-plus batterylimit --set 80

# Remove the limit (charge to 100%)
z13ctl-plus batterylimit --set 100
```

---

## Fan curves

Both physical fans cool the same APU, so the same curve is always applied to
both fans simultaneously.

```sh
# Check current fan curves
z13ctl-plus fancurve --get

# Set a custom fan curve using PWM values (8 temp:speed pairs, both fans)
z13ctl-plus fancurve --set "48:2,53:22,57:30,60:43,63:56,65:68,70:89,76:102"

# Or use percentages (0–100%)
z13ctl-plus fancurve --set "48:1%,53:9%,57:12%,60:17%,63:22%,65:27%,70:35%,76:40%"

# Reset both fans to firmware auto mode
z13ctl-plus fancurve --reset
```

---

## TDP control

TDP (Thermal Design Power) controls how much power the APU can draw. There are
three limits that form a hierarchy: PL1 is the sustained (continuous) limit,
PL2 allows short-term bursts above PL1 for several seconds, and PL3 allows
instantaneous spikes for milliseconds. Setting all three to the same value gives
a flat power cap; setting PL2 and PL3 higher allows bursty workloads to
temporarily exceed PL1.

Stock profiles (quiet/balanced/performance) manage TDP automatically. A custom
TDP value is an independent override and does not change the physical profile.

```sh
# Check current TDP/PPT values
z13ctl-plus tdp --get

# Set all PPT limits to 50W
z13ctl-plus tdp --set 50

# Set with individual PL overrides
z13ctl-plus tdp --set 45 --pl2 55 --pl3 60

# Force high TDP (above 75W, fans set to 50% minimum)
z13ctl-plus tdp --set 85 --force

# Reset TDP to the current profile's firmware defaults
z13ctl-plus tdp --reset
```

---

## Undervolting (Curve Optimizer)

Undervolting reduces CPU voltage via AMD Curve Optimizer (CO), lowering
temperatures and power draw without reducing performance. Requires the
`ryzen_smu` kernel module (optional — see [Installation](installation.md)).

CO values are volatile — they reset on reboot and sleep. The daemon reapplies
it automatically on startup and resume while the override is active.

```sh
# Check current CO value
z13ctl-plus undervolt --get

# Set CPU CO to -20
z13ctl-plus undervolt --set -20

# Reset to stock voltage
z13ctl-plus undervolt --reset
```

Safety limit (matching G-Helper defaults): CPU 0 to -40.

---

## Independent tuning overrides

Fan curve, TDP, and undervolt controls are independently active. Changing the
physical profile preserves and reapplies them; resetting one does not reset the
others.

```sh
# Set up independent overrides
z13ctl-plus fancurve --set "48:2,53:22,57:30,60:43,63:56,65:68,70:89,76:102"
z13ctl-plus tdp --set 50

# Change the physical profile without dropping overrides
z13ctl-plus profile --set balanced

# Reset only TDP
z13ctl-plus tdp --reset
```

---

## Per-device control

Use `--device` to target only the keyboard or lightbar:

```sh
# Keyboard to red, lightbar to blue
z13ctl-plus apply --color red --device keyboard
z13ctl-plus apply --color blue --device lightbar

# Turn off just the lightbar
z13ctl-plus off --device lightbar
```

---

## Preview without applying changes

`--dry-run` shows exactly what packets or writes would be sent, without
touching any hardware:

```sh
z13ctl-plus --dry-run apply --mode rainbow --speed fast
z13ctl-plus --dry-run setup
```

---

## See all named colors

```sh
z13ctl-plus apply --list-colors
```

This prints a live swatch table in your terminal. Any 6-digit hex value
(`RRGGBB`, without `#`) is also accepted wherever a color name is.

---

## Next steps

- [Commands](commands.md) — every flag and option for every command
- [Daemon](daemon.md) — set up the daemon for state persistence, boot
  restore, and sleep/resume recovery
- [Z13GUI+](https://github.com/aic0d3r/z13gui-plus) — graphical interface for all
  z13ctl-plus features (touch-friendly GTK4 overlay)
