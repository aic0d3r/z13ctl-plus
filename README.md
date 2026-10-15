# z13ctl+ — RGB, fan curves, TDP & undervolt control for the ASUS ROG Flow Z13 and AMD Strix Halo

<div align="center">

[![Release](https://img.shields.io/github/v/release/aic0d3r/z13ctl-plus)](https://github.com/aic0d3r/z13ctl-plus/releases)
[![AUR](https://img.shields.io/aur/version/z13ctl-plus-bin)](https://aur.archlinux.org/packages/z13ctl-plus-bin)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

[What Plus Adds](#what-plus-adds) •
[Tablet stack](#tablet-stack) •
[Install](#install) •
[Quick Start](#quick-start) •
[Documentation](#documentation) •
[License](#license)

</div>

Linux control for the **2025 ASUS ROG Flow Z13** (GZ302): RGB keyboard and
lightbar lighting, custom fan curves, TDP/PPT power limits, CPU undervolting
via Curve Optimizer, battery charge limiting, boot sound and panel overdrive,
plus power automation, AMD P-State policy, coordinated display state, and
telemetry — through one CLI and a systemd user daemon.

Built for the Z13, and the daemon **reports what the machine actually has**
(`capabilities` in `get-state`). On other AMD Strix Halo hardware it starts
without ASUS interfaces, answers "not supported on this machine" for controls it
cannot drive, and lets clients such as Z13GUI+ hide them. That support is
experimental until more machines are reported — see
[Compatibility](#compatibility).

`z13ctl-plus` is an independently named fork of
[`dahui/z13ctl`](https://github.com/dahui/z13ctl) with its own command, socket,
state, and packages, so it installs alongside upstream — enable only one
project's hardware daemon at a time. The daemon protocol and Go API stay
compatible; client integration details are in the
[API documentation](https://aic0d3r.github.io/z13ctl-plus/api/).

## What Plus Adds

- **Whole-system power automation** — capture named presets and assign separate
  configurations to plugged-in and battery operation
- **Composable tuning** — keep the physical profile independent from fan-curve,
  TDP, and undervolt overrides; reset or restore each without disturbing the others
- **AMD P-State policy** — manage minimum CPU frequency, EPP, and boost through
  presets and the API
- **Coordinated display state** — serialize KDE posture, rotation, scale, and
  refresh changes for the internal panel without touching external displays
- **Expanded telemetry** — expose CPU, GPU, NPU, memory, VRAM, dual-fan, thermal,
  power, clock, and detailed battery data
- **Better hardware recovery** — restore lighting after lid-open events that do
  not suspend and preserve firmware zero-RPM behavior in the Quiet profile

> [!TIP]
> **Prefer a touch interface? Install
> [Z13GUI+](https://github.com/aic0d3r/z13gui-plus).** It exposes power automation,
> CPU policy, expanded telemetry, refresh rate, and tuning controls through the
> daemon. Display posture/rotation and scripting remain CLI/API workflows.

## Compatibility

| Machine | Status |
|---------|--------|
| ASUS ROG Flow Z13 (2025, GZ302) | **Supported** — every control |
| Other AMD Strix Halo laptop or mini PC | **Experimental** — telemetry, CPU policy and (with `ryzen_smu`) undervolt should work; lighting, fan curves, TDP, boot sound, panel overdrive and charge limit need ASUS hardware interfaces |
| Anything else | Not supported |

Commands for missing hardware fail with `not supported on this machine: no <what>`.
If you try it on another machine, please open a
[compatibility report](https://github.com/aic0d3r/z13ctl-plus/issues/new?template=compatibility_report.md).

## Tablet stack

`z13ctl-plus` is the display and settings authority in a three-repo tablet
stack for the Z13. Install the other two repos alongside it:

- [z13-tablet-kit](https://github.com/aic0d3r/z13-tablet-kit) — detects
  dock/folio/tablet posture, toggles the touchscreen, and provides touch
  scrolling and two-finger context menus
- **z13ctl-plus** (this repo) — owns internal-panel scale/rotation/refresh
  changes and persists tablet settings (desktop touch policy, two-finger hold,
  folio touchpad opt-in, scroll speed)
- [z13gui-plus](https://github.com/aic0d3r/z13gui-plus) — GTK4 overlay drawer
  exposing live posture/health and the tablet toggles

Flow: the kit detects posture and publishes it to this daemon, which applies
the display changes; the GUI shows state and persists settings back through the
daemon; the daemon projects them to a runtime settings file the kit reads and
applies live.

## Install

```sh
# Arch Linux (AUR)
yay -S z13ctl-plus-bin

# Debian / Ubuntu
sudo apt install ./z13ctl-plus_*.deb

# Fedora / RHEL
sudo dnf install ./z13ctl-plus_*.rpm

# Manual (from release tarball)
wget https://github.com/aic0d3r/z13ctl-plus/releases/download/v2.5.0/z13ctl-plus_2.5.0_linux_amd64.tar.gz
tar xzf z13ctl-plus_2.5.0_linux_amd64.tar.gz
sudo install -Dm755 z13ctl-plus /usr/local/bin/z13ctl-plus
sudo z13ctl-plus setup

# After stopping any other Z13 hardware daemon
systemctl --user enable --now z13ctl-plus.socket z13ctl-plus.service
```

See the [Installation guide](https://aic0d3r.github.io/z13ctl-plus/installation/) for
systemd service setup, source builds, v1-to-v2 migration, and uninstall
instructions. Packages leave the Plus user daemon disabled; enable it only after
stopping any other daemon that controls the same hardware.

## Quick Start

```sh
# Create/update the recommended AC and battery presets and apply the current one
z13ctl-plus preset restore-recommended

# Inspect and enable automatic source switching
z13ctl-plus preset list
z13ctl-plus preset policy --enable --ac "Plugged In" --battery "On Battery"

# Apply a named configuration manually
z13ctl-plus preset apply "On Battery"

# Publish tablet posture; rotation is an optional explicit override
z13ctl-plus display posture tablet
z13ctl-plus display rotation right

# Inspect expanded live telemetry and active overrides
z13ctl-plus status
```

## Documentation

Full documentation at **<https://aic0d3r.github.io/z13ctl-plus>**

- [Installation](https://aic0d3r.github.io/z13ctl-plus/installation/)
- [Quick Start](https://aic0d3r.github.io/z13ctl-plus/getting-started/)
- [Commands](https://aic0d3r.github.io/z13ctl-plus/commands/)
- [Daemon](https://aic0d3r.github.io/z13ctl-plus/daemon/)
- [API](https://aic0d3r.github.io/z13ctl-plus/api/)
- [Contributing](https://aic0d3r.github.io/z13ctl-plus/contributing/)

## License

[Apache 2.0](LICENSE)
