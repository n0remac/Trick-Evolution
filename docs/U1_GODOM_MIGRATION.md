# U1 — shared GoDom migration

Status: ready for implementation. Approved 2026-11-12.
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
