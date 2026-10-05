# z13ctl-plus — Project Context for Claude

## What this project is

`z13ctl-plus` is an independently named compatible fork of `dahui/z13ctl` for
controlling RGB lighting, fan curves, TDP (PPT power limits), and system settings
on the 2025 ASUS ROG Flow Z13. It preserves intentional API and daemon wire
compatibility, not runtime-name compatibility. Root Go module and repository:
`github.com/aic0d3r/z13ctl-plus`. License: Apache 2.0.

## v2 namespace invariants

- The only installed command is `z13ctl-plus`; never add a `z13ctl` alias.
- User units are `z13ctl-plus.socket` and `z13ctl-plus.service`. The system
  permissions unit is `z13ctl-plus-perms.service`; the udev file is
  `99-z13ctl-plus.rules`.
- Runtime socket: `$XDG_RUNTIME_DIR/z13ctl-plus/z13ctl-plus.sock`, falling back
  to `/tmp/z13ctl-plus/z13ctl-plus.sock` when `XDG_RUNTIME_DIR` is unset.
- State: `$XDG_STATE_HOME/z13ctl-plus/state.json`, normally
  `~/.local/state/z13ctl-plus/state.json`.
- The separate API module intentionally remains `github.com/dahui/z13ctl/api`.
  Fork API source targets only the Plus socket. External clients such as
  z13gui-plus must use a replacement selecting the matching fork API source;
  importing upstream source at the canonical path targets the upstream socket.
- Packages do not provide, conflict with, or replace upstream and can be
  co-installed. Package installation leaves the Plus user daemon disabled.
  Enable only one project's hardware daemon. The Plus CLI checks only the Plus
  socket, so do not mix an upstream daemon with Plus direct-fallback commands.
- v1 shared the legacy `z13ctl` runtime namespace. `z13ctl-plus migrate-state`
  validates and copies legacy state only when Plus state is absent. It never
  changes services or removes source state/artifacts. Legacy paths are
  provenance-ambiguous; cleanup must be explicit and limited to known fork files.

## Package layout

```
api/                         Public client API submodule (github.com/dahui/z13ctl/api)
  go.mod                     Separate module; stdlib only; importable by z13gui-plus and external tools
  types.go                   State, LightingState, FanCurvePoint, FanCurveState, TDPState, UndervoltState (exported protocol types)
  client.go                  SocketPath, Send*, Subscribe (all client functions)
  example_test.go            testable examples for all Send* and Subscribe functions
main.go                      entry point
cmd/                         Cobra subcommands
  root.go                    root command, Version var, dryRunFlag, deviceFlag, noButtonFlag
  apply.go                   apply lighting effect
  brightness.go              set brightness only
  daemon.go                  start the daemon (z13ctl-plus daemon)
  migrate_state.go           explicitly copy validated v1 state into the Plus namespace
  list.go                    list hidraw devices
  off.go                     turn lighting off
  profile.go                 get/set physical performance profile (platform_profile sysfs)
  preset.go                  named presets + automatic AC/battery assignment CLI
  display.go                 posture/rotation intent publisher for shared KScreen coordinator
  batterylimit.go            get/set battery charge limit (power_supply sysfs)
  bootsound.go               get/set POST boot sound (asus-armoury firmware-attributes)
  paneloverdrive.go          get/set panel refresh overdrive (asus-armoury firmware-attributes)
  fancurve.go                get/set/reset custom fan curves (hwmon sysfs)
  tdp.go                     get/set/reset TDP power limits (asus-nb-wmi PPT sysfs)
  undervolt.go               get/set/reset CPU Curve Optimizer offsets via ryzen_smu
  status.go                  display system status (temperature, fans, profile, TDP, battery)
  setup.go                   install udev rules; applySysfsPerms helper (HID, hwmon, PPT, firmware-attributes, ryzen_smu)
internal/
  aura/                      Aura HID protocol implementation
    aura.go                  Writer interface + Init/SetPower/SetBrightness/SetMode/Apply/TurnOff
    modes.go                 Mode and Speed constants + ModeFromString/SpeedFromString
  cli/                       CLI helpers shared by cmd/ and internal/daemon/
    cli.go                   package doc file only
    colors.go                named color table, ResolveColor, PrintColorList
    parse.go                 ParseColor, ParseBrightness
    sysfs.go                 FindProfilePath, SetProfile, FindBatteryThresholdPath, FindBootSoundPath, SetBootSound, FindPanelOverdrivePath, SetPanelOverdrive, ReadAPUTemperature, FindBatteryCapacityPath
    fan.go                   hwmon discovery, fan curve read/write (both fans), RPM read, mode control, ParseFanCurve
    tdp.go                   PPT sysfs helpers, safety constants (TDPMin/TDPMaxSafe/TDPMaxForced), ReadAllPPT, SetTDP, ResetTDP
    smu.go                   SMU sysfs communication: SMUAvailable, SMUProbeUndervolt, SendSMUCommand, response codes
    undervolt.go             Curve Optimizer commands: SetCurveOptimizer, ResetCurveOptimizer, ValidateCOValues, safety limits
    undervolt_test.go        encodeCOValue, ValidateCOValues, smuResponseError tests
    fan_test.go              ParseFanCurve, FanModeName tests
    dryrun.go                DryRunApply, DryRunOff, DryRunBrightness, DryRunProfile, DryRunBatteryLimit, DryRunBootSound, DryRunPanelOverdrive, DryRunFanCurve, DryRunFanCurveReset, DryRunTdp, DryRunTdpReset, DryRunUndervolt, DryRunUndervoltReset
  daemon/                    long-running daemon (socket server, state, button watcher)
    daemon.go                Package doc, Daemon struct, Run(), getListener() (socket activation)
    state.go                 XDG state file persistence (uses api.State/api.LightingState)
    button.go                evdev button watcher (KEY_PROG3 / Armoury Crate button on 2025 Z13)
    hotplug.go               detachable-keyboard reattach watcher (polls sysfs, reopens HID + restores lighting)
    preset.go                named preset capture/apply and AC/battery policy watcher
    display.go               serialized/coalescing internal-panel display coordinator
    server.go                JSON request handler; handleConn(), dispatch(), command handlers
    resume.go                DBus logind PrepareForSleep watcher; turns off lightbar on sleep, restores lighting + volatile state on resume
  hid/
    hid.go                   package doc file only
    device.go                Device type, Write, Descriptions, Close
    scan.go                  FindDevice, ListDevices, sysfs discovery, hasAuraReport
    export_test.go           test-only exports: NewTestDevice, NewTestDeviceAnon,
                             UeventToDevPath, DeviceNameFromUevent
contrib/
  systemd/user/
    z13ctl-plus.socket       systemd user socket unit (socket activation, %t/z13ctl-plus/z13ctl-plus.sock)
    z13ctl-plus.service      systemd user service unit (Type=notify, Restart=on-failure)
  systemd/system/
    z13ctl-plus-perms.service system oneshot unit: chgrp/chmod on battery + firmware-attributes sysfs at boot
  udev/
    99-z13ctl-plus.rules     persistent HID, input, and sysfs access rules
```

## Key architectural decisions

- `aura.Writer` interface (not `*hid.Device`) — decouples aura from hid, enables
  mock-based testing without hardware.
- `hid.Device` holds `[]hidrawNode`; writes go to all nodes simultaneously.
- Zone 0 = keyboard (`0b05:1a30`), Zone 1 = lightbar (`0b05:18c6`). Both are always
  addressed; each physical device silently ignores the other zone's packets.
- `profile.go` and `batterylimit.go` use path discovery via `/sys/class/*/` rather
  than hardcoded paths, so udev-chmoded device inodes are used (not the ACPI alias).
- `setup.go` uses a two-part permission strategy: udev rules for boot persistence
  (real ADD events) + direct `chgrp`/`chmod` in Go for immediate effect (systemd 259+
  does not execute `RUN{program}` on synthetic `udevadm trigger` events).
- `--dry-run` is a global persistent flag; each command checks `dryRunFlag` and
  calls the appropriate `cli.DryRun*` function.
- `--no-button` is a global persistent flag; only affects the daemon subcommand.
  When set, the button watcher goroutine is not started and EVIOCGRAB is not issued,
  allowing other tools to exclusively grab the Armoury Crate button device.
- **Daemon socket fallback**: CLI commands try the Unix socket first (1 s timeout);
  fall back to direct HID/sysfs if the daemon is not running. Detection is implicit:
  connection refused → fall back. Only the Plus socket is checked; an upstream
  daemon does not suppress direct fallback.
- **Daemon systemd integration**: `z13ctl-plus.socket` uses socket activation (`LISTEN_FDS`).
  `z13ctl-plus.service` uses `Type=notify` (sd_notify READY=1 after HID open + state restore),
  `WantedBy=graphical-session.target` (works in both desktop and Steam Gaming Mode).
  Logging goes to journald via stdout/stderr.
- **State persistence**: `$XDG_STATE_HOME/z13ctl-plus/state.json` (atomic write via temp +
  rename). `State.Profile` is always the physical profile; fan, TDP, and undervolt
  overrides have independent persisted active flags and are restored independently.
- **Explicit v1 migration**: `MigrateLegacyState()` reads and validates the legacy
  `$XDG_STATE_HOME/z13ctl/state.json`, then atomically installs a copy only if
  Plus state does not exist. It does not remove the legacy file or manage units.
- **Named presets and power policy**: Presets independently capture profile,
  fan/TDP/undervolt, CPU min/EPP/boost, panel overdrive, and refresh rate. The
  daemon polls external `power_supply` devices and applies assigned AC/battery
  presets only at startup or a real source transition; manual overrides last
  until the next transition. Refresh intent is handled by the shared display
  coordinator rather than issuing independent KScreen writes.
- **Shared display coordinator**: Tablet-kit publishes posture and presets publish
  refresh intent. Desktop/laptop use 150% scale and force normal rotation; tablet
  uses 175% and preserves Plasma's current rotation unless an explicit rotation
  intent exists. The daemon coalesces requested internal-panel changes, never
  touches external outputs, waits 10 seconds after desktop/dock posture, and skips
  writes while eDP-1 is disabled.
- **Button watcher**: Finds "Asus WMI hotkeys" input device by sysfs name
  (`/sys/class/input/*/device/name`), opens it, grabs it exclusively with EVIOCGRAB,
  and listens for KEY_PROG3 (code 202) key-down events. KEY_PROG3 is the Armoury Crate
  button keycode on the 2025 ROG Flow Z13 (differs from older ASUS models that use
  KEY_PROG1). Forwards press events to subscribed GUI connections via long-lived
  socket connections. Disabled with `--no-button` to allow other tools exclusive
  access to the button device.
- **Daemon socket protocol**: Newline-delimited JSON over Unix socket. All responses
  are `{"ok":bool,...}`. CLI `--get` commands read sysfs directly (always ground truth).
  GUI/Decky callers use the daemon socket for all operations including GET. Protocol:
  | Command | Request | Response field |
  |---|---|---|
  | apply | `{"cmd":"apply","mode":"cycle","color":"FF0000","brightness":3,"device":"lightbar"}` | `ok` |
  | off | `{"cmd":"off","device":""}` | `ok` |
  | brightness | `{"cmd":"brightness","brightness":2,"device":""}` | `ok` |
  | profile set | `{"cmd":"profile","set":"performance"}` | `ok` |
  | profile get | `{"cmd":"profile-get"}` | `ok`, `value` |
  | battery set | `{"cmd":"batterylimit","set":"80"}` | `ok` |
  | battery get | `{"cmd":"batterylimit-get"}` | `ok`, `value` |
  | boot sound set | `{"cmd":"bootsound","set":"1"}` | `ok` |
  | boot sound get | `{"cmd":"bootsound-get"}` | `ok`, `value` |
  | panel overdrive set | `{"cmd":"paneloverdrive","set":"1"}` | `ok` |
  | panel overdrive get | `{"cmd":"paneloverdrive-get"}` | `ok`, `value` |
  | fan curve get | `{"cmd":"fancurve-get"}` | `ok`, `value` (JSON) |
  | fan curve set | `{"cmd":"fancurve","set":"48:2,..."}` | `ok` |
  | fan curve reset | `{"cmd":"fancurve-reset"}` | `ok` |
  | tdp get | `{"cmd":"tdp-get"}` | `ok`, `value` (JSON) |
  | tdp set | `{"cmd":"tdp","set":"60","pl1":"55","pl2":"65","pl3":"70","force":true}` | `ok` |
  | tdp reset | `{"cmd":"tdp-reset"}` | `ok` |
  | undervolt set | `{"cmd":"undervolt","set":"-20"}` | `ok` |
  | undervolt get | `{"cmd":"undervolt-get"}` | `ok`, `value` (JSON: `cpu_co`, `active`, `profile`) |
  | undervolt reset | `{"cmd":"undervolt-reset"}` | `ok` |
  | full state | `{"cmd":"get-state"}` | `ok`, `state` (cached + sysfs + live temp/RPM + undervolt_available) |
  | preset save/apply/delete | `{"cmd":"preset-save","name":"Gaming"}` (or `preset-apply`, `preset-delete`) | `ok` |
  | power policy | `{"cmd":"power-policy-set","enabled":true,"ac_preset":"AC","battery_preset":"Battery"}` | `ok` |
  | display posture | `{"cmd":"display-posture","posture":"tablet"}` | `ok` |
  | display rotation | `{"cmd":"display-rotation","rotation":"left"}` | `ok` |
  | tablet settings | `{"cmd":"tablet-settings-set","tablet_settings":{"disable_touchscreen_in_desktop":true,"two_finger_hold_context_menu":false,"touchpad_enabled":false,"scroll_sensitivity":3,"scroll_speed":2}}` | `ok` |
  | tablet status | `{"cmd":"tablet-integration-report","tablet_integration":{"posture":"tablet",...}}` | `ok` |
  | subscribe | `{"cmd":"subscribe","events":["gui-toggle","state-changed"]}` | `ok`, then streams events |
- **IPC library**: Hand-rolled JSON. gRPC/drpc require protobuf code generation;
  `net/rpc` lacks streaming; Twirp is HTTP-only. Decky plugin Python backend connects
  via `asyncio.open_unix_connection()` + `json` — zero extra deps.
- **Shared sysfs helpers**: `internal/cli/sysfs.go` contains `FindProfilePath()`,
  `FindBatteryThresholdPath()`, `FindBootSoundPath()`, and `FindPanelOverdrivePath()`,
  used by both `cmd/` and `internal/daemon/server.go` to avoid duplication. Daemon
  GET handlers read sysfs directly (not cached state) for accuracy when another
  process has modified the setting.
- **Optional tablet companion**: `TabletSettings` persist in daemon state and
  project mode `0600` to `$XDG_RUNTIME_DIR/z13ctl-plus/tablet-settings.json`.
  `TabletIntegration` is the latest in-memory heartbeat only. Both updates emit
  `state-changed`; the daemon never persists a stale heartbeat. The opt-in
  two-finger context-menu gesture emits the context-menu key after a quick
  stationary two-finger hold (~650 ms) and is canceled by movement.
- **asus-armoury firmware-attributes**: The `asus_armoury` kernel module (mainline
  since Linux 6.19) exposes BIOS attributes via the `fw_attributes_class` interface
  at `/sys/class/firmware-attributes/asus-armoury/attributes/`. On the 2025 Z13,
  `boot_sound` (POST beep toggle, 0/1) and `panel_overdrive` (panel refresh overdrive,
  0/1) are available and writable. These are BIOS firmware settings managed by the
  kernel — no daemon state persistence needed. The `current_value` files are
  `0644 root:root` by default; `setup.go` handles permissions via udev rules
  (`SUBSYSTEM=="firmware-attributes", KERNEL=="asus-armoury"`) and direct `applySysfsPerms()`.
  Other attributes exist (`charge_mode` is read-only charger type detection; PPT
  controls exist but are empty on Z13 due to missing DMI calibration data).
- **Fan curves**: Two hwmon devices under asus-nb-wmi: `asus` (RPM readings +
  `pwm_enable`) and `asus_custom_fan_curve` (8-point curves + `pwm_enable`).
  hwmon numbers are unstable across reboots — discovery by `name` sysfs attribute
  via `FindFanHwmonPath()`. Both hwmon devices expose `pwm_enable` and both must
  be written for the kernel to honor the mode change. `pwm_enable` values:
  0=full-speed, 1=custom, 2=auto/firmware. Both fans cool the same APU (no
  discrete GPU), so the same curve is always applied to both fans simultaneously.
- **TDP (PPT power limits)**: Direct platform device attributes at
  `/sys/devices/platform/asus-nb-wmi/ppt_*` (NOT the firmware-attributes interface,
  which has empty calibration data). Five attributes: `ppt_pl1_spl` (Sustained),
  `ppt_pl2_sppt` (Short Boost), `ppt_fppt` (Fast Boost), `ppt_apu_sppt`,
  `ppt_platform_sppt`. Safety limits: 5–75W (safe), up to 93W with `--force`
  (G-Helper absolute max for 2025 Z13 GZ302E). Sustained PL1 above 75W applies a
  temporary 50%+ fan safety curve before PPT writes; the desired curve or auto
  mode returns when PL1 becomes safe. APU sPPT and Platform sPPT follow PL2.
- **Independent tuning overrides**: The physical profile is always quiet,
  balanced, or performance. Fan curve, TDP, and undervolt overrides are activated
  and reset independently. Profile changes preserve and reapply active overrides.
- **Undervolt (Curve Optimizer)**: CPU voltage reduction via AMD Curve Optimizer,
  using direct SMU communication through the `ryzen_smu` kernel module's sysfs
  interface at `/sys/kernel/ryzen_smu_drv/`. Uses only the MP1 0x4C command for
  CPU CO (iGPU CO was removed — Strix Halo does not support it). Optional
  dependency — gracefully disabled when the module is not installed.
  `SMUProbeUndervolt()` sends a safe no-op probe at daemon startup to detect
  whether the installed `ryzen_smu` fork actually supports CO commands on this
  platform (the amkillam fork is required for Strix Halo; the leogx9r fork does
  not work). CO values are volatile (reset on reboot/sleep); the daemon
  reapplies them on startup and resume while the override is active.
  Safety limit: CPU 0 to -40. `--get` returns saved values from daemon state
  plus the current profile (no sysfs readback exists for CO). `UndervoltState`
  includes an `Active bool` field indicating whether the offset is currently
  applied. The `get-state` response includes
  `undervolt_available` (using `SMUProbeUndervolt()`, not just `SMUAvailable()`)
  so GUIs can hide controls when ryzen_smu is not installed or the wrong fork
  is present.
- **NPU telemetry (amdxdna DRM ioctl)**: `internal/cli/npu.go` reads power (W)
  and per-column utilization (%) via `DRM_AMDXDNA_QUERY_SENSORS`, plus the
  current MP-NPU clock (npuclk) via `DRM_AMDXDNA_QUERY_CLOCK_METADATA`. These
  are hardware-backed sensors (not derived submission-counter deltas), exposed
  by the `amdxdna` driver (mainline since Linux 6.19; `amdxdna-dkms` 7.0+ on
  older kernels). Unlike the amdgpu/iGPU readers which use sysfs, the NPU
  readers issue ioctls on `/dev/accel/accelN` (discovered via
  `/sys/class/accel/accel*/device/vendor == 0x1022`). Multiple column sensors
  are averaged to one busy %. No `xrt-smi` or `xdna-top` dependency — pure
  Go, one new file. Added to `api.TelemetryState` and surfaced on the
  `status` command and in the daemon's `get-state` response. Column
  utilization and power sensors are gated by driver version; older `amdxdna`
  stacks may return power only.
  **Note on `NPUClockMHz`**: the value is *live* (driver source: `mp_npu_clock.
  freq_mhz = ndev->npuclk_freq`, updated on every DPM switch via
  `aie2_smu.c:114`). In practice on Strix Halo it appears static at 1267 MHz
  because the driver pins DPM at max whenever any process holds the device
  open — but it is not metadata. Older `amdxdna` versions (pre-7.0) may not
  update `npuclk_freq` and would report a fixed value.
- **NPU power modes — NOT EXPOSED**: the amdxdna driver's DPM power_mode
  interface (DEFAULT/HIGH/TURBO via `DRM_AMDXDNA_SET_STATE` with
  `DRM_AMDXDNA_SET_POWER_MODE`) was implemented and then removed after
  empirical A/B testing proved it has no measurable effect. Findings:
  - The driver marks SET_STATE as `DRM_ROOT_ONLY` (`amdxdna_pci_drv.c`),
    so setting modes required `CAP_SYS_ADMIN`.
  - Only 3 of 5 uAPI modes work: `aie2_pm_set_mode` (`aie2_pm.c:82-126`)
    implements DEFAULT (which equals max DPM on NPU5 because
    `dft_dpm_level == max_dpm_level`), HIGH (max DPM, clk_gating on), and
    TURBO (max DPM, clk_gating off). LOW and MEDIUM return `-EOPNOTSUPP`.
  - **Sustained generation A/B (Qwen3-0.6B, 12 samples per mode)**:
    default=2.14W, high=2.16W, turbo=2.16W — within 20mW of each other.
  - **Bursty A/B (40-token generations every 6s, 5s idle gaps)**: all three
    modes show identical decay curves, troughing at 0.42-0.43W between
    bursts — 10mW spread, indistinguishable from sensor noise.
  - **Root cause**: the driver pins DPM at max whenever any process holds
    `/dev/accel/accelN` open. The 0.42W idle floor (vs 0.00W when the
    device is fully released) is device-open leakage at the pinned max DPM
    level; `power_mode` cannot reduce it. The only knob that moves NPU
    power is whether the device file is open, which is application
    behavior, not a mode setting.
  - **Conclusion**: DEFAULT/HIGH/TURBO are functionally equivalent in
    measured power across sustained AND bursty workloads. Exposing the
    interface would mislead users into thinking they can save power;
    honest UX is to omit it. The amdxdna driver may improve in future
    versions; if so, re-adding the CLI surface is ~1 hour of work.
- **Sleep/resume hook**: `internal/daemon/resume.go` watches for DBus
  `org.freedesktop.login1.Manager.PrepareForSleep` signals. On sleep
  (`PrepareForSleep(true)`), turns off the lightbar via `aura.TurnOff()` (the
  keyboard turns off automatically in hardware, but the lightbar does not).
  On resume (`PrepareForSleep(false)`), restores lighting (regardless of profile)
  and reapplies volatile state: fan curves, TDP, and undervolt (when custom
  profile is active). Uses `github.com/godbus/dbus/v5`.
- **Keyboard hotplug watcher**: `internal/daemon/hotplug.go` handles the
  detachable keyboard. The keyboard (`0b05:1a30`) is its own HID device that
  loses power when detached; on reattach the firmware does not restore the
  previous RGB effect. The daemon opens the HID device once at startup and holds
  `d.dev`, so after a detach/reattach cycle the keyboard appears as a *new* hidraw
  node that the stale `d.dev` never references. `watchHotplug()` polls
  `hid.HasDevice("keyboard")` (sysfs-only presence check, no device open) every 2s;
  on an absent → present transition it calls `reopenAndRestore()`, which re-runs
  `hid.FindDevice("")` under `d.mu`, swaps in the new device (closing the old one),
  and re-applies saved lighting via the existing `applyLightingState()` (honoring
  per-device overrides). If the reopen fails — e.g. udev has not yet chmod'd the new
  hidraw node — the watcher does not latch the present state and retries on the next
  tick. No action is taken on detach (the keyboard powers off in hardware). Run()'s
  device-close defer closes whatever `d.dev` currently is, since hotplug may have
  replaced it.

## Go conventions established in this project

- Per-file descriptive comments go **below** the `package` line, not before it.
  Before-package comments must be `// Package X ...` format (revive `package-comments`).
- Every package has exactly one file with the package-doc comment (`// Package X ...`).
  For `cmd/`, that comment is in `root.go`. For `aura`, `hid`, and `cli`, there are
  dedicated `aura.go`, `hid.go`, and `cli.go` doc files.
- Octal literals must use `0o644` form (gocritic `octalLiteral`).
- Do not use `filepath.Join` with arguments that contain path separators
  (gocritic `filepathJoin`); use string concatenation instead.
- `t.Parallel()` must NOT be used in tests that redirect `os.Stdout` (race on global).
  This applies to all tests in `internal/cli/dryrun_test.go`.

## Linting

golangci-lint **v2** format. Config at `.golangci.yml`.
- Linter settings nest under `linters.settings:` (not top-level `linters-settings:`).
- `issues.exclude` and `issues.exclude-rules` are not valid in v2.
- Enabled linters: errcheck, govet (with shadow), staticcheck, misspell, revive
  (exported + unused-parameter), gocritic (diagnostic + style tags).
- Verify config with: `golangci-lint config verify`

## Testing

- Hardware not required. `internal/aura` uses `mockWriter`; `internal/hid` uses
  `os.Pipe()` backed devices via `NewTestDevice`.
- Current coverage: ~79% aura, ~97% cli, ~27% hid (hardware-bound scan functions
  are untestable without a device).
- aura error branches (write failures) are not covered because mockWriter never errors.

## Build / release

```sh
make build              # go build with version from git tags via ldflags
make test               # go test ./...
make cover              # test + coverage report
make lint               # golangci-lint run ./...
make mod-tidy           # go mod tidy for all modules (main + api/)
sudo make install            # install pre-built binary to /usr/local/bin
make install-service         # install + enable systemd user units (socket + service)
make uninstall-service       # stop, disable, and remove systemd user units
sudo make install-perms-service    # install system oneshot service for sysfs permissions on boot
sudo make uninstall-perms-service  # remove system permissions service
make snapshot           # goreleaser release --snapshot --clean  (no publish)
make release            # goreleaser release --clean             (requires pushed v* tag)
make clean              # remove z13ctl-plus binary, dist/, coverage artifacts
```

Version is injected at link time: `-X github.com/aic0d3r/z13ctl-plus/cmd.Version={{.Version}}`.
Default value in source is `"2.0.0"` (used only in local builds without ldflags).

## goreleaser

Config: `.goreleaser.yml`. GitHub Actions workflow: `.github/workflows/release.yml`.
- Builds for `linux/amd64` only (hidraw is Linux-specific; Z13 is x86_64).
- `before.hooks`: `go mod tidy` only.
- Release archives, packages, and the binary inside are named `z13ctl-plus`.
- Packages intentionally do not declare upstream provide/conflict/replace
  relationships and do not enable the Plus user daemon during installation.
- Archives include `README.md`, `LICENSE`, `contrib/systemd/**/*`, and
  `contrib/udev/*`.
- `prerelease: auto` — tags with a pre-release suffix (e.g. `v2.0.0-beta`) are marked
  as pre-release on GitHub automatically.
- AUR publishing is manual (`workflow_dispatch`) and requires `AUR_KEY`.
- To release: `git tag -a vX.Y.Z -F release-notes.md && git push origin vX.Y.Z`.

## Documentation

- `README.md` — user-facing (installation, commands, colors, contributing).
- `PROTOCOL.md` — technical HID protocol reference for developers.
- Per-command docs are not generated separately; `README.md` and `--help` are the
  authoritative references.

## Current status and next steps

### Phase 1 (daemon) — COMPLETE

The daemon is fully implemented and passing `make build && make test && make lint`.

**Key daemon details:**
- Socket path: `$XDG_RUNTIME_DIR/z13ctl-plus/z13ctl-plus.sock`
- State file: `$XDG_STATE_HOME/z13ctl-plus/state.json`
- Protocol: one newline-terminated JSON request → one JSON response; long-lived
  connections for `{"cmd":"subscribe","events":["gui-toggle"]}` (GUI use)
- Button watcher uses `github.com/holoplot/go-evdev`; systemd integration uses
  `github.com/coreos/go-systemd/v22` (activation + daemon packages)
- `ATTR{name}=="asus-wmi"` was removed from the platform-profile udev rule because
  platform-profile class devices do not have a `name` sysfs attribute file. The rule
  now matches `SUBSYSTEM=="platform-profile"` alone.
- `z13ctl-plus-perms.service` (system-level oneshot) runs `chmod g+w` on
  `BAT*/charge_control_end_threshold` and firmware-attributes `current_value` files
  after `sysinit.target`. This is necessary because these attributes may be created
  after observable udev events — so no udev `RUN+=` hook can catch them reliably.

**To activate on this machine (must be done once after each `sudo z13ctl-plus setup`):**
```sh
sudo z13ctl-plus setup           # rewrites rules file; applies sysfs perms immediately
sudo make install-perms-service  # installs battery sysfs permissions service
make install-service             # installs daemon socket + service units
```

### Phase 2 (GUI) — COMPLETE

The `z13gui-plus` overlay and the `api/` submodule are complete and released.
- Root module is `github.com/aic0d3r/z13ctl-plus`; only the API submodule retains
  `github.com/dahui/z13ctl/api`, selected by Plus clients through a replacement.
- `z13gui-plus` (repo `aic0d3r/z13gui-plus`) is a GTK4 Wayland/Gamescope overlay
  drawer talking to the daemon through `api/`.
- Current release: **v2.1.0** (main) and **api/v1.5.0** (submodule).

**Fork API release boundary:** Root `vX.Y.Z` tags in `aic0d3r/z13ctl-plus` do not
publish the canonical `github.com/dahui/z13ctl/api` module. Plus clients select
matching fork API source via a replacement; the API submodule is tagged separately
as `api/vX.Y.Z` (e.g. `api/v1.5.0`). Standalone builds of `z13gui-plus` resolve
`github.com/aic0d3r/z13ctl-plus/api` from the published `api/v*` tag.

### Phase 3 (tablet integration) — COMPLETE

`z13ctl-plus` is the display and tablet-settings authority in a three-repo stack:
- **z13-tablet-kit** (`aic0d3r/z13-tablet-kit`): posture detection (dock/folio/tablet),
  touchscreen toggle, virtual `SW_TABLET_MODE`, touch scroll + two-finger-hold context
  menu, OSK posture gating.
- **z13ctl-plus** (this repo): owns internal-panel scale/rotation/refresh and persists
  tablet settings.
- **z13gui-plus** (`aic0d3r/z13gui-plus`): overlay drawer exposing live posture/health
  and the tablet toggles.

Tablet-specific daemon behaviour (all complete):
- `z13ctl-plus tablet settings` (persist) + `z13ctl-plus tablet report` (ephemeral
  integration state); settings projected to
  `$XDG_RUNTIME_DIR/z13ctl-plus/tablet-settings.json` for the kit to read live.
- Display posture: tablet scale `1.75`; desktop/laptop `1.5`. Plasma retains tablet
  rotation unless an explicit rotation intent is sent. The kit never calls
  `kscreen-doctor` directly.
- Tablet settings: desktop-touch policy, `two_finger_hold_context_menu`, folio
  touchpad opt-in, scroll sensitivity/speed.
