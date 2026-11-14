# U3/U4 dice-inspired card table

Implemented on `feature/u3-dice-inspired-table` after the U2 GoDom and Three.js slice. The [UI roadmap](UI_GODOM_3D_ROADMAP.md) is the scope and acceptance reference. The Letter Dice Table's `arenaLayout.ts` informed aspect-aware camera distance; `Die.tsx` informed horizontal ray-plane dragging, pointer capture, and cancellation recovery.

## Presentation and authority

`cardtable.Build` turns one private `PlayerView` into a versioned scene: felt surface and play inlay, a tilted near-edge hand fan, opponent card backs from public counts, and flat played cards in seat-specific positions. No opponent card identity is included. It supplies Go-computed legal selections and Go-described landing and collection motions. GoDom renders the visible status, score, and inspectable active/overridden rules. The canvas persists outside `#screen` swaps, fills the safe viewport, and fits portrait and landscape through camera distance changes.

The browser projects mouse or touch rays to a horizontal plane and keeps the initial grab offset. Hover lifts, tap selects, and the explicit **Play selected card** action remains. A legal center release clicks the existing authenticated HTMX form. Invalid and canceled drags return to the hand; accepted releases also restore the local hand card while the authoritative state arrives, so a rejected command cannot leave it stranded. Pointer movement and animation frames stay in the browser.

A browser presentation queue consumes public card and trick events by identity. A card lifts, falls, bounces, and settles before a bot pause. A completed trick holds before collection. Updates received during animation queue behind the active sequence, with bounded motion waits. Duplicate event IDs do not replay; reconnect clears the queue and restores the latest snapshot. The Go engine remains authoritative for card legality, turn, scoring, and trick completion.

The fall and bounce use bounded Go-described animation steps. This milestone does not add a general rigid-body physics engine.

## Verification

- Focused `cardtable`, `scene`, `animation`, and `gameui` tests passed, including public-seat projection, private-hand selection, Go-authored motions, and completed-trick hold scene.
- The existing black-box bot completion test is part of the passing `gameui` test run and covers full bot matches.
- A bounded ten-hand two-player Chromium match passed in 55.137 seconds, including scores, rule reveals, midgame reconnect, and final result restoration. Report: `/tmp/trick-u3-browser/20261114T013003-full-46/result.json`.
- Final focused 3D Chromium smoke passed in 24.631 seconds. It checked the portrait canvas and five-card framing, hidden confirmation, tap-to-play, invalid and legal drag, a completed trick with a bot, observed bot landing pause and trick collection hold, resize, and reconnect. Report: `/tmp/trick-u3-browser/20261114T015848-3d-smoke-72/result.json`. Earlier runs intermittently stalled in Chromium DevTools on this loaded Pi; the host had load around 15 and full swap usage. Stalls are not recorded as gameplay passes.
- A failure screenshot at `/tmp/trick-u3-browser/20261114T015422-3d-smoke-60/alice.png` exposed side clipping in the five-card fan after the table was enlarged. The fan was tightened, corner labels were added, and the final smoke checks all five projected centers fit in the portrait canvas. The same screenshot confirmed the table fills the viewport and the hidden End game confirmation no longer overlays play.
- Manual phone checks remain: full-hand readability, portrait and landscape HUD overlap, touch drag feel and cancellation, and perceived landing/collection pacing on a physical phone. No phone check was performed in this session.

No merge or deployment is part of this branch.
