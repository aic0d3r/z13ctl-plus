# Ecosystem, compatibility & fork details

## Z13 stack

| Project | Purpose |
|---|---|
| **z13ctl-plus** (this repo) | Hardware daemon + CLI; owns internal-panel scale/rotation/refresh and persists tablet settings |
| [z13gui-plus](https://github.com/aic0d3r/z13gui-plus) | GTK4 overlay drawer: touch UI for the daemon, live posture/health, tablet toggles |
| [z13-tablet-kit](https://github.com/aic0d3r/z13-tablet-kit) | Detects dock/folio/tablet posture, toggles the touchscreen, touch scrolling and two-finger context menus |
| [z13-backlight-quirk](https://github.com/aic0d3r/z13-backlight-quirk) | DKMS kernel module that lowers the Z13's minimum screen brightness (AUR `z13-backlight-quirk-dkms`). Not needed once the upstream dri-devel patch ships in your kernel |

### Tablet flow

The kit detects posture and publishes it to this daemon, which applies the
display changes. The GUI shows state and persists settings back through the
daemon. The daemon projects them to a runtime settings file the kit reads and
applies live. Persisted tablet settings: desktop touch policy, two-finger hold,
folio touchpad opt-in, scroll speed.

## Compatibility

| Machine | Status |
|---------|--------|
| ASUS ROG Flow Z13 (2025, GZ302) | **Supported** — every control |
| Other AMD Strix Halo laptop or mini PC | **Experimental** — telemetry, CPU policy and (with `ryzen_smu`) undervolt should work; lighting, fan curves, TDP, boot sound, panel overdrive and charge limit need ASUS hardware interfaces |
| Anything else | Not supported |

The daemon reports what the machine actually has (`capabilities` in
`get-state`). On other hardware it starts without ASUS interfaces, answers
`not supported on this machine: no <what>` for controls it cannot drive, and
lets clients such as Z13GUI+ hide them. If you try it on another machine, please
open a [compatibility report](https://github.com/aic0d3r/z13ctl-plus/issues/new?template=compatibility_report.md).

## Fork of dahui/z13ctl

`z13ctl-plus` is an independently named fork of
[`dahui/z13ctl`](https://github.com/dahui/z13ctl) with its own command, socket,
state, and packages, so it installs alongside upstream — enable only one
project's hardware daemon at a time. The daemon protocol and Go API stay
compatible; see the [API documentation](api.md) for client integration.
