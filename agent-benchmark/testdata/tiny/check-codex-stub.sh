#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
work=$(mktemp -d)
sh "$root/testdata/tiny/setup.sh" "$work" >/dev/null
printf '{}\n' > "$work/auth.json"
mkdir -p "$work/module-cache/cache/download"
printf 'prepared\n' > "$work/module-cache/cache/download/marker"
cd "$root"
/usr/local/go/bin/go build -o "$work/agent-benchmark" ./cmd/agent-benchmark
"$work/agent-benchmark" -pack "$work/task.json" -out "$work/codex-stub" \
  -adapter codex -codex-bin "$root/testdata/tiny/stub-codex.sh" \
  -codex-auth-file "$work/auth.json" -go-mod-cache "$work/module-cache" \
  -model stub-model -reasoning-effort low
test -f "$work/codex-stub/candidate.patch"
test -f "$work/codex-stub/agent-events.jsonl"
test -f "$work/codex-stub/agent-stderr.log"
test -f "$work/codex-stub/agent-last-message.txt"
test -f "$work/codex-stub/result.json"
test -f "$work/codex-stub/result.md"
grep -q '"passed": true' "$work/codex-stub/result.json"
grep -q '"cached_input_tokens": 3' "$work/codex-stub/result.json"
printf 'Codex stub CLI fixture passed; reports: %s\n' "$work"
