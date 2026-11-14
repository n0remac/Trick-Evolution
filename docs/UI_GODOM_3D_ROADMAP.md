# UI roadmap: GoDom adoption and a mobile 3D card table

Approved 2026-11-12. U1 through U4 are merged into main. U5 covers table polish and release.

## Direction

Keep authoritative rules and AI in Go. First migrate the current interface to the shared github.com/n0remac/GoDom library. Then build new scene, animation, physics, browser runtime, and card-table features as packages INSIDE Trick Evolution. Do not implement these new packages in GoDom yet. Extract later only after gameplay proves their API and usefulness outside this application.

Application code should use Go functions to describe 3D objects, materials, transforms, physical properties, interaction bindings, and animation sequences. Reusable JavaScript in an embedded browser runtime is expected: Go declares behavior; Three.js and a browser physics engine execute frame-by-frame rendering and simulation.

## U1 — adopt GoDom

See [U1_GODOM_MIGRATION.md](U1_GODOM_MIGRATION.md). Preserve current appearance and behavior while migrating composition and updates. No 3D implementation in this stage.

## U2 — local 3D runtime and vertical slice

Provisional local packages:
- scene: typed descriptions of scenes, cameras, groups, meshes, materials, transforms, and interaction IDs.
- animation: sequences, parallel actions, easing, durations, waits, cancellation.
- physics: bodies, colliders, forces, simulation settings.
- browser3d: embedded JavaScript runtime, pinned renderer dependencies, reconciliation and lifecycle.
- cardtable: Trick Evolution-specific cards, hand layout, rule cards and presentation.

The generic packages must not import the game domain. Use versioned validated descriptions and stable object IDs, not arbitrary generated JavaScript strings. Keep the canvas persistent across HTMX updates. Implement create/update/remove and resource cleanup. Begin with one Go-described card that can be selected on a phone and moved using a Go-described animation. Select the physics engine when building the concrete integration, rather than committing to a speculative full wrapper.

## U3 — mobile table

Build on the U2 prototype in `cardtable`, `scene`, `animation`, and `browser3d`. Use the [Letter Dice Table](https://github.com/n0remac/Cards/tree/dice/frontend/src/pages/dice) as the layout and interaction reference. Read [arenaLayout.ts](https://github.com/n0remac/Cards/blob/dice/frontend/src/pages/dice/arenaLayout.ts) for responsive perspective framing and [Die.tsx](https://github.com/n0remac/Cards/blob/dice/frontend/src/pages/dice/Die.tsx) for projecting pointer rays onto a horizontal drag plane and handling pointer cancellation. Adapt those techniques to the existing Three.js runtime.

- Give the Three.js canvas the full playable viewport with safe-area handling. Start with the camera about 45 degrees above the table. Fit the hand fan, opponent seats, and center of the table together in portrait and landscape. Keep the fanned near-edge hand tilted for readability and land played cards flat in separate seat positions. Use a coherent table surface, lighting, and shadows to show height.
- Hover lifts and highlights a card for inspection. Tap selects and lifts a card, and the existing explicit Play control remains available. Drag holds the card above the tabletop and follows the mouse or finger locally. Highlight the play area when a legal held card can be dropped. A valid release submits one play through the existing authoritative Go action. Invalid, canceled, or rejected drops return to the hand. Keep the pointer grab offset stable and recover when pointer capture is lost.
- Keep the canvas mounted across GoDom and HTMX updates. Drive visible cards, seat positions, legal actions, and animations from Go descriptions. Preserve hidden opponent hands. Show turn indicators, visible rules with inspectable overridden rules, and score/results overlays. Keep the full game playable on phones without hover or page scrolling.

Acceptance: first play one complete trick against a bot using the new table. Then complete a match. Extend the browser smoke path for a valid drag, invalid or canceled drag, tap-to-play, bot reply, resize, and reconnect. Check full-hand readability and drag feel on a phone when available.

## U4 — animation, physical cards and pacing

Start with thin rigid cards and a static table, not flexible-card simulation. Hand and selected cards are layout/animation controlled. Play sequence: lift, move toward release point, release with controlled velocity/rotation, fall/collide/settle, pause. Collect tricks using controlled animation. Keep throws within useful readable play areas.

Expose animation sequences and physical settings through Go functions. Add a browser presentation queue keyed by revision/event identity. Preserve a short visible pause AFTER a computer card lands, not merely a delay before its decision. Hold completed tricks before collection. Handle incoming updates during animation, duplicates, cancellation and interrupted sequences. Settling waits require timeouts. On reconnect restore current state without replaying old animation backlog.

Game authority and scoring never depend on physical positions. Physics is local presentation. Never send every frame or pointer movement through the server.

## U5 — polish and release

Smooth the table, inlay, and card silhouettes with rounded geometry and antialiasing. Give the hand fan enough height above the felt for the entire card face to be visible. A click or tap plays a legal card directly. Dragging to the center remains available.

### Match HUD

- Keep one Rules control at the top left and the host's End game control at the top right. Show connection warnings and rule reveals only when relevant. Put the room code, hand count, and detailed rule list inside Rules. Keep the revision in the protocol.
- Anchor player names and points to their seats. Mark whose turn it is on the nameplate. Keep the middle clear for played cards and show hand or match results there when play pauses.
- Place Deal next hand at the bottom right for the host. Keep the bottom clear during play.
- In the next layout pass, show a new rule beside the table at the hand break, with a way to reopen it under Rules. Test whether the seat highlight and the cards give players enough direction. If players miss a follow-suit restriction, show it beside the hand when it applies.

Check phone orientation and safe areas, reconnect, motion preferences, rendering resolution, idle rendering, and resource cleanup. Keep the existing engine, AI, and full-game tests. Check the full hand, corner controls, and two- through four-seat layouts on a phone, including long names and rotations during play.

## Extraction gate

Move a package to GoDom only when it has no Trick Evolution dependency, a proven documented API and lifecycle, useful tests, and a standalone non-game example. Extraction is separate follow-up work, not a prerequisite for U1–U5.

## Sequence

U1 first, then U2, U3, U4 and U5. UI work takes priority over S1/S2 sandbox expansion. Preserve D1 release gates and D2 main-only automatic deployment. Record actual validation and deployment evidence; never describe planned work as complete.
