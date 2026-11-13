# Post-F3 roadmap: complete-game E2E, permanent dev deployment, and sandbox growth

## Review baseline (2026-11-12)

F3 commit `98ff4f3` on `feature/f3-multiplayer-integration` replaces the old game handlers with `gameui.Server` and authoritative `domain.Step` room state. Per-tab session tokens, reconnect, command deduplication, privacy projections, revisioned snapshots, origin and payload checks, idle cleanup, and a browser UI are implemented. The F3 Happier transcript reports focused Go tests and a root build passing, plus a **manual/one-off headless Chromium** full ten-hand two-tab smoke test after fixing a nil-hand rendering bug. This is not yet a committed, reproducible complete-game browser E2E suite. The checked-in WebSocket test reaches the first reveal at three hands, not the end of the default ten-hand game. The targeted Go race run did not execute because ThreadSanitizer rejected the Pi's VMA layout. No permanent Trick Evolution service or deploy timer was found during review.

StarCoaster on this Pi uses **systemd user services** for its game and a separate deploy oneshot with a periodic timer; Trick Evolution should adopt the operational pattern, not reuse StarCoaster's service, directories, or deployment hooks. The StarCoaster deployment currently has layered service drop-ins, so inspect the effective units and actual build scripts before copying details. Do not alter StarCoaster services.

## D1 — complete-game E2E and release gates (next)

1. Add a deterministic **black-box WebSocket/HTTP full-game test** using real joins and authenticated WebSocket actions, not direct `domain.Step` calls. Drive **all ten hands** and 3/6/9 rule reveals with legal plays, verify final winner(s), scoring and conservation, final phase, event/revision progression, no hidden cards in opponent/public payloads, and refusal of post-game actions. Run at least one 2-player and one 4-player fixture; seed shuffle via the test server, never production randomness. Test reconnect during a game and at completion, duplicate actions, and a forbidden cross-room command.
2. Add a **repeatable two-browser-context E2E test** (Playwright or Chromium CDP, choose the smallest reliable local setup) that uses the actual page UI and plays a **complete game**, including rule reveals, scoreboard, end screen, and reload/reconnect. Make this a checked-in test with a documented invocation and time bound, not just a Happier smoke-script transcript. Disable Chromium background timer throttling in headless runs. Keep a separate short smoke test for frequent runs.
3. Provide `scripts/test-release.sh` or equivalent to run focused Go unit/integration tests, root build, full-game WebSocket test, and optional browser E2E, with explicit flags so browser tests are not accidentally repeated in every development loop. Capture logs/screenshots on failure, avoid infinite retries, and fail the gate on assertions rather than merely successful HTTP responses.
4. Run the targeted `-race` test on a supported Linux host/CI runner; if unavailable, record it as unverified, not passed. Add CI workflow for the lightweight tests and a separately triggered full-game browser gate when practical.

**Acceptance:** clean checkout can run the documented automated full-game browser test and black-box complete-match test; both reach game completion and assert final outcomes. No release/deploy if required gates fail. The existing manual smoke remains a useful complement, not the sole evidence.

**D1 implementation status (2026-11-12): complete on `feature/d1-complete-game-gates`.** [D1_RELEASE_GATES.md](D1_RELEASE_GATES.md) documents the checked-in black-box 2- and 4-player ten-hand fixtures, isolated-context Chromium runner, fast smoke option, release script, CI triggers, failure artifacts, local evidence, and remaining verification limits. The full local browser run passed; the race check remains unverified on this Pi because ThreadSanitizer rejected its VMA layout. The manual Ubuntu x64 workflow has not run because the branch has not been pushed. D2 deployment remains untouched.

## D2 — permanent DEV Pi deployment (after D1 gate)

1. Create a dedicated, unprivileged Trick Evolution deployment layout (e.g. `/home/pi/services/trick-evolution/{releases,current,logs}`) and a **systemd user service** `trick-evolution.service` with `Restart=always`, sensible startup/shutdown limits, loopback bind on a dedicated nonconflicting port, and persistent user lingering if required. Never run from a Happier working tree.
2. Provide a versioned deployment script in the repository: fetch a pinned ref or approved branch, build in an isolated worktree/build directory with writable Go caches, run the D1 release gate, install an immutable release directory, atomically switch `current`, restart the service, health-check HTTP/WebSocket, and automatically roll back on failed health check. Record deployed commit and deployment log. Do not automatically deploy unreviewed feature branches.
3. Add `trick-evolution-deploy.service` and a timer or other explicit trigger analogous to StarCoaster's dev deployment; choose whether it polls a designated branch or is manually triggered, and document that choice. The preferred first rollout is **manual trigger**, then opt in to periodic deploys once the release gate is reliable. Keep deploy and runtime services separate.
4. Expose a stable **private dev URL** through the existing Caddy/Tailscale arrangement or a dedicated Caddy route after checking hostnames, ports, and auth boundaries. Ensure WebSocket upgrades, HTTPS/origin behavior, and secure handling of session tokens. Do not open an unauthenticated public service by accident.
5. Document `systemctl --user status/restart`, `journalctl --user -u trick-evolution`, deploy/rollback, version check, ports, health checks, and reboot persistence. Run one reboot/restart and two-client reconnect test **on dev**; note that in-memory rooms intentionally do **not** survive a process restart.

**Acceptance:** app starts on Pi boot, restarts after crash, is reachable at stable dev URL, deploys only a tested revision, and can roll back; StarCoaster remains unaffected. Important distinction: **permanent service** does not yet mean **persistent matches**.

**D2 implementation status (2026-11-12): deployed with public HTTPS.** See [D2_DEV_DEPLOYMENT.md](D2_DEV_DEPLOYMENT.md) for the public-hostname override, main-only automatic release policy, installed topology, rollback test, Porkbun DNS correction, TLS/WebSocket evidence, and remaining reboot verification limit. The original private-URL and manual-only preferences above were superseded by the user's approved public HTTPS and automatic `origin/main` requirements.

## D3 — stabilize and playtest the initial experience

- Short manual playtesting with 2–4 players; refine readability of active rules, whose turn it is, rule reveals, end-of-hand pacing, and accessibility/mobile layout.
- Decide game duration, scoring baseline, reveal frequency, and what the host controls. Track rule-teaching comprehension; use manual testing for expensive/subjective UX checks.
- Decide whether to persist room state across service restarts. If desired, add snapshots with catalog versioning, authenticated seat restoration, and a bounded recovery test as a separate milestone. Do not make persistence a prerequisite for the initial always-on deployment.

## S1 — composable rules and sandbox presets

- Introduce explicit effect families, compatibility constraints, precedence, and override-chain semantics; test compositions rather than allowing arbitrary contradictory combinations.
- Define a canonical normalized ruleset including setup, scoring, deck, bidding/trump/teams when implemented; support named presets and user-created validated presets, with save/share later.
- Preserve Evolution Mode's random rule phases, but constrain advanced rules by dependencies/teachability; retain a deliberately chaotic mode.

## S2 — learning and named-game recognition

- Add per-rule explanations and examples at reveal time, plus a brief rule summary before each hand.
- Define versioned **specific traditional-game variants** as declarative presets using the same rule mechanics as the sandbox. Detect **exact** match of normalized active rules + configuration, not loose title matching; show partial similarities separately and do not interrupt evolution.
- Expand mechanics incrementally toward a named game (e.g. a documented Pitch variant): trump, scoring, bidding, and any required team/hand rules, each with full-game fixtures and teaching content. Do not claim Pitch recognition before its complete mechanics are implemented.

## Suggested Happier session boundaries

- **Next session: D1 complete-game E2E and CI/release gates.** Keep it focused; do not deploy yet.
- **Following session: D2 dev Pi service and safe deployment.** Run D1 gates once per release, not in an expensive continuous loop. Inspect the actual StarCoaster effective service/timer and Caddy routes before changing system configuration.
- Then D3 playtesting, followed by S1/S2 in bounded slices. All work remains on a branch until reviewed; don't push or deploy without explicit authorization.
