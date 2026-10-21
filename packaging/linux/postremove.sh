#!/bin/sh
# After removing: systemd forgets the service. Purge also removes the
# settings; the data folder (/var/lib/deelook: recorded sessions) and the
# user deelook are kept either way, to be deleted by hand.
set -e

if [ -d /run/systemd/system ]; then
  systemctl daemon-reload || true
fi
if [ "$1" = purge ]; then
  rm -rf /etc/deelook
fi
