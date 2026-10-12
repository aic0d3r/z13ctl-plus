# API Module

The `api/` module is a standalone Go library for communicating with the
z13ctl-plus daemon from external tools — GUI frontends, Decky plugins, shell
integrations, or anything else that wants to control z13ctl-plus programmatically.

## Import

```go
import "github.com/dahui/z13ctl/api"
```

The module is deliberately stdlib-only (no third-party dependencies) so that
integrations can pull it in without inheriting the CLI's dependency tree.

!!! warning "Canonical import path, fork API source"
    z13ctl-plus retains the upstream import path for API and wire compatibility,
    but its fork-sourced `SocketPath` targets only the Plus socket. External
    clients such as z13gui-plus must build against the matching fork API source,
    not the upstream source at the same import path. For a local checkout:

    ```go
    require github.com/dahui/z13ctl/api v1.6.0

    replace github.com/dahui/z13ctl/api => ../z13ctl-plus/api
    ```

    Released clients should use the corresponding published fork `api/v*` source
    as their replacement. Do not combine the upstream API source with the Plus
    daemon and expect socket discovery to work.

It is a separate Go module at `./api` with its own `go.mod`. If you are working
on both the main binary and the API library simultaneously, create a
[go.work](https://go.dev/ref/mod#workspaces) file:

```sh
go work init . ./api
```

---

## Connection model

All `Send*` functions open a fresh Unix socket connection to the daemon, send
one JSON request, read one JSON response, and close the connection. This is
intentionally simple and stateless.

If the daemon is not running (connection refused), every `Send*` function returns
`(false, nil)` — the first return value (`handled bool`) signals whether the
daemon was reached. Callers can use this to decide whether to fall back to direct
hardware access.

```go
handled, err := api.SendApply("", "FF0000", "000000", "static", "normal", 3)
if !handled {
    // daemon not running; do your own HID access here
}
```

`Subscribe` follows the same pattern but holds the connection open to receive a
stream of events.

---

## Socket path

```go
path := api.SocketPath()
// $XDG_RUNTIME_DIR/z13ctl-plus/z13ctl-plus.sock
// (or /tmp/z13ctl-plus/z13ctl-plus.sock)

settingsPath := api.TabletSettingsPath()
// $XDG_RUNTIME_DIR/z13ctl-plus/tablet-settings.json
```

---

## Examples

**Apply lighting:**

```go
// Static cyan at full brightness on all devices
handled, err := api.SendApply("", "00FFFF", "000000", "static", "normal", 3)

// Breathe between red and blue on the keyboard only
handled, err := api.SendApply("keyboard", "FF0000", "0000FF", "breathe", "slow", 3)
```

**System settings:**

```go
// Battery limit
handled, limit, err := api.SendBatteryLimitGet()
handled, err       := api.SendBatteryLimitSet(80)

// Performance profile
handled, profile, err := api.SendProfileGet()
handled, err          := api.SendProfileSet("performance")

// Boot sound and panel overdrive
handled, err := api.SendBootSoundSet(0)
handled, err := api.SendPanelOverdriveSet(1)

// Live AMD P-State controls
handled, err := api.SendCPUMinFrequencySet(625000) // kHz
handled, err := api.SendCPUEPPSet("balance_power")
handled, err := api.SendCPUBoostSet(false)

// Fan curves (applied to both fans simultaneously)
handled, value, err := api.SendFanCurveGet()
handled, err         := api.SendFanCurveSet("48:2,53:22,57:30,60:43,63:56,65:68,70:89,76:102")
handled, err         := api.SendFanCurveReset()

// TDP (PPT power limits)
handled, value, err := api.SendTdpGet()
handled, err         := api.SendTdpSet("50", "", "", "", false)  // all PPTs to 50W
handled, err         := api.SendTdpReset()

// Undervolt (Curve Optimizer — requires ryzen_smu kernel module)
handled, value, err := api.SendUndervoltGet()
handled, err         := api.SendUndervoltSet("-20")  // CPU CO -20
handled, err         := api.SendUndervoltReset()

// Reset fan, TDP, and undervolt overrides together
handled, err := api.SendTuningReset()

// Named power presets and automatic source switching
handled, err := api.SendPresetSave("Daily")
handled, err := api.SendPresetSet("Daily", api.Preset{Profile: "balanced", RefreshRate: 60})
handled, err := api.SendPresetApply("Daily")
handled, err := api.SendPresetDelete("Daily")
handled, err := api.SendRestoreRecommended()
handled, err := api.SendPowerPolicySet(true, "Plugged In", "On Battery")

// Optional tablet companion settings and live heartbeat
handled, err := api.SendTabletSettingsSet(api.TabletSettings{
    DisableTouchscreenInDesktop: true,
    TwoFingerHoldContextMenu:    false,
    TouchpadEnabled:             false,
    ScrollSensitivity:           3,
    ScrollSpeed:                 2,
    KeepDisplayScale:            false,
})
handled, err := api.SendTabletIntegrationReport(api.TabletIntegration{
    Posture: "tablet", Healthy: true, LastSeen: time.Now().Unix(),
})
```

**Full state snapshot (for GUI initialization):**

```go
handled, state, err := api.SendGetState()
if handled && err == nil {
    fmt.Println("lighting mode:", state.Lighting.Mode)
    fmt.Println("profile:", state.Profile)
    fmt.Println("battery limit:", state.Battery)
    fmt.Println("fan curve:", state.FanCurve)
    fmt.Println("fan override active:", state.FanCurveActive)
    fmt.Println("fan safety active:", state.FanSafetyActive)
    fmt.Println("tdp:", state.TDP)
    fmt.Println("tdp override active:", state.TDPActive)
    fmt.Println("undervolt:", state.Undervolt)
    fmt.Println("undervolt override active:", state.UndervoltActive)
    fmt.Println("undervolt available:", state.UndervoltAvailable)
    fmt.Println("CPU power:", state.CPUPower)
    fmt.Println("APU temp:", state.Temperature, "°C")
    fmt.Println("fan RPM:", state.FanRPM)
    fmt.Println("tablet settings:", state.TabletSettings)
    fmt.Println("tablet integration:", state.TabletIntegration) // latest live report, if any
}
```

**Subscribe to daemon events:**

```go
ch, cancel, err := api.Subscribe([]string{"gui-toggle", "state-changed"})
if ch == nil {
    // daemon not running
}
defer cancel()
for event := range ch {
    fmt.Println("received:", event)
    // Fetch state again when event == "state-changed".
}
```

## Full API reference

See [pkg.go.dev](https://pkg.go.dev/github.com/dahui/z13ctl/api) for the compatible
exported surface. The source displayed there is upstream and targets the upstream
socket; Plus clients still need the matching fork API source described above.
