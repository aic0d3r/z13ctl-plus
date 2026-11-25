#!/bin/sh
udevadm control --reload-rules || true
# MODE/GROUP rules apply on synthetic events; RUN programs do not (systemd 259+),
# so sysfs permissions come from the perms service below.
udevadm trigger --subsystem-match=hidraw --subsystem-match=input || true
systemctl daemon-reload || true
# restart, not enable --now: on upgrade the oneshot is already active and must
# rerun with the new binary. It keeps the group saved by `z13ctl-plus setup`.
systemctl enable z13ctl-plus-perms.service || true
systemctl restart z13ctl-plus-perms.service || true
# Reload units and restart only an already-running daemon so the new binary is used.
for u in $(loginctl list-users --no-legend 2>/dev/null | awk '{print $1}'); do
  systemctl --user --machine="$u@.host" daemon-reload 2>/dev/null || true
  systemctl --user --machine="$u@.host" try-restart z13ctl-plus.service 2>/dev/null || true
done
echo "z13ctl-plus user daemon installed disabled; enable only after stopping other Z13 daemons:"
echo "  systemctl --user enable --now z13ctl-plus.socket z13ctl-plus.service"
echo "Check the setup with: z13ctl-plus doctor"
