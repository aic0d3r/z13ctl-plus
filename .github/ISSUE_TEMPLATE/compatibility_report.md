---
name: Compatibility Report
about: Report how z13ctl-plus behaves on a machine other than the ROG Flow Z13
labels: compatibility
---

**Machine**
- Vendor and model (`cat /sys/class/dmi/id/sys_vendor /sys/class/dmi/id/product_name`):
- CPU (for example Ryzen AI Max+ 395):
- Laptop, mini PC, or desktop:
- Battery present:

**Software**
- Linux distribution and kernel (`uname -r`):
- z13ctl-plus version (`z13ctl-plus --version`):
- Client in use (CLI, Z13GUI+, other):

**What the daemon reports**

Paste the `capabilities` object (needs `socat` and `jq`):

`echo '{"cmd":"get-state"}' | socat - UNIX-CONNECT:$XDG_RUNTIME_DIR/z13ctl-plus/z13ctl-plus.sock | jq '.state.capabilities'`

```
```

**What works**

Mark each as working, "not supported" (clear error), or broken:

- `z13ctl-plus status` (temperature, fans, profile, TDP, battery):
- Profile switching:
- Fan curve:
- TDP and undervolt:
- CPU boost, EPP, minimum frequency:
- Battery charge limit:
- RGB lighting:
- Refresh rate, panel overdrive, boot sound:
- Presets and power automation:

**Anything unexpected**

Commands that failed with a raw error instead of "not supported on this machine",
or startup problems (`journalctl --user -u z13ctl-plus.service`).
