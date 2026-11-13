# U1 — shared GoDom migration

Status: implemented and locally verified on `feature/u1-godom-migration`; deployment evidence follows below. Approved 2026-11-12.
Parent plan: [UI_GODOM_3D_ROADMAP.md](UI_GODOM_3D_ROADMAP.md).

## Goal and baseline

Migrate the current playable interface to github.com/n0remac/GoDom while preserving the game and its current visual layout. Baseline main cd55a85 includes heuristic and bounded Monte Carlo bots, lobby bot addition and host end-game controls. /game currently serves embedded HTML/CSS and handwritten JavaScript rendering JSON player snapshots; local html/websocket/database-era packages also exist. This is a real interface migration, not merely adding an unused dependency.

Read README.md, docs/D1_RELEASE_GATES.md, docs/D2_DEV_DEPLOYMENT.md, docs/F3_MULTIPLAYER.md, docs/A1_HEURISTIC_OPPONENTS.md, docs/A2_MONTE_CARLO_DESIGN.md and docs/LOBBY_AND_END_GAME.md before implementation. Inspect GoDom and OrcasMakers as library usage references.

## Work

1. Pin a reviewed GoDom version and align the Go toolchain, CI and deployment build requirements. Current reviewed GoDom is 5cfc25919c35 and declares Go 1.25; check current source before choosing the pin.
2. Audit local html, websocket and database-related packages and all consumers. Replace compatible duplicated functionality with shared imports. Delete only code proven unused/replaced. Avoid an unrelated persistence/schema migration; document retained app-specific differences.
3. Render page shell, join/lobby, seats/scores, turn status, rules, trick, private hand and results as composable GoDom Go functions. Preserve /game and existing public routes, room links, per-tab sessions, add-bot, start, play, next-hand, reconnect and confirmed end-game behavior.
4. Replace game-specific JavaScript DOM construction with server-rendered HTML updates through HTMX and its WebSocket extension. A small browser adapter for session lifecycle, command IDs, status and HTMX hooks is acceptable; do not reimplement every component in JS. Keep stable IDs and a clear future persistent-canvas boundary.
5. Preserve the authoritative room server. Do NOT replace its auth, origin checks, payload limits, private per-seat projection, command idempotency, revisions, locks, cancellation or ordered publishing with the generic GoDom hub. Adapt transport intentionally. HTMX form messages contain additional fields (including HEADERS); map and validate the expected envelope explicitly without weakening command validation.
6. Establish revision-aware update handling, rejected-command feedback and reconnect resynchronization. Prevent duplicate/stale updates and actions from producing incorrect UI. Never send opponents' private hands in HTML, attributes or hidden nodes.
7. Keep legal-action decisions in Go. Render enabled controls and explanations from authoritative legality; remove duplicate browser rule evaluation where migrated.
8. Align and pin compatible HTMX core and WS extension assets. Reviewed GoDom DefaultLayout mixes core 2.0.0 with the old 1.9.12 extension. Prefer application-owned explicit layout/assets using GoDom primitives if needed; do not make unrelated GoDom repo changes. Ensure forms, errors, out-of-band replacements and post-swap listeners work with the chosen versions.
9. Update documentation with actual migration choices, remaining local packages, validation evidence and known limits.

## Out of scope

No Three.js, physics engine, 3D redesign, new rule mechanics, AI changes, package extraction, new GoDom features or changes to other projects/services. New reusable features will be built inside Trick Evolution in later stages.

## Verification

Preserve focused domain/gameui tests and black-box complete-game coverage. Adapt protocol assertions when necessary without removing behavioral guarantees. Add meaningful coverage for Go-rendered HTML privacy, legality, rejected commands, duplicate/stale delivery and reconnect. Adapt browser selectors and tests to the real rendered interface; do not bypass UI to claim browser coverage.

Run the existing focused release gate/root build, a short browser smoke (including bot controls and confirmed ending), then one bounded complete ten-hand browser match after implementation stabilizes. Preserve human and bot complete-game fixtures. Record race/toolchain/environment limits honestly. Avoid repeated expensive full-game loops; manual phone UX checks belong to the user.

## Acceptance

A human can join, add bots, start, legally play a complete game, observe rule reveals and scores, reconnect and finish/end using the GoDom-rendered interface. Current URLs and deployment topology remain compatible. Hidden information and authority protections remain intact. No handwritten JS game-screen builder remains as the primary renderer.

## Delivery

Work on an isolated feature branch/worktree; commit implementation and documentation. Run gates before merging. The user-approved milestone ends with a behavior-preserving dev deployment via the documented D2 main-only release path at trick.noremac.dev. Merge/push only after gates pass, then verify the deployed commit and HTTP/WebSocket smoke. If blocked, stop at a concrete report rather than bypassing gates. Do not alter StarCoaster or unrelated sessions/services.

## U1 implementation (2026-11-12)

- Pinned GoDom at `5cfc25919c35` (`v0.0.0-20260221051826-5cfc25919c35`) after comparing its HTML DSL with the local copy and reviewing current GoDom and OrcasMakers usage. GoDom requires Go 1.25; the Pi has `/usr/local/go/bin/go` 1.25.2. CI reads `go.mod`, and the D2 builder uses that binary. The release scripts set writable build, module, and GOPATH caches.
- `gameui/render.go` composes the document shell, join form, lobby, seats and scores, rules, trick, private hand, legal-action explanation, and results as GoDom nodes. The old handwritten game-screen builder and embedded `index.html` were removed. The existing CSS and DOM selectors preserve the current layout. The stable `#live` element owns the WS connection and is outside the replaceable `#screen`, leaving a persistent canvas boundary for later local Trick Evolution packages.
- The app serves pinned, embedded HTMX core 2.0.11 and WS extension 2.0.4 assets. This avoids GoDom `DefaultLayout`'s mismatched core/extension pair and avoids runtime CDN availability. SHA-256: core `d6fdc75f204e6bdefa99b69bf1e6d4ac69b8a364f77929f45c13476b4000f717`; extension `896afc2936ded7a56373edf16ce75b0fe9bf49c03d1f55aee9baa94cb87c8d3e`.
- Browser game commands are HTMX `ws-send` forms. The small browser adapter retains per-tab sessions, adds random action IDs, shows connection and error status, handles end-game confirmation, and rejects duplicate or older HTML updates by a monotonic room publication sequence. The WS extension reconnects; each connection receives a new per-seat projection, including on unchanged game revisions. Each HTML form includes the expected game revision. The server rejects delayed actions if that revision has advanced.
- The authoritative room server still binds tokens to seats, enforces origin and payload limits, serializes transitions/publication, deduplicates IDs, and projects each private hand separately. `format=html` adds GoDom HTML messages to the same authenticated endpoint. Only this format accepts HTMX's `HEADERS` field and string form revision; unknown command fields remain rejected. Existing JSON clients and complete-game fixtures retain their strict protocol.
- The duplicate local `html` DSL was removed and the physical card catalog now imports shared GoDom. The local `websocket` helper remains for this app's optional browser-log endpoint; GoDom's generic hub cannot replace the room's security and publishing contract. The unused local `db` package has a different file/document/search API from GoDom's database store and is retained without a schema migration. No code was moved into GoDom or other projects.

### Verification and release

The focused release gate (`scripts/test-release.sh`) passed after migration, including human and bot ten-hand HTTP/WebSocket fixtures and root/browser builds. Focused GoDom checks cover rendered hand privacy, legal controls, HTMX envelope validation, visible rejection, stale-revision rejection, and reconnect projection. The bounded bot browser smoke passed in 2.553 seconds, including confirmed end-game and reload (`/tmp/trick-u1-browser/20261113T063336-bot-smoke-3458576/result.json`). Chromium required running the browser test outside the filesystem sandbox.

The combined `scripts/test-release.sh --browser-full --browser-bin /usr/bin/chromium --artifacts /tmp/trick-u1-browser` passed its Go and build steps, but Chromium's DevTools connection to Alice stalled during hand 2 while Bob's page was at revision 18. Earlier attempts had browser startup and page-readiness timeouts under high Pi load. The browser harness now gives Chromium more startup/DevTools time and polls autoplay at 120 ms instead of 40 ms; game assertions were unchanged. One final direct bounded full browser run **passed in 43.284 seconds** (`/tmp/trick-u1-browser/20261113T064822-full-3469032/result.json`): ten hands, revision 110, rule reveals, midgame and final reconnect, 28–22 scores, Alice winner, and no captured JavaScript exception. This supersedes the failed environmental attempts without treating them as passes. The Pi race test remains unverified because ThreadSanitizer previously reported `unsupported VMA range`.

Before main integration, check that the latest `origin/main` is an ancestor of the verified commit. The D2 release log and `state/deployed` are the records of the actual deployed SHA; verify them along with public HTTP/WSS after the main-only deployment.
