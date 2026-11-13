#!/usr/bin/env bash
set -euo pipefail
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
remote_file="/tmp/trick-caddy-$$.caddy"
trap 'ssh -o BatchMode=yes caddy-admin "rm -f -- $remote_file" >/dev/null 2>&1 || true' EXIT
scp -q "$repo/deploy/caddy/trick.caddy" "caddy-admin:$remote_file"
ssh -o BatchMode=yes caddy-admin "sudo -n bash -s -- $remote_file" < "$repo/deploy/caddy/apply-route.sh"
