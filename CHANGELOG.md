# Changelog

All notable changes. Newest first. Bodies mirror the GitHub release notes;
release-notes.md (gitignored) is the scratch file for the next entry.

## v2.5.1 (2026-10-16)

- Resume restores the power-policy preset for the current power source when
  it changed during sleep (e.g. unplugged while asleep). Previously the AC
  preset was reapplied for about a second before the battery preset took over.

## v2.5.0 (2026-10-16)

- Commands that need missing hardware (charge limit, boot sound, panel
  overdrive, fan curve, TDP) answer `not supported on this machine: no <what>`
  instead of a raw sysfs error.
- A fresh install starts even when cpufreq is unreadable.
- `get-state` reports `capabilities` (profile, fan curve, TDP, battery, charge
  limit, boot sound, panel overdrive, internal display) so clients can hide
  controls the machine does not have, e.g. on an AMD Strix Halo desktop.
  The object is absent from older daemons; read that as "everything present".
  API module: `api/v1.9.0`.

## v2.4.0 (2026-10-13)

- Refresh-rate requests are refused with "the built-in panel is off" while
  eDP-1 is disabled (docked with only an external display on). Previously
  the request was accepted, did nothing, and the rate was applied on the
  next undock. Presets still store their rate and apply it when the panel
  returns.
- `get-state` reports `panel_off` (eDP-1 read and disabled; not set when the
  read fails) and `external_display` (any other output enabled), from the
  same cached `kscreen-doctor` read as `refresh_rate`.
- API module: `api/v1.8.0`.

## v2.3.0 (2026-10-13)

- New `preset-set` command (`api.SendPresetSet`) stores explicit preset
  values and re-applies the preset when it is active; fan curves are
  stripped. New `preset-rename` command (`api.SendPresetRename`) keeps the
  preset's AC/battery assignment and active marker.
- Applying a preset continues past a failed step and reports every failure.
- Power-source changes come from UPower `OnBattery` signals; a 30 s sysfs
  poll remains as fallback, with a 2 s settle and 10 s retry after failures.
- Turning power automation off restores the settings captured when it was
  enabled (persisted, so it survives a daemon restart), unless a manual
  change happened in between.
- With no TDP override active, `get-state`/`tdp-get` report the profile's
  firmware PPT instead of the stale asus-nb-wmi write cache.
- Presets no longer capture or apply fan mode; fan-curve changes no longer
  clear the active preset.
- API module: `api/v1.7.0`.

## v2.2.2 (2026-10-12)

- The daemon now warns when `powerprofilesctl set` fails during a profile
  switch instead of discarding the error. PPD's amd-pstate driver aborts
  profile activation when its per-policy `policy*/boost` write fails
  (EINVAL) while the global `cpufreq/boost` file is 0 — the file
  z13ctl-plus writes. PPD then sticks on its previous profile, so the
  governor and CPU energy preference stop following profile switches until
  boost is re-enabled. The warning names the remedy; the full interplay
  (including the optional PPD mask) is documented in the installation
  troubleshooting section.
- Docs: PROTOCOL.md moved into `docs/protocol.md` without duplication and
  `tdp --reset` clarified (restores the current physical profile's firmware
  defaults, does not switch to balanced); stale AUR lag note removed,
  api module version corrected, and missing `KeepDisplayScale` added to the
  `docs/api.md` example.

## v2.2.1 (2026-10-12)

- Display scale chosen in System Settings is no longer overwritten. The daemon
  used to re-apply 150% (desktop/laptop) or 175% (tablet) on every 30-second
  posture re-publish from z13-tablet-kit. Scale now changes only when the
  posture changes, and each posture restores the scale last used in it
  (defaults: 150% desktop/laptop, 175% tablet). The first posture after the
  daemon starts keeps KDE's scale.
  [z13-tablet-kit#1](https://github.com/aic0d3r/z13-tablet-kit/issues/1)
- New tablet setting `keep_display_scale` (`tablet settings set
  --keep-display-scale`, Z13GUI+ "Keep my display scale"): when on, posture
  changes never touch the screen scale. `tablet settings set` now requires
  this sixth flag.
- api v1.6.0: `TabletSettings.KeepDisplayScale`.

## v2.2.0 (2026-10-11)

- Lower the high-TDP fan safety floor from 80% to 50% PWM (127), matching
  upstream `dahui/z13ctl#17`: the safety curve now ramps 127 → 255 between
  30 °C and 80 °C instead of holding 204+. Sustained PL1 > 75 W is quieter
  while the APU is cool; full speed at 80 °C, threshold, fans-before-PPT
  ordering, and the "user curve stands if it meets the floor" rule
  (introduced in v2.1.1) are unchanged. [#1](https://github.com/aic0d3r/z13ctl-plus/pull/1)

## v2.1.1 (2026-10-11)
- `get-state` could abort the daemon with a concurrent map read/write while a
  per-device lighting command ran (GUI polling + lightbar change).
- Data race on the HID device handle between the sleep/lid restore paths and
  the detachable-keyboard hotplug reopen.
- Concurrent state saves could land out of order and silently revert a
  settings change (lost update).

- All-device `brightness` now updates per-device overrides too; the old value
  no longer resurrects after a daemon restart.
- `off --device X` followed by `brightness --device X` no longer saves an
  unrestorable lighting entry (which also caused a 2-second hotplug retry loop).
- High-TDP fan safety is now a floor, not a replacement: a user curve whose
  PWM values all meet the 80% floor is kept at sustained TDP above 75 W.
- Daemonless `tdp --set 60` after a forced high-TDP set lifts the safety fan
  curve back to firmware auto (it previously stayed active forever).
- Refresh-rate changes apply when the tablet kit is absent (previously
  silently dropped while returning ok).
- `--dry-run tdp` output now matches what the code actually does.
- `z13ctl-plus status` no longer always reports 0% CPU utilisation.
- `SetProfile` returns an error instead of silently doing nothing when no
  platform-profile device exposes a profile file.

- Socket deadlines on the CLI client, the subscribe handshake, and server
  connections: one blocked hardware write can no longer hang callers forever.
- A subscriber that stops reading is dropped instead of freezing event
  delivery for every client.
- TDP validation is shared between CLI and daemon (`cli.ValidateTDP`), and a
  failed TDP apply rolls back to the previous known power state.
- `setup` validates the group, counts and reports permission failures instead
  of printing success, and warns when the group is `users` or `wheel`.
- The boot permissions service tolerates missing sysfs files (`|| true` per
  line), so PPT/cpufreq/ryzen_smu permissions are applied on kernels without
  asus-armoury (pre <6.19) instead of the unit failing mid-way.

- New GitHub Actions CI (build, vet, test -race, api module, golangci-lint).
- New protocol drift test (every `api.Send*` decoded with
  `DisallowUnknownFields`), dispatch validation tests, and fan-floor tests.

- Re-run `sudo z13ctl-plus setup` once to install the fixed permissions
  service unit, then `systemctl --user restart z13ctl-plus.socket
  z13ctl-plus.service`.
- The `api` submodule is tagged `api/v1.5.1` (client socket deadlines);
  GUI clients should update their replacement.

---

**z13ctl-plus** is an independently named fork of
[dahui/z13ctl](https://github.com/dahui/z13ctl) that preserves compatible
API and protocol interfaces where practical.

## Changelog
* a13c0069f2d9005579f094b09d19bd9bcfa06f42 massive bugfix drop
* 65fc10713a5a4b4d7787e73f2baaecad59761323 docs: add wget for release tarball in install instructions
* c91fe3a92ba0b383f31d9a4c08f412f5aeff9cdc docs: note AUR lag and recommend release tarball
* 0488254f2e846a55cf1308cb8901458fc350956b docs: update AGENTS.md for completed tablet integration (v2.1.0)

## v2.1.0 (2026-08-05)


---

**z13ctl-plus** is an independently named fork of
[dahui/z13ctl](https://github.com/dahui/z13ctl) that preserves compatible
API and protocol interfaces where practical.

## Changelog
* c040c0311e40e584d9717a340a593fef588e9bdb docs: link tablet stack repos and rename CLAUDE.md to AGENTS.md
* c92aa8658343be505c12d674341d33587a865a1b feat: two-finger-hold context menu and 175% tablet scale
* 3d6f0997def6bb18aeb3763b602d9667d098954d feat: expose tablet integration state
* 9b2f399f8e46ef6a7c551cc1f35ca03a67d91b56 fix: preserve Plasma tablet rotation

## v2.0.1 (2026-08-03)
Report live keyboard and lightbar presence so clients can handle detached devices without discarding saved lighting intent.

---

**z13ctl-plus** is an independently named fork of
[dahui/z13ctl](https://github.com/dahui/z13ctl) that preserves compatible
API and protocol interfaces where practical.

## Changelog
* e1ffd80bde3f54d2b0b46e36d8a3ff04b1725010 fix: report detached lighting devices

## v2.0.0 (2026-08-02)
This release gives the fork a complete, independent runtime identity while
preserving intentional API and daemon protocol compatibility.

## Breaking changes

- The installed command is now `z13ctl-plus`; no `z13ctl` alias is installed.
- User units are `z13ctl-plus.socket` and `z13ctl-plus.service`.
- The permissions unit is `z13ctl-plus-perms.service` and the udev file is
  `99-z13ctl-plus.rules`.
- Runtime data moves to `$XDG_RUNTIME_DIR/z13ctl-plus` and persisted state moves
  to `$XDG_STATE_HOME/z13ctl-plus`.
- Packages no longer provide, conflict with, or replace upstream `z13ctl` and
  leave the Plus user daemon disabled after installation.

## Migration

Run `z13ctl-plus migrate-state` to validate and copy v1 state into the Plus
namespace. It refuses to overwrite existing Plus state and leaves the legacy
source untouched. The command does not modify services or remove old files.

After selecting the Plus hardware daemon, enable it explicitly:

```sh
systemctl --user enable --now z13ctl-plus.socket z13ctl-plus.service
```

Legacy `z13ctl` paths may belong to upstream or to v1 of this fork. Disable or
remove them only after verifying their provenance.

The compatible fork API source is released separately as `api/v1.4.0` and
targets only the Plus socket.

---

**z13ctl-plus** is an independently named fork of
[dahui/z13ctl](https://github.com/dahui/z13ctl) that preserves compatible
API and protocol interfaces where practical.

## Changelog
* 632254f02ea5da15320355c6385cd0337d1a0b5b feat: adopt independent plus namespace
* d493cf6527fc3cde84a81f8fd2b8391c2d55be95 Rename project from z13ctl-plus to z13ctl

## v1.3.1 (2026-07-28)
- Clarify the plus-only feature set and link the z13gui-plus companion project.

---

**z13ctl-plus** is a compatibility-preserving distribution of
[dahui/z13ctl](https://github.com/dahui/z13ctl). The installed command and
runtime interfaces remain `z13ctl`.

## Changelog
* 07117afe32084f43f800282692cfdce6bf538903 docs: highlight plus-only capabilities
* 293fdff29f78d5273958e50046dace1db07af421 fix: preserve zero-rpm quiet fan control
* 65e09563a28badb41386926eb296103735b4a385 feat: delay quiet profile fan ramp
* c5d5f07fe4ff26687b2b1cf812b0b7a798d53719 docs: link to z13gui-plus companion
* c9184e35a0cad5b98d5af4f5b089599eba9111db fix: support sensorless NPU drivers
* fb9e00d45e5d4e4e2528c13959e71adb503779f0 docs: focus plus comparison on features
* a106a2f2a3a1a36ad26b82d974dead6d51a5dbaf docs: summarize z13ctl-plus improvements
