#!/usr/bin/env bash
# An initial, explicit local commit may bootstrap the service. The timer always
# invokes --main, which fetches and deploys only origin/main.
set -Eeuo pipefail

usage() { echo 'Usage: deploy-dev-pi.sh --main [--retry] | --from-local COMMIT' >&2; exit 2; }
mode=${1:-}; shift || true
retry=false
case "$mode" in
  --main) [[ ${1:-} == --retry ]] && { retry=true; shift; }; (($# == 0)) || usage ;;
  --from-local) (($# == 1)) || usage; local_commit=$1; shift ;;
  *) usage ;;
esac

root=${TRICK_DEPLOY_ROOT:-/home/pi/services/trick-evolution}
origin_url=${TRICK_DEPLOY_ORIGIN_URL:-https://github.com/n0remac/Trick-Evolution.git}
local_repo=${TRICK_DEPLOY_LOCAL_REPO:-/home/pi/Projects/trick-evolution}
go_cmd=${TRICK_DEPLOY_GO:-/usr/local/go/bin/go}
mkdir -p "$root"/{releases,logs,state}
exec 9>"$root/deploy.lock"
flock -n 9 || { echo 'Another Trick Evolution deployment is running'; exit 0; }
exec > >(tee -a "$root/logs/deploy.log") 2>&1
echo "$(date -Is) deployment mode $mode"

source_git=$root/source.git
if [[ ! -d $source_git ]]; then
  git init --bare -q "$source_git"
  git --git-dir="$source_git" remote add origin "$origin_url"
fi
git --git-dir="$source_git" fetch -q --no-tags origin refs/heads/main:refs/heads/main
main_head=$(git --git-dir="$source_git" rev-parse refs/heads/main)
seen_file=$root/state/last-main-seen
failed_file=$root/state/failed-main
if [[ $mode == --main ]]; then
  target=$main_head
  if [[ -f $seen_file ]]; then
    seen=$(cat "$seen_file")
    if [[ $seen == "$target" && $retry == false ]]; then
      echo "origin/main unchanged at $target; no gate run"
      exit 0
    fi
    git --git-dir="$source_git" merge-base --is-ancestor "$seen" "$target" || {
      echo "origin/main was rewritten; refusing automatic deployment ($seen -> $target)" >&2
      exit 1
    }
  fi
  if [[ -f $failed_file && $(cat "$failed_file") == "$target" && $retry == false ]]; then
    echo "origin/main $target failed before; awaiting a new commit or --retry"
    exit 0
  fi
  archive_repo="git --git-dir=$source_git"
else
  [[ $local_commit =~ ^[0-9a-f]{40}$ ]] || usage
  target=$(git -C "$local_repo" rev-parse --verify "$local_commit^{commit}")
  [[ $target == "$local_commit" ]] || usage
  archive_repo="git -C $local_repo"
fi

release=$root/releases/$target
stage=''
switched=false
previous=''
cleanup() { [[ -z $stage || ! -d $stage ]] || rm -rf -- "$stage"; }
rollback() {
  rc=$?
  trap - ERR
  set +e
  echo "Deployment of $target failed (exit $rc)"
  if [[ $mode == --main ]]; then printf '%s\n' "$target" > "$failed_file"; fi
  if [[ $switched == true ]]; then
    if [[ -n $previous ]]; then
      ln -s "$previous" "$root/.current.rollback"
      mv -Tf "$root/.current.rollback" "$root/current"
      systemctl --user restart trick-evolution.service
      echo "Rolled back to $previous"
    else
      systemctl --user stop trick-evolution.service
      rm -f "$root/current"
      echo 'Stopped first release after failed health check'
    fi
  fi
  cleanup
  exit "$rc"
}
trap cleanup EXIT
trap rollback ERR

if [[ ! -d $release ]]; then
  stage=$(mktemp -d "$root/releases/.build.XXXXXXXX")
  mkdir -p "$stage/source" "$stage/bin"
  if [[ $mode == --main ]]; then
    git --git-dir="$source_git" archive "$target" | tar -x -C "$stage/source"
  else
    git -C "$local_repo" archive "$target" | tar -x -C "$stage/source"
  fi
  cd "$stage/source"
  export GOCACHE=${GOCACHE:-/tmp/trick-evolution-go-cache}
  export GOMODCACHE=${GOMODCACHE:-/tmp/trick-evolution-mod-cache}
  export GOFLAGS="${GOFLAGS:+$GOFLAGS }-buildvcs=false"
  mkdir -p "$GOCACHE" "$GOMODCACHE"
  echo "Running D1 gate once for $target"
  bash scripts/test-release.sh
  "$go_cmd" build -ldflags "-X main.buildCommit=$target" -o "$stage/bin/trick-evolution" .
  "$go_cmd" build -o "$stage/bin/deploy-smoke" ./cmd/deploy-smoke
  printf '%s\n' "$target" > "$stage/COMMIT"
  chmod -R a-w "$stage"
  mv "$stage" "$release"
  stage=''
else
  [[ $(cat "$release/COMMIT") == "$target" ]] || { echo 'Release commit mismatch' >&2; exit 1; }
  echo "Reusing tested release $target"
fi

if [[ -L $root/current ]]; then previous=$(readlink "$root/current"); fi
if [[ $previous != "releases/$target" ]]; then
  ln -s "releases/$target" "$root/.current.next"
  mv -Tf "$root/.current.next" "$root/current"
  switched=true
fi
systemctl --user restart trick-evolution.service
healthy=false
for attempt in {1..10}; do
  if "$root/current/bin/deploy-smoke" -url http://127.0.0.1:18081 -commit "$target"; then healthy=true; break; fi
  sleep 1
done
[[ $healthy == true ]] || { echo 'Health check failed' >&2; exit 1; }
printf '%s\n' "$target" > "$root/state/deployed.tmp"
mv -f "$root/state/deployed.tmp" "$root/state/deployed"
if [[ $mode == --main ]]; then
  printf '%s\n' "$target" > "$seen_file.tmp"
  mv -f "$seen_file.tmp" "$seen_file"
  rm -f "$failed_file"
else
  # Preserve the main baseline: an initial feature release must not be
  # immediately replaced by the older, unchanged main branch.
  if [[ ! -f $seen_file ]]; then
    printf '%s\n' "$main_head" > "$seen_file.tmp"
    mv -f "$seen_file.tmp" "$seen_file"
  fi
fi
echo "$(date -Is) deployed $target; origin/main baseline $(cat "$seen_file")"
