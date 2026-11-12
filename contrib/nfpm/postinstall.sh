#!/bin/sh
udevadm control --reload-rules
udevadm trigger
systemctl enable --now z13ctl-plus-perms.service || true
# Reload units and restart only an already-running daemon so the new binary is used.
for u in $(loginctl list-users --no-legend 2>/dev/null | awk '{print $1}'); do
  systemctl --user --machine="$u@.host" daemon-reload 2>/dev/null || true
  systemctl --user --machine="$u@.host" try-restart z13ctl-plus.service 2>/dev/null || true
done
echo "z13ctl-plus user daemon installed disabled; enable only after stopping other Z13 daemons:"
echo "  systemctl --user enable --now z13ctl-plus.socket z13ctl-plus.service"
