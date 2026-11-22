#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
work=$(mktemp -d)
sh "$root/testdata/tiny/setup.sh" "$work" >/dev/null
cd "$root"
/usr/local/go/bin/go build -o "$work/agent-benchmark" ./cmd/agent-benchmark
"$work/agent-benchmark" -pack "$work/task.json" -patch testdata/tiny/passing.patch -out "$work/passing"
if "$work/agent-benchmark" -pack "$work/task.json" -patch testdata/tiny/failing.patch -out "$work/failing"; then
  echo "failing patch unexpectedly passed" >&2
  exit 1
else
  status=$?
  if [ "$status" -ne 1 ]; then
    echo "failing patch exited $status instead of 1" >&2
    exit 1
  fi
fi
test -f "$work/passing/result.json"
test -f "$work/passing/result.md"
test -f "$work/failing/result.json"
test -f "$work/failing/result.md"
printf 'CLI fixture passed; reports: %s\n' "$work"
