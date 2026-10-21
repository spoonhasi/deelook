#!/bin/sh
# Before removing (not before an upgrade, after which postinstall restarts
# it): the service stops.
set -e

case "$1" in
  remove | purge)
    if [ -d /run/systemd/system ]; then
      systemctl stop deelook.service || true
      systemctl disable deelook.service >/dev/null 2>&1 || true
    fi
    ;;
esac
