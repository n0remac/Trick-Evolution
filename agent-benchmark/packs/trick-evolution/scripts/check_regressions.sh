#!/bin/sh
set -eu

pack_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
for task in invite removal; do
  for layout in historical colocated; do
    repo="$pack_root/_generated/$task/$layout/repo"
    checkout="$work/$task-$layout"
    git clone --quiet --no-hardlinks "$repo" "$checkout"
    printf '%s/%s: ' "$task" "$layout"
    (cd "$checkout" && GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off /usr/local/go/bin/go test ./animation ./cards ./cardtable ./cmd/browser-e2e ./domain ./gameui ./scene -count=1 -timeout=5m)
  done
done
