#!/bin/sh
# After installing or upgrading: the user deelook runs as, the settings
# file on the first install, and the service (started, or restarted on the
# new version).
set -e

if ! getent passwd deelook >/dev/null; then
  useradd --system --user-group --home-dir /var/lib/deelook --shell /usr/sbin/nologin deelook
fi

# the settings in use: never over the one there (an upgrade keeps it)
if [ ! -e /etc/deelook/deelook.toml ]; then
  cp /opt/deelook/deelook.toml.default /etc/deelook/deelook.toml
fi
# deelook changes the deemesh-hub address in it when set on the page
chown deelook:deelook /etc/deelook /etc/deelook/deelook.toml /var/lib/deelook

if [ -d /run/systemd/system ]; then
  systemctl daemon-reload
  systemctl enable deelook.service >/dev/null 2>&1 || true
  systemctl restart deelook.service || true
fi
