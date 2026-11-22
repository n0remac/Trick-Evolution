#!/bin/sh
set -eu

fixture_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if [ "$#" -ne 1 ]; then
  echo "usage: sh testdata/tiny/setup.sh OUTPUT_DIR" >&2
  exit 2
fi
output=$1
mkdir -p "$output/repo"
output=$(CDPATH= cd -- "$output" && pwd)
git -C "$output/repo" init --quiet -b main
cp "$fixture_dir/base/answer.txt" "$output/repo/answer.txt"
git -C "$output/repo" add answer.txt
git -C "$output/repo" -c user.name='Benchmark Fixture' -c user.email='fixture@example.invalid' commit --quiet -m baseline
baseline=$(git -C "$output/repo" rev-parse HEAD)
cp "$fixture_dir/accept.sh" "$output/accept.sh"
cat > "$output/task.json" <<EOF
{
  "name": "tiny-answer",
  "repository": "repo",
  "baseline": "$baseline",
  "prompt": "Change answer.txt so it contains correct followed by a newline.",
  "budget_seconds": 10,
  "acceptance_command": ["sh", "{pack_dir}/accept.sh"]
}
EOF
printf '%s\n' "$output"
