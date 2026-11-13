#!/usr/bin/env bash
set -euo pipefail

repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
root=/home/pi/services/trick-evolution
units=/home/pi/.config/systemd/user
mkdir -p "$root"/{bin,releases,logs,state} "$units"
install -m 0755 "$repo/scripts/deploy-dev-pi.sh" "$root/bin/deploy-dev-pi.sh"
for name in trick-evolution.service trick-evolution-deploy.service trick-evolution-deploy.timer trick-evolution-tunnel.service; do
  install -m 0644 "$repo/deploy/systemd/$name" "$units/$name"
done
systemctl --user daemon-reload
echo 'Units installed. Bootstrap a committed release before enabling the service/timer.'
