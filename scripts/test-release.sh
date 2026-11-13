#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scripts/test-release.sh [--browser-smoke | --browser-bot-smoke | --browser-full] [--race]
                               [--browser-bin PATH] [--artifacts DIR]

Default: focused Go packages (including the ten-hand black-box fixtures) and
the root build. Browser tests are opt-in and run once per invocation.
USAGE
}

browser_mode=none
run_race=false
browser_bin=${BROWSER_BIN:-}
artifact_root=artifacts/browser-e2e

while (($#)); do
  case "$1" in
    --browser-smoke)
      [[ "$browser_mode" == none ]] || { usage >&2; exit 2; }
      browser_mode=smoke
      shift
      ;;
    --browser-bot-smoke)
      [[ "$browser_mode" == none ]] || { usage >&2; exit 2; }
      browser_mode=bot-smoke
      shift
      ;;
    --browser-full)
      [[ "$browser_mode" == none ]] || { usage >&2; exit 2; }
      browser_mode=full
      shift
      ;;
    --race)
      run_race=true
      shift
      ;;
    --browser-bin)
      (($# >= 2)) || { usage >&2; exit 2; }
      browser_bin=$2
      shift 2
      ;;
    --artifacts)
      (($# >= 2)) || { usage >&2; exit 2; }
      artifact_root=$2
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      printf 'Unknown option: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

if command -v go >/dev/null 2>&1; then
  go_cmd=$(command -v go)
elif [[ -x /usr/local/go/bin/go ]]; then
  go_cmd=/usr/local/go/bin/go
else
  printf 'Go was not found on PATH or at /usr/local/go/bin/go\n' >&2
  exit 2
fi

export GOCACHE=${GOCACHE:-/tmp/trick-evolution-go-cache}
export GOMODCACHE=${GOMODCACHE:-/tmp/trick-evolution-mod-cache}
mkdir -p "$GOCACHE" "$GOMODCACHE"

build_dir=$(mktemp -d /tmp/trick-evolution-release.XXXXXX)
trap 'rm -rf -- "$build_dir"' EXIT

printf '[D1] Focused Go packages and ten-hand HTTP/WebSocket fixtures\n'
"$go_cmd" test ./domain ./cards ./websocket ./html ./gameui -count=1 -timeout=60s

printf '[D1] Root build\n'
"$go_cmd" build -o "$build_dir/trick-evolution" .
"$go_cmd" build -o "$build_dir/browser-e2e" ./cmd/browser-e2e

if [[ "$run_race" == true ]]; then
  printf '[D1] Targeted concurrent-room race test\n'
  "$go_cmd" test -race ./gameui -run '^TestConcurrentRooms$' -count=1 -timeout=120s
fi

if [[ "$browser_mode" != none ]]; then
  printf '[D1] Real-browser %s gate\n' "$browser_mode"
  browser_args=(-mode "$browser_mode" -artifacts "$artifact_root")
  if [[ -n "$browser_bin" ]]; then browser_args+=(-browser "$browser_bin"); fi
  "$build_dir/browser-e2e" "${browser_args[@]}"
fi

printf '[D1] Release gate passed\n'
