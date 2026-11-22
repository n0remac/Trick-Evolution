#!/bin/sh
set -eu

task=$1
evaluator_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cp "$evaluator_dir/browser_test.go" "$BENCH_CHECKOUT/cmd/browser-e2e/benchmark_browser_test.go"
cd "$BENCH_CHECKOUT"
case "$task" in
  invite) test_name=TestBenchmarkBrowserInvite ;;
  removal) test_name=TestBenchmarkBrowserRemoval ;;
  *) echo "unknown task: $task" >&2; exit 2 ;;
esac
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off /usr/local/go/bin/go test ./cmd/browser-e2e -run "^$test_name$" -count=1 -timeout=4m -v
