#!/bin/sh
set -eu

task=$1
evaluator_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
case "$task" in
  invite)
    cp "$evaluator_dir/invite_test.go" "$BENCH_CHECKOUT/gameui/benchmark_invite_test.go"
    ;;
  removal)
    cp "$evaluator_dir/removal_domain_test.go" "$BENCH_CHECKOUT/domain/benchmark_removal_test.go"
    cp "$evaluator_dir/removal_server_test.go" "$BENCH_CHECKOUT/gameui/benchmark_removal_test.go"
    ;;
  *)
    echo "unknown task: $task" >&2
    exit 2
    ;;
esac
cd "$BENCH_CHECKOUT"
GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off /usr/local/go/bin/go test ./domain ./gameui -run '^TestBenchmark' -count=1 -timeout=4m
