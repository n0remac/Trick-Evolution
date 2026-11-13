#!/usr/bin/env bash
# Runs as root on the Caddy droplet. The caller provides a staged site file.
set -Eeuo pipefail
candidate=${1:?staged Caddy site required}
site=/etc/caddy/sites/trick.caddy
backup=''
if [[ -e $site ]]; then
  backup="/etc/caddy/sites/trick.caddy.backup.$(date +%Y%m%d%H%M%S)"
  cp -p "$site" "$backup"
fi
restore() {
  rc=$?
  trap - ERR
  if [[ -n $backup ]]; then cp -p "$backup" "$site"; else rm -f "$site"; fi
  caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null || true
  systemctl reload caddy || true
  echo "Caddy update failed (exit $rc); restored previous route" >&2
  exit "$rc"
}
trap restore ERR
install -m 0644 "$candidate" "$site"
caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile
systemctl reload caddy
echo "Caddy route installed at $site; prior copy: ${backup:-none}"
