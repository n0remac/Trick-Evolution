#!/bin/sh
set -eu

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
  echo "usage: sh check_browser.sh TASK LAYOUT [CANDIDATE_PATCH]" >&2
  exit 2
fi
task=$1
layout=$2
pack_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
generated="$pack_root/_generated"
patch=${3:-"$generated/$task/$layout.patch"}
patch=$(realpath "$patch")
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
git clone --quiet --no-hardlinks "$generated/$task/$layout/repo" "$work/repo"
git -C "$work/repo" apply "$patch"
BENCH_CHECKOUT="$work/repo" sh "$pack_root/_evaluator/browser.sh" "$task"
