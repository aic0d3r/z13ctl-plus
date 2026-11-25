#!/bin/sh
# Upgrades also run the old package's pre-remove (on RPM after the new
# post-install); only a real removal may disable the units.
case "$1" in
  upgrade|failed-upgrade|1) exit 0 ;;
esac
systemctl --global disable z13ctl-plus.socket z13ctl-plus.service || true
systemctl disable --now z13ctl-plus-perms.service || true
