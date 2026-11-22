#!/bin/sh
set -eu

pack_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
generated=${1:-"$pack_root/_generated"}
cache="$generated/deps/modcache"
test -d "$cache/cache/download"
test -f "$generated/deps/manifest.json"

for task in invite removal; do
  for layout in historical colocated; do
    repo="$generated/$task/$layout/repo"
    bwrap --unshare-all --share-net --die-with-parent --clearenv \
      --ro-bind /usr /usr --symlink usr/bin /bin --symlink usr/lib /lib \
      --dev /dev --proc /proc --tmpfs /tmp \
      --ro-bind "$repo" /workspace --ro-bind "$cache" /gomodcache \
      --setenv HOME /tmp --setenv PATH /usr/local/go/bin:/usr/bin:/bin \
      --setenv GOCACHE /tmp/gocache --setenv GOMODCACHE /gomodcache \
      --setenv GOPROXY off --setenv GOSUMDB off --setenv GOTOOLCHAIN local \
      --setenv GOWORK off --chdir /workspace \
      /usr/local/go/bin/go list -deps -test \
      ./animation ./cards ./cardtable ./cmd/browser-e2e ./domain ./gameui ./scene \
      >/dev/null
    printf 'offline dependencies: %s/%s passed\n' "$task" "$layout"
  done
done
