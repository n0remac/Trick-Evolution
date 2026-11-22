#!/bin/sh
set -eu

test "$PWD" = /workspace
test -f .git/shallow
git status --short >/dev/null
test "$GOMODCACHE" = /gomodcache
test "$GOPROXY" = off
test -f /gomodcache/cache/download/marker
if touch /gomodcache/cache/download/write-probe 2>/dev/null; then echo 'module cache is writable' >&2; exit 14; fi
if touch .git/write-probe 2>/dev/null; then echo '.git is writable' >&2; exit 12; fi
test "$(cat)" = 'Change answer.txt so it contains correct followed by a newline.'
case " $* " in *' --sandbox workspace-write '*) ;; *) echo 'sandbox flag missing' >&2; exit 8;; esac
case " $* " in *' approval_policy="never" '*) ;; *) echo 'approval policy missing' >&2; exit 9;; esac
case " $* " in *' sandbox_workspace_write.network_access=false '*) ;; *) echo 'tool network policy missing' >&2; exit 10;; esac
case " $* " in *' web_search="disabled" '*) ;; *) echo 'web search policy missing' >&2; exit 11;; esac
case " $* " in *' shell_environment_policy.inherit="all" '*) ;; *) echo 'shell environment policy missing' >&2; exit 13;; esac

printf 'correct\n' > answer.txt
printf 'Stub finished.\n' > /artifacts/last-message
printf '%s\n' '{"type":"thread.started","thread_id":"offline-stub"}'
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":12,"cached_input_tokens":3,"output_tokens":4}}'
