# Installation

## Prerequisites

- Linux kernel with `hidraw` support (standard on all mainstream distributions)
- 2025 ASUS ROG Flow Z13 (USB IDs `0b05:18c6` and `0b05:1a30`)

!!! warning "InputPlumber conflict (Bazzite, ChimeraOS, Nobara)"
    On gaming distributions that ship [InputPlumber](https://github.com/ShadowBlip/InputPlumber),
    the z13ctl-plus daemon's button watcher will fail to open the Armoury Crate button
    device. InputPlumber ships a built-in profile for the ROG Flow Z13 that grabs
    the device exclusively. See [InputPlumber compatibility](daemon.md#inputplumber-compatibility)
    for the override instructions.

### Optional: ryzen_smu kernel module (for undervolting)

CPU undervolting via AMD Curve Optimizer requires the `ryzen_smu` kernel
module. Without it, all other features work normally — undervolt commands will
return a helpful error explaining how to install the module.

!!! warning "Strix Halo requires the amkillam fork"
    The original `leogx9r/ryzen_smu` does not support Strix Halo (the SoC in
    the 2025 Z13). You must use the
    [amkillam/ryzen_smu](https://github.com/amkillam/ryzen_smu) fork. The
    `z13ctl-plus` daemon detects the wrong fork at startup and reports
    undervolt as unavailable.

| Distribution | Package | Source |
|---|---|---|
| Arch / CachyOS | `ryzen_smu-dkms-git` | AUR ([amkillam fork](https://github.com/amkillam/ryzen_smu)) |
| Other distros | build from source | [github.com/amkillam/ryzen_smu](https://github.com/amkillam/ryzen_smu) |

```sh
# Arch / CachyOS
yay -S ryzen_smu-dkms-git

# Other distros (build from amkillam fork)
git clone https://github.com/amkillam/ryzen_smu.git
cd ryzen_smu
make
sudo make install
```

### NPU telemetry on CachyOS

CachyOS kernels that provide amdxdna natively should use that in-tree module.
It supports NPU power and column-utilization sensors, while older
`amdxdna-dkms` builds may allow inference but return `EOPNOTSUPP` for the
sensor query. In that case z13ctl-plus reports `ACTIVE` without inventing watt or
utilization values.

Check which module is selected and validate the runtime:

```sh
modinfo -n amdxdna
flm validate
```

The optimal setup reports a module below
`kernel/drivers/accel/amdxdna/`. A path below `updates/dkms/` means DKMS is
overriding the kernel module. Before changing it, confirm the kernel package
actually includes a native module:

```sh
pacman -Qo "/usr/lib/modules/$(uname -r)/kernel/drivers/accel/amdxdna/amdxdna.ko.zst"
```

If that file is owned by the installed kernel, test it reversibly:

```sh
sudo dkms remove amdxdna/7.0 -k "$(uname -r)"
sudo depmod -a "$(uname -r)"
reboot
```

After reboot, run `modinfo -n amdxdna` and `flm validate` again. Restore the
DKMS override if the native driver causes a regression:

```sh
sudo dkms install amdxdna/7.0 -k "$(uname -r)"
reboot
```

Do not remove DKMS on an older kernel that lacks a native amdxdna module.
z13ctl-plus supports both configurations: sensor-capable drivers report watts and
utilization, while sensorless drivers report activity only.

---

## Install

z13ctl-plus installs only the `z13ctl-plus` command. It does not install a
`z13ctl` alias. Its packages do not provide, conflict with, or replace the
upstream package, so both projects can be installed at the same time.

=== "Release binary"

    Download the latest `linux_amd64` archive from the
    [Releases](https://github.com/aic0d3r/z13ctl-plus/releases) page, then extract
    and install:

    ```sh
    tar xzf z13ctl-plus_*_linux_amd64.tar.gz
    sudo install -Dm755 z13ctl-plus /usr/local/bin/z13ctl-plus
    ```

    **One-time permissions setup** (requires root):

    ```sh
    sudo z13ctl-plus setup
    ```

    Then log out and back in (or run `newgrp users`) for the group membership
    to take effect in your current session.

    **Install the systemd user service** (socket activation):

    ```sh
    install -Dm644 contrib/systemd/user/z13ctl-plus.socket \
        ~/.config/systemd/user/z13ctl-plus.socket
    install -Dm644 contrib/systemd/user/z13ctl-plus.service \
        ~/.config/systemd/user/z13ctl-plus.service
    systemctl --user daemon-reload
    systemctl --user enable --now z13ctl-plus.socket z13ctl-plus.service
    ```

=== "Arch Linux (AUR)"

    Install the [z13ctl-plus-bin](https://aur.archlinux.org/packages/z13ctl-plus-bin)
    package with your preferred AUR helper:

    ```sh
    yay -S z13ctl-plus-bin
    ```

    The package installs the binary, udev rules, systemd units, and the battery
    permissions service. It leaves the z13ctl-plus user daemon disabled so the
    package can coexist with upstream. After installing, add your user to the
    `users` group if not already a member:

    ```sh
    sudo usermod -aG users $USER
    ```

    Then log out and back in for the group membership to take effect.

    Alternatively, download the `.pkg.tar.zst` package directly from the
    [Releases](https://github.com/aic0d3r/z13ctl-plus/releases) page and install with
    pacman:

    ```sh
    sudo pacman -U z13ctl-plus_*.pkg.tar.zst
    ```

=== "Debian / Ubuntu"

    Download the `.deb` package from the
    [Releases](https://github.com/aic0d3r/z13ctl-plus/releases) page, then install:

    ```sh
    sudo apt install ./z13ctl-plus_*.deb
    ```

    The package installs the binary, udev rules, systemd units, and the battery
    permissions service. It leaves the z13ctl-plus user daemon disabled so the
    package can coexist with upstream. After installing, add your user to the
    `users` group if not already a member:

    ```sh
    sudo usermod -aG users $USER
    ```

    Then log out and back in for the group membership to take effect.

=== "Fedora / RHEL"

    Download the `.rpm` package from the
    [Releases](https://github.com/aic0d3r/z13ctl-plus/releases) page, then install:

    ```sh
    sudo dnf install ./z13ctl-plus_*.rpm
    ```

    The package installs the binary, udev rules, systemd units, and the battery
    permissions service. It leaves the z13ctl-plus user daemon disabled so the
    package can coexist with upstream. After installing, add your user to the
    `users` group if not already a member:

    ```sh
    sudo usermod -aG users $USER
    ```

    Then log out and back in for the group membership to take effect.

=== "From source"

    Requires Go 1.25 or later.

    ```sh
    git clone https://github.com/aic0d3r/z13ctl-plus
    cd z13ctl-plus
    make build
    sudo make install
    sudo z13ctl-plus setup
    ```

    `make build` creates `./z13ctl-plus`, and `make install` installs it as
    `/usr/local/bin/z13ctl-plus`;
    the separate setup command installs its udev rules and permissions service.

    Install the systemd user service:

    ```sh
    make install-service
    ```

---

## Co-installation and daemon choice

Package installation leaves the z13ctl-plus user daemon disabled. Before
enabling it, stop and disable any other Z13 hardware daemon you chose to replace.
Run only one project's hardware daemon at a time.

The z13ctl-plus CLI checks only the Plus socket. If that socket is unavailable,
hardware commands fall back to direct HID/sysfs access. Do not leave the upstream
daemon running while using z13ctl-plus direct-fallback commands; the two processes
can otherwise issue competing hardware writes.

To select the Plus daemon after installation:

```sh
systemctl --user enable --now z13ctl-plus.socket z13ctl-plus.service
```

---

## Migrate from v1 to v2

v1 of this fork used the upstream `z13ctl` command and runtime paths. Those paths
are provenance-ambiguous: they may belong to upstream, the old fork, or a mix of
both. Only disable or remove a legacy path when you know it came from the fork.

Before starting the v2 daemon, switch services and copy state explicitly:

1. Install v2 and run `sudo z13ctl-plus setup`.
2. If you know the active `z13ctl.socket` and `z13ctl.service` came from v1 of
   this fork, stop and disable them:

    ```sh
    systemctl --user disable --now z13ctl.socket z13ctl.service
    ```

3. Preview and perform the state copy:

    ```sh
    z13ctl-plus --dry-run migrate-state
    z13ctl-plus migrate-state
    ```

4. Enable the v2 units:

    ```sh
    systemctl --user daemon-reload
    systemctl --user enable --now z13ctl-plus.socket z13ctl-plus.service
    ```

`migrate-state` validates and copies
`$XDG_STATE_HOME/z13ctl/state.json` to
`$XDG_STATE_HOME/z13ctl-plus/state.json` only when the Plus state file is absent.
With the normal XDG defaults, the source is
`~/.local/state/z13ctl/state.json` and the destination is
`~/.local/state/z13ctl-plus/state.json`. It refuses invalid or missing source
state and refuses to overwrite existing Plus state. It does not start, stop,
enable, or disable services, and it does not remove the source state or any old
artifact.

### Optional legacy cleanup

After v2 state and service behavior are verified, you may remove known v1-fork
artifacts manually. Potential legacy paths are `/usr/local/bin/z13ctl`,
`~/.config/systemd/user/z13ctl.socket`,
`~/.config/systemd/user/z13ctl.service`,
`/etc/systemd/system/z13ctl-perms.service`,
`/etc/udev/rules.d/99-z13ctl.rules`, and
`$XDG_STATE_HOME/z13ctl/`. Do not remove any of them when provenance is unknown
or when you still use upstream. Prefer the old package's uninstall process for
package-owned files.

---

## Verify the installation

```sh
z13ctl-plus list
```

This should print the discovered hidraw devices and confirm Aura support.
If it prints nothing, see [Troubleshooting](#troubleshooting) below.

---

## GUI Frontend

**Highly recommended for most users.** Install
[Z13GUI+](https://github.com/aic0d3r/z13gui-plus) alongside z13ctl-plus — a touch-friendly
GTK4 overlay that communicates with z13ctl-plus through its daemon. It provides
graphical controls for power automation, CPU policy, expanded telemetry, refresh
rate, and tuning. Display posture/rotation and scripting remain CLI/API workflows.
See the
[Z13GUI+ README](https://github.com/aic0d3r/z13gui-plus#readme) for installation
instructions.

!!! note "Requires the daemon"
    Z13GUI+ talks to z13ctl-plus over its Plus daemon socket, so make sure the
    [systemd user service](daemon.md) is enabled after choosing z13ctl-plus as
    the only hardware daemon. Packages install the units but leave them disabled.

---

## Next steps

You're installed and ready to go. Head to the [Quick Start](getting-started.md)
guide to set your first lighting effect, fan curve, and performance profile.

---

## What `setup` does

`sudo z13ctl-plus setup` performs four steps:

1. Writes `/etc/udev/rules.d/99-z13ctl-plus.rules` — grants the `users` group
   `MODE=0660` / `GROUP=users` on the ASUS HID and input device nodes; uses
   `RUN+=chgrp/chmod` to set permissions on the platform-profile attribute,
   hwmon fan curve attributes (`asus_custom_fan_curve` + `asus` pwm_enable),
   and asus-nb-wmi PPT power limit attributes when the drivers load.
2. Reloads udev and applies permissions immediately to all currently present
   files — including `ryzen_smu` sysfs files for undervolting (if the module
   is loaded).
3. Writes `/etc/systemd/system/z13ctl-plus-perms.service` and enables it — a
   `Type=oneshot` service that runs `chgrp` + `chmod g+w` on
   `BAT*/charge_control_end_threshold` and `ryzen_smu_drv` sysfs files at boot.
4. Starts the service immediately so battery limit and undervolt are accessible
   right away.

!!! note "Why a separate oneshot service for battery and ryzen_smu?"
    The `charge_control_end_threshold` sysfs attribute is added by the
    `asus_nb_wmi` kernel driver late in its `probe()` sequence — after all
    observable udev child-device events have already fired. The `ryzen_smu`
    files are kobjects under `/sys/kernel/`, not udev-managed devices. There
    is no udev hook that can reliably target either. The `z13ctl-plus-perms.service`
    unit is a self-contained workaround that runs at `sysinit.target`. It has
    no dependency on the z13ctl-plus binary and can be inspected at any time:

    ```sh
    systemctl cat z13ctl-plus-perms.service
    ```

Use `--dry-run` to preview all changes without root or any side effects:

```sh
z13ctl-plus --dry-run setup
```

---

## Uninstall

Remove the user service first:

```sh
systemctl --user disable --now z13ctl-plus.socket z13ctl-plus.service
rm -f ~/.config/systemd/user/z13ctl-plus.socket \
      ~/.config/systemd/user/z13ctl-plus.service
systemctl --user daemon-reload
```

Then remove the binary and system files:

```sh
sudo rm /usr/local/bin/z13ctl-plus
sudo rm /etc/udev/rules.d/99-z13ctl-plus.rules
sudo udevadm control --reload-rules
sudo systemctl disable --now z13ctl-plus-perms.service
sudo rm /etc/systemd/system/z13ctl-plus-perms.service
sudo systemctl daemon-reload
```

---

## Troubleshooting

**`z13ctl-plus list` prints nothing**

The device was not found. Check that the kernel loaded the `hid` and `hidraw`
modules:

```sh
lsmod | grep hid
ls /dev/hidraw*
```

If `/dev/hidraw*` devices exist but none match the ASUS USB IDs, the device
is present but not recognized. This tool targets the 2025 ROG Flow Z13
specifically (USB IDs `0b05:18c6` and `0b05:1a30`).

**Permission denied**

Run `sudo z13ctl-plus setup` and log out/in to apply the group membership.
Verify with:

```sh
ls -la /dev/hidraw*
groups
```

**Profile switches stop changing CPU behavior after turning CPU boost off**

The daemon writes the *global* `cpufreq/boost` file, while
[power-profiles-daemon](https://gitlab.freedesktop.org/upower/power-profiles-daemon)
(PPD) restores profiles by writing *per-policy* `policy*/boost` files. The
kernel rejects those per-policy writes while global boost is off
(`EINVAL`), so PPD aborts and sticks on its previous profile — the governor
stays `performance`, CPU energy preference locks to `performance`, and
profile switches appear dead. The daemon logs
`powerprofilesctl set failed; PPD may stay stuck until CPU boost is re-enabled`
when this happens.

Workarounds, in order of preference:

- Re-enable CPU boost, then switch profiles once — this unsticks PPD.
- If you don't rely on your desktop's power-profile applet, mask PPD and let
  z13ctl-plus own power management alone. Without PPD the kernel keeps its
  default governor, so CPU energy preference stays fully selectable:
  ```sh
  sudo systemctl mask power-profiles-daemon
  sudo reboot   # or stop every PPD process
  ```

z13ctl-plus deliberately does **not** mask or configure PPD itself: the
installer only manages its own files, and masking would silently break the
KDE/GNOME power-profile UIs for anyone who wants both.

**Daemon logs "button watcher stopped; retrying … permission denied"**

Another process may be holding an exclusive grab on the Armoury Crate button
device. On gaming distributions, [InputPlumber](https://github.com/ShadowBlip/InputPlumber)
is a common cause. See
[InputPlumber compatibility](daemon.md#inputplumber-compatibility) for the fix.

**Daemon not starting**

```sh
systemctl --user status z13ctl-plus.socket z13ctl-plus.service
journalctl --user -u z13ctl-plus.service -n 50
```
