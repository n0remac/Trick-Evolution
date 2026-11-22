#!/bin/sh
set -eu
if printf 'correct\n' | cmp -s - "$BENCH_CHECKOUT/answer.txt"; then
  echo "answer accepted"
else
  actual=$(cat "$BENCH_CHECKOUT/answer.txt")
  echo "expected correct, got $actual" >&2
  exit 1
fi
