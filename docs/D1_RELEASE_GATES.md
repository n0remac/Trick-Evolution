# D1 complete-game verification and release gates

D1 is a verification milestone. It does not change game rules or deploy the service. All complete-game tests use a fixed shuffle sequence in a test server; the production server still chooses its own random seeds.

## Run the gates

From a clean checkout with Go 1.25 or newer:

```sh
scripts/test-release.sh
```

This runs the focused `domain`, `cards`, `websocket`, and `gameui` Go packages once, including the ten-hand black-box WebSocket fixtures and GoDom HTML checks, then builds the root server and browser runner. The script defaults Go build and module caches and GOPATH to writable `/tmp/trick-evolution-*` directories. It exits on the first failed assertion or build.

With Chromium or Chrome installed, choose **one** browser mode for a run:

```sh
scripts/test-release.sh --browser-smoke
scripts/test-release.sh --browser-bot-smoke
scripts/test-release.sh --browser-full
```

The fast smoke joins two isolated browser contexts and plays the first trick. The bot smoke joins alone, adds a bot from the lobby, scores a trick, ends the game from the UI, reloads the terminal state, and returns to join. The `3d-smoke` mode tests pointer selection, local motion, an accepted bot play, renderer reconciliation, resize and reconnect on a phone-size viewport. Full mode uses the actual UI to finish all ten hands, checks the rule reveals and scoreboard after every hand, and reloads the guest during hand 5 and after game completion. It has a 180-second overall bound; smoke modes have 45 seconds. All launch headless Chromium with background timer throttling disabled. Use `--browser-bin /path/to/chrome` or `BROWSER_BIN=/path/to/chrome` if auto-detection does not find the browser. Use `--artifacts DIR` to choose where reports and failure files go. The runner can also be invoked directly:

```sh
go run ./cmd/browser-e2e -mode full -browser /usr/bin/chromium
```

Each browser run writes `result.json` and `chromium.log` under `artifacts/browser-e2e/<timestamp>-<mode>-<pid>/`. On failure it also writes `failure.txt` with the last completed steps, a screenshot, DOM HTML, UI state JSON, and console/exception JSON for each page that reached DevTools. The runner prints that directory and exits nonzero. The artifacts directory is ignored by Git.

The optional `scripts/test-release.sh --race` runs the targeted concurrent-room race test. It requires a host whose ThreadSanitizer supports its virtual-memory layout. A failed or unrun race check must not be reported as passed.

## What the automated tests assert

`TestCompleteGameBlackBox` is in an external Go test package. It creates rooms with HTTP joins and interacts only through authenticated WebSocket commands and projections. Its 2- and 4-player fixtures each play ten complete five-trick hands. They assert:

- Reveals occur exactly after hands 3, 6, and 9, after hand-completion events.
- Every accepted command advances the revision by one; the final revisions are 110 and 210.
- Every hand conserves its dealt cards across private hands, visible plays, and public captured counts; exactly 50 tricks and 50 points are awarded in each match.
- Fixed seeded final scores are **28–22** for two players and **10–13–16–11** for four players. The public winners and final game event agree with the highest score.
- Public payloads and each opponent's payload omit all unplayed private cards at every revision.
- A midgame reconnect restores the private hand; a completed-game reconnect restores the final state. Replayed action IDs do not advance the revision. Cross-room tokens and forged identity fields are refused. Play and next-hand commands are refused after game completion.

The browser runner asserts the two-seat UI's exact 28–22 outcome, ten hand boundaries, reveals at 3/6/9, winner text, and absence of JavaScript exceptions. Its separate smoke mode reaches one awarded trick. These checks exercise the real page and its WebSocket connection, rather than calling `domain.Step` directly.

## CI and current evidence

`.github/workflows/d1-release.yml` runs the lightweight gate on pushes and pull requests. A manually triggered workflow with `full_browser=true` runs the full Chromium gate and targeted race check on an Ubuntu x64 runner. Browser artifacts are uploaded when that job fails. This workflow has been committed but has **not run remotely** because this branch was not pushed.

Local evidence on 2026-11-12:

| Check | Result |
| --- | --- |
| `go test ./gameui -run TestCompleteGameBlackBox -count=1 -timeout=30s` | Passed: two- and four-player complete matches. |
| `scripts/test-release.sh` | Passed: focused packages and root build. |
| `scripts/test-release.sh --browser-smoke --browser-bin /usr/bin/chromium` | Passed: combined gate and short UI check. |
| `go run ./cmd/browser-e2e -mode smoke -browser /usr/bin/chromium` | Passed in 2.43s. |
| `go run ./cmd/browser-e2e -mode full -browser /usr/bin/chromium` | Passed in 19.293s; ten hands, 28–22, reconnect. |
| `go test -race ./gameui -run TestConcurrentRooms -count=1` | **Unverified:** the Pi's ThreadSanitizer reported `unsupported VMA range`. |

The unsupported race run and unexecuted remote workflow are verification limits. D2 deployment must remain separate; no service, timer, proxy route, or StarCoaster configuration was changed for D1.
