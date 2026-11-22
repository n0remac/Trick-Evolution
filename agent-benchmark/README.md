# Trick Evolution coding-agent benchmark harness

This Go module is vendored under `agent-benchmark/` in the Trick Evolution repository. Run its commands from this directory, not the game repository root. It runs a task pack with either a deterministic fixture patch or a
fresh noninteractive Codex CLI session. The runner uses the Go standard library,
Git, and (for Codex) Bubblewrap. The offline fixtures make no model calls.

## Run the tiny fixture

From the repository root:

```sh
fixture_dir=$(mktemp -d)
sh testdata/tiny/setup.sh "$fixture_dir"
/usr/local/go/bin/go run ./cmd/agent-benchmark \
  -pack "$fixture_dir/task.json" \
  -patch testdata/tiny/passing.patch \
  -out "$fixture_dir/passing"
/usr/local/go/bin/go run ./cmd/agent-benchmark \
  -pack "$fixture_dir/task.json" \
  -patch testdata/tiny/failing.patch \
  -out "$fixture_dir/failing"
```

The passing command exits 0. The failing command exits 1 because acceptance
rejects `incorrect`; `go run` also prints `exit status 1`. Both output
directories contain `result.json` and `result.md`. Inspect them with
`cat "$fixture_dir/passing/result.json"` and
`cat "$fixture_dir/failing/result.md"`. The fixture repository remains at its
baseline. Run `/usr/local/go/bin/go test ./...` for the automated fixture test.

## Add a task pack

Create a local Git repository with a committed baseline and put a JSON task
pack alongside an evaluator-owned acceptance script. For example:

```json
{
  "name": "example-task",
  "repository": "repo",
  "baseline": "0123456789abcdef0123456789abcdef01234567",
  "prompt": "Describe the requested code change here.",
  "budget_seconds": 120,
  "acceptance_command": ["sh", "{pack_dir}/accept.sh"],
  "acceptance_timeout_seconds": 60
}
```

Set `baseline` to the full hash from `git rev-parse HEAD`; the value above is
only a placeholder. `repository` is relative to the pack file, or it can be
absolute. `budget_seconds` limits the agent process. Patch capture and grading
are timed separately. `acceptance_timeout_seconds` is optional and defaults to
60 seconds.
The acceptance command is an argument array, not a shell string. Its working
directory is the grading checkout. `{pack_dir}` and `{checkout}` are replaced
in each argument; `BENCH_PACK_DIR` and `BENCH_CHECKOUT` are also set in its
environment. Keep acceptance scripts outside the candidate repository so a
candidate patch cannot change the evaluator's check.

Supply a Git-compatible patch with `-patch`. The fake adapter applies it in a
fresh local clone at the baseline. The harness captures that checkout's staged
diff against the pinned commit, creates a second fresh clone, and applies the diff
there for acceptance. This checks the patch that the attempt actually produced
and leaves the source repository untouched. Each invocation creates new
checkouts, including when a previous attempt failed.

## Codex CLI adapter

The Codex adapter starts a new `codex exec` session per attempt. The prompt is
sent on stdin, the working directory is `/workspace` inside a Bubblewrap mount
namespace, and `--ephemeral` prevents session persistence. The candidate Git
repository contains only the pinned commit and tree. The source repository,
evaluator files, fixture patches, previous results, and other host home files
are not mounted. Bubblewrap mounts the candidate read/write except for its
read-only `.git`, system binaries and Go read-only, CA certificates and DNS
configuration read-only, the single Codex `auth.json` read-only, and a new empty
scratch mount for the final message. It gives the backend its own temporary home
and `/tmp`; no host environment is passed through. If Bubblewrap cannot
start a namespace, the attempt fails at `configure_agent` before Codex runs.
The agent can inspect Git status but cannot stage or commit; the runner stages
the resulting working tree after the session ends.

Codex uses `--sandbox workspace-write`, `approval_policy="never"`, disabled
shell network access, disabled web search, ignored user config and exec rules,
and a curated shell environment containing paths and offline Go settings. The backend retains host network
access for its authenticated model requests; commands run by Codex are subject
to its workspace-write sandbox and network policy. The adapter does not request
automatic review, extra writable directories, MCP tools, or full access.
Codex's own nested sandbox must work on the host for a live attempt; the offline
stub verifies the outer boundary and invocation without calling a model.

For a Go task with external modules, pass `-go-mod-cache DIR` pointing to a
prepared Go module cache. Bubblewrap mounts that directory read-only at
`/gomodcache`; Go receives `GOMODCACHE=/gomodcache` and `GOPROXY=off`. The
default is an empty private cache, which is suitable for the tiny fixture but
cannot compile a project with uncached modules. Prepare the cache outside the
candidate checkout and use the same snapshot for every compared layout. The
[Trick Evolution pack](packs/trick-evolution/README.md) provides an offline
preparation script and a four-layout boundary check.

Run the complete offline CLI shakedown with:

```sh
sh testdata/tiny/check-codex-stub.sh
```

For a future **real** shakedown, first create or select a local task pack and
ensure `codex`, `bwrap`, Git, `/usr/local/go/bin/go`, and
`~/.codex/auth.json` are present. The backend needs API network access;
agent shell commands have network disabled, so dependencies must already be
available in the candidate tree or through the system toolchain. Then run:

```sh
/usr/local/go/bin/go run ./cmd/agent-benchmark \
  -adapter codex -pack /absolute/path/to/task.json \
  -out /absolute/path/to/new-results-directory \
  -model YOUR_CODEX_MODEL -reasoning-effort medium \
  -go-mod-cache /absolute/path/to/prepared/modcache
```

Use a new output directory for each attempt. `-codex-bin PATH` and
`-codex-auth-file PATH` select a different installed CLI or auth file. The
model and reasoning effort must be chosen explicitly for a comparison; the
default effort is `medium`. The installed CLI checked for this adapter was
`codex-cli 0.166.0` on this Pi. Its `exec --help` supports `--json`, `--ephemeral`,
`--ignore-user-config`, `--ignore-rules`, `--strict-config`, `--sandbox`,
`--model`, and `--output-last-message`. The JSONL `turn.completed.usage` event
is parsed as provider data, including `cached_input_tokens`; absent usage stays
absent rather than becoming zero. The [official Codex noninteractive guide](https://learn.chatgpt.com/docs/non-interactive-mode)
documents these event names and fields.

Codex reports include a `cost` object. `actual_billed_api_usd` is `null` and
`actual_status` says that authoritative billing is unavailable; token usage is
not a bill. The runner treats `input_tokens` as total input, subtracts
`cached_input_tokens` to get uncached input, and counts `output_tokens` once.
Reasoning output is already included in output tokens. The raw usage event is
also retained under `agent.usage`. Rates are never inferred from the model
name, including `gpt-6-sol`.

To calculate a pricing-based estimate, supply all three explicit USD rates per
million tokens with a Codex run:

```sh
  -price-input-per-million INPUT_RATE \
  -price-cached-input-per-million CACHED_RATE \
  -price-output-per-million OUTPUT_RATE
```

The resulting `estimated_api_usd` uses those rates and the event token counts.
Without complete usage or caller-supplied rates it stays `null`, with a reason
in `estimate_status`. Consult your billing system for actual charged dollars;
the estimate does not include any charges absent from the usage event.
For an offline patch regrade, `-saved-result ORIGINAL_RESULT.json` attaches the
matching Codex attempt's historical usage to the new fake-adapter report. The
runner checks the candidate hash, task, and baseline before linking it. The
`cost.source` field identifies that saved usage, while the original result
remains untouched. Explicit price flags can also be supplied to this regrade.

JSON and Markdown reports include overall status, failure stage and details,
total and agent elapsed time, candidate diff hash, and acceptance exit code,
output, and duration when acceptance ran. Codex reports also record its exit
status, policy and settings, raw provider usage when reported, and paths to
`candidate.patch`, `agent-events.jsonl`, `agent-stderr.log`, and the final
message when available. Events and stderr are retained even on backend failure
or timeout. Reports include their final 16 KiB for quick diagnosis. Outputs
have private filesystem permissions; they can still contain candidate source
and agent messages, so choose a private results directory. Exit codes are 0
for accepted patches, 1 for benchmark failures, and 2 for CLI usage or
report-write errors.

For a single command that builds the CLI and verifies both outcomes, run
`sh testdata/tiny/check-cli.sh`. It prints the temporary report directory.

The first project-specific pack is documented in
[packs/trick-evolution/README.md](packs/trick-evolution/README.md).
