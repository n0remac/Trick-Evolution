# UI roadmap: GoDom adoption and a mobile 3D card table

Approved 2026-11-12. Status: planned; U1 is the next implementation.

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

Whole-viewport field with dynamic viewport and safe-area handling; large angled bottom fan; tap-to-lift/inspect and explicit play action; compact opponent positions; readable turn indicators; played cards placed by seat; rules as cards on the field with expanded details; overridden rules covered/inactive but inspectable. Lobby, scores, settings and results use overlays. Do not require hover or page scrolling for normal gameplay. Supply legal-action information from Go rather than duplicating rules in the browser.

Acceptance: complete a match on a phone using the new table, initially with simple movement.

## U4 — animation, physical cards and pacing

Start with thin rigid cards and a static table, not flexible-card simulation. Hand and selected cards are layout/animation controlled. Play sequence: lift, move toward release point, release with controlled velocity/rotation, fall/collide/settle, pause. Collect tricks using controlled animation. Keep throws within useful readable play areas.

Expose animation sequences and physical settings through Go functions. Add a browser presentation queue keyed by revision/event identity. Preserve a short visible pause AFTER a computer card lands, not merely a delay before its decision. Hold completed tricks before collection. Handle incoming updates during animation, duplicates, cancellation and interrupted sequences. Settling waits require timeouts. On reconnect restore current state without replaying old animation backlog.

Game authority and scoring never depend on physical positions. Physics is local presentation. Never send every frame or pointer movement through the server.

## U5 — polish and release

Phone orientation/resize, reconnect, reduced motion, keyboard/accessibility play path, rendering resolution cap, idle rendering control, GPU/physics resource cleanup, and deployment. Preserve focused engine/AI and full-game tests; add transport, sequence and lifecycle tests where meaningful. Run a bounded full-browser match for release. Use manual phone playtesting for readability, overlap, physics feel and pacing; avoid long repeated automated loops.

## Extraction gate

Move a package to GoDom only when it has no Trick Evolution dependency, a proven documented API and lifecycle, useful tests, and a standalone non-game example. Extraction is separate follow-up work, not a prerequisite for U1–U5.

## Sequence

U1 first, then U2, U3, U4 and U5. UI work takes priority over S1/S2 sandbox expansion. Preserve D1 release gates and D2 main-only automatic deployment. Record actual validation and deployment evidence; never describe planned work as complete.
