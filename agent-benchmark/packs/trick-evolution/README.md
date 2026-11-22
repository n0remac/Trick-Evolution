# Trick Evolution task pack, milestone 1

This pack lives in the `agent-benchmark/` subdirectory of the Trick Evolution repository. It remains a separate Go module. The generator reads a
local copy of [n0remac/Trick-Evolution](https://github.com/n0remac/Trick-Evolution)
with the pinned commits available, exports source trees, and creates new
single-commit Git repositories. It does not modify the source repository. The
generated candidate repositories contain no source history, evaluator tests,
reference patches, or solution commits.

## Pinned provenance

| Task | Source revision | Historical snapshot | Co-located snapshot |
| --- | --- | --- | --- |
| Invite | `efc7a7f542c049cc4e93703544390c8debbbd1f4` | `fdca2c35c1ebb7ce28794a7802b60d1a03cd4ab9` | `df4f2b205b0c1f23a8037bad9e2fd153cd721ed8` |
| Removal | `94d093326a37171b01f5dd0ed7e33b3914c247b6` | `c29654622d514bba40bd196515cbf5f7978fda32` | `01a675d6fe6779605ccf0b3cbb996aa1db4f1249` |

The invite source is the parent of the historical implementation commit
`9a66f72baea1cfce08201782f5b004d6791768a6`. The generator uses that
commit privately to derive an invite-only reference patch; it omits the
commit's separate opening-rule changes. Removal is an unpublished benchmark
feature with a private generated reference patch. Neither reference is part
of a candidate repository.

The historical snapshot is an archive of its source revision. The co-located
snapshot moves existing GoDom controls, registration/dispatch wiring, and
supporting HTTP handlers into a feature file. Invite moves the join screen,
join and Endless handlers, and route registration. Removal moves the draft
form and controls and extracts draft command dispatch. Domain rules stay in
`domain`. All other source, `go.mod`, `go.sum`, source README, and typed command
decoding are identical within each pair. The task prompt, tools, run budget,
and evaluator are identical within each pair. The generator uses fixed Git
author and commit times, so the snapshot SHAs are reproducible.

## Recreate and run

From the nested `agent-benchmark` root, with a local source copy that has
all three source commits:

```sh
python3 packs/trick-evolution/scripts/build_pack.py \
  --source .. \
  --out packs/trick-evolution/_generated
sh packs/trick-evolution/scripts/check_regressions.sh
python3 packs/trick-evolution/scripts/check_layouts.py packs/trick-evolution/_generated
sh packs/trick-evolution/scripts/calibrate.sh
python3 packs/trick-evolution/scripts/prepare_deps.py
sh packs/trick-evolution/scripts/check_agent_deps.sh
sh packs/trick-evolution/scripts/check_browser.sh invite historical
sh packs/trick-evolution/scripts/check_browser.sh invite colocated
sh packs/trick-evolution/scripts/check_browser.sh removal historical
sh packs/trick-evolution/scripts/check_browser.sh removal colocated
```

Use `--force` only to replace this pack's existing `_generated` directory. The
generator prints the full source and snapshot SHAs. Generated manifests live
at `_generated/<task>/<layout>/task.json`; the repository named there has just
the one baseline commit. The prompt is the same text from `prompts/<task>.md`
for both layouts. Reference patches and evaluator files are siblings of the
candidate repositories. The generic harness sees only its manifest and fake
agent patch path. `calibrate.sh` runs all eight combinations: unchanged
behavior via an inert marker patch and each private known-good patch.

The project pins dependency versions in its `go.mod` and `go.sum`. On this Pi,
`/usr/local/go/bin/go mod download` was needed once to fill missing public Go
modules. The evaluator and regression scripts then run with `GOPROXY=off`,
`GOSUMDB=off`, and `GOTOOLCHAIN=local`. The evaluator runs targeted domain and
server/UI tests in the clean grading checkout after copying its tests from
outside that checkout. It uses no live model calls. `check_browser.sh` uses
`/usr/bin/chromium` through the source project's CDP browser runner in a
temporary patched checkout. Browser tests are a separate smoke check; the
harness acceptance command is server/domain based so Chromium timing does not
determine the primary outcome.

The Codex adapter gets a separate dependency snapshot at
`_generated/deps/modcache`, outside every candidate repository. Run
`prepare_deps.py` after generating the four snapshots; it reads the existing
local Go download cache without a network fallback, resolves the packages used
by the regression and evaluator checks, and records all four baseline and
`go.mod`/`go.sum` hashes in `_generated/deps/manifest.json`. Supply that path
with `-go-mod-cache`. The adapter mounts it read-only and keeps `GOPROXY=off`;
its per-attempt build cache is fresh. `check_agent_deps.sh` verifies package
resolution for all four layouts inside a Bubblewrap boundary. This snapshot
covers the listed task packages, not every unrelated package in the module.
On this Pi, Go may print a nonfatal read-only stat-cache warning for a
synthetic baseline module version. A compile-only
`go test ./domain ./gameui -run '^$'` succeeded inside the boundary; the
server/domain acceptance checks were calibrated separately in clean grading
checkouts.

## Tasks and checks

The invite prompt requires an independently authorized guest to enter the
same room and current hand from a share link. The evaluator accepts both an
invite returned when Endless starts and an invite requested later by the live
player. It checks private websocket authority, separate sessions, both
players' cards and views, turn, trick, rules, scores, invalid and wrong-room
invites, expired or closed-room invites, a full table, and ordinary joins.
Invite acceptance tests use in-memory HTTP connections so they can run when
the host blocks listening sockets. The browser smoke uses two isolated
Chromium contexts to exercise share and join; it recognizes either share
control used by the two supported workflows.

The removal prompt defines an eligible target as a currently active rule
instance introduced after match start. Starting baselines and lobby-selected
initial rules are excluded. A removal must preserve required mechanics,
active dependents, and a legal next hand plan. At a draft, the host may use
the typed `remove_rule` command with existing `rule` and `boundaryId` fields.
The evaluator checks non-host, stale, absent, starting, dependency-breaking,
and repeated actions leave state unchanged, then checks one valid removal
marks history, clears the boundary, advances to hand-complete, and increments
revision once. Its server check verifies the draft control and WebSocket
command. The browser smoke autoplays to a draft and clicks the removal control.

The [fixed rubric](RUBRIC.md) awards points for core outcome, invalid-action
handling, page interaction, and regression safety. It is separate from the
candidate checkout and is used unchanged for both layouts. Checks observe
state, HTTP/WebSocket behavior, rendered controls, and browser outcomes;
they do not assert implementation file names or code organization.

## Verification performed on this Pi

- Four baseline snapshots produced the full SHAs above. The same existing
  regression packages (`animation`, `cards`, `cardtable`, `cmd/browser-e2e`,
  `domain`, `gameui`, `scene`) passed on all four baselines in disposable
  checkouts. A full `go test ./...` was attempted; the first pass spent over
  ten minutes compiling `go-sqlite3` C code for the unrelated database
  package, so it was stopped. The focused regression gate completed.
- The current Go harness with its fake patch adapter produced eight
  JSON/Markdown reports in `_generated/results`: each unchanged baseline
  failed acceptance, and each matching reference patch passed acceptance.
  The inert marker patch ensures a baseline failure reaches the evaluator
  rather than failing patch application.
- Both invite browser checks passed. Removal browser smoke passed on the
  historical layout and on the co-located layout on retry. The first
  co-located removal browser run hit a Chromium CDP read timeout; its retry
  passed. Browser timing is a separate observation from acceptance.

The final harness run reported these exact results (total elapsed time includes
fresh local clones and grading):

| Task/layout | Baseline probe | Reference patch |
| --- | --- | --- |
| Invite historical | fail, exit 1, 42,175 ms | pass, exit 0, 73,379 ms |
| Invite co-located | fail, exit 1, 74,043 ms | pass, exit 0, 47,045 ms |
| Removal historical | fail, exit 1, 53,122 ms | pass, exit 0, 37,420 ms |
| Removal co-located | fail, exit 1, 75,748 ms | pass, exit 0, 33,450 ms |

Browser smoke durations were 14.35 seconds and 5.38 seconds for historical
and co-located invite; removal took 19.03 seconds historically and 107.58
seconds on the successful co-located retry. The failed co-located browser run
stopped at 79.92 seconds with a CDP read timeout. A later historical invite
rerun passed in 113.75 seconds, so the separate browser smoke now allows
three minutes for slow Pi runs. The source repository was
clean after all checks, and each generated candidate repository still had
one commit and a clean working tree.

## Later one-agent comparison

Run one fixed agent configuration across two layouts, two tasks, and three
repeats: twelve attempts. Start a fresh agent session and fresh harness
checkout for every attempt. Give the agent only its prompt and candidate
checkout; keep evaluator, rubric, references, source history, and prior
attempts outside its tools. Hold Go version, dependency cache, tool access,
model settings, task budget (900 seconds), and acceptance timeout (300 seconds)
constant. Use the same evaluator and review rubric for both layouts of each
task. Record acceptance, elapsed time, browser status, and rubric evidence.

Vary run order to reduce drift. One balanced sequence is:

| Repeat | Attempt order |
| --- | --- |
| 1 | Invite historical → Removal co-located → Invite co-located → Removal historical |
| 2 | Removal historical → Invite co-located → Removal co-located → Invite historical |
| 3 | Invite co-located → Removal historical → Invite historical → Removal co-located |

No paid agent trial or reviewer call is part of this milestone. Trick-point
restoration and a top-bar task are future additions.

## One live shakedown, when authorized

Use the invite historical snapshot first. It has a calibrated failing
baseline, passing reference patch, and independent browser smoke. Choose one
explicit Codex model and keep the adapter's `medium` reasoning effort, 900
second agent budget, and 300 second evaluator timeout. Use a new result
directory and the same prepared cache intended for the comparison:

```sh
model=YOUR_CODEX_MODEL
out="$PWD/packs/trick-evolution/_generated/live-shakedown/invite-historical-001"
/usr/local/go/bin/go run ./cmd/agent-benchmark \
  -adapter codex \
  -pack packs/trick-evolution/_generated/invite/historical/task.json \
  -out "$out" -model "$model" -reasoning-effort medium \
  -go-mod-cache "$PWD/packs/trick-evolution/_generated/deps/modcache"
```

Before starting, run `/usr/local/go/bin/go test ./...`,
`sh testdata/tiny/check-codex-stub.sh`, and
`sh packs/trick-evolution/scripts/check_agent_deps.sh`; confirm `codex`,
Bubblewrap, and local authentication are available. A live run needs network
access for Codex's model API, while agent shell tools remain offline. Inspect
`result.json`, `result.md`, `candidate.patch`, JSONL events, and stderr. A
backend or isolation failure is a shakedown result, not a task score. If a
candidate patch was captured, run the browser smoke against that patch in a
disposable checkout:

```sh
sh packs/trick-evolution/scripts/check_browser.sh invite historical "$out/candidate.patch"
```

The browser smoke is reported separately from the server/domain acceptance
result. Record the exact model, CLI version, prepared-cache manifest, harness
Git revision, and output paths. Freeze those inputs before beginning the
balanced twelve-attempt comparison; one shakedown is not a layout benchmark.

## Regrade a saved invite patch offline

The saved `invite-historical-002` candidate can be graded with the fake patch
adapter. This starts no Codex session and writes into `_generated/regrade`,
leaving the original `live-shakedown` report and patch untouched:

```sh
/usr/local/go/bin/go run ./cmd/agent-benchmark \
  -pack packs/trick-evolution/_generated/invite/historical/task.json \
  -patch packs/trick-evolution/_generated/live-shakedown/invite-historical-002/candidate.patch \
  -saved-result packs/trick-evolution/_generated/live-shakedown/invite-historical-002/result.json \
  -out packs/trick-evolution/_generated/regrade/invite-historical-002
```

The regrade report describes this offline acceptance run. Its `adapter` is
`fake`, so its elapsed time is not the original agent time. Its `cost.source`
labels the token counts as saved Codex usage, and the original model usage
remains in the untouched live report. For that saved usage,
1,356,986 total input tokens include 1,313,408 cached and 43,578 uncached
input tokens. Its 11,406 output tokens already include 2,499 reasoning
tokens. Actual billed API dollars are unknown, and no pricing-based dollar
estimate is possible without explicit rates. See
[`regrades/invite-historical-002.md`](regrades/invite-historical-002.md) for
the acceptance comparison.
