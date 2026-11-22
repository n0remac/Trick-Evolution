#!/bin/sh
set -eu

pack_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
workspace=$(CDPATH= cd -- "$pack_root/../.." && pwd)
generated=${1:-"$pack_root/_generated"}
python3 "$pack_root/scripts/check_layouts.py" "$generated"
mkdir -p "$generated/bin"
cd "$workspace"
/usr/local/go/bin/go build -o "$generated/bin/agent-benchmark" ./cmd/agent-benchmark
for task in invite removal; do
  for layout in historical colocated; do
    pack="$generated/$task/$layout/task.json"
    for fixture in missing reference; do
      if [ "$fixture" = missing ]; then
        patch="$pack_root/reference/missing.patch"
        expected=1
      else
        patch="$generated/$task/$layout.patch"
        expected=0
      fi
      report="$generated/results/$task/$layout/$fixture"
      if "$generated/bin/agent-benchmark" -pack "$pack" -patch "$patch" -out "$report"; then
        status=0
      else
        status=$?
      fi
      if [ "$status" -ne "$expected" ]; then
        echo "unexpected $task/$layout/$fixture exit $status (wanted $expected)" >&2
        cat "$report/result.md" >&2
        exit 1
      fi
    done
  done
done
python3 "$pack_root/scripts/check_reports.py" "$generated/results"
