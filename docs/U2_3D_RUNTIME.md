# U2 — local 3D runtime and card-play slice

Status: implemented, verified, and deployed on main. U1 baseline was `2d10ae869ac98363caef7b33b97bfee0ea57c1b1`. U2 follows [the UI roadmap](UI_GODOM_3D_ROADMAP.md), [D1](D1_RELEASE_GATES.md), and [D2](D2_DEV_DEPLOYMENT.md).

## Scope and design

- `scene` defines a version 1 description with camera, background, ordered parent groups, box meshes, material color and optional face text, transforms, and stable pick IDs. Validation bounds counts, identifiers, finite numbers, dimensions, colors, and camera FOV.
- `animation` defines version 1 target steps with transforms, durations, and three named easings. Validation caps step count and total time. The browser cancels a running sequence when a new scene arrives or the connection closes.
- `browser3d` embeds a reusable Three.js module and the small browser reconciler. It creates, updates, and removes objects by ID; disposes geometry, materials, textures and renderer resources; responds to canvas resize; caps device pixel ratio at 2 and backing dimensions at 960×600; and renders on change or during animation. There is no dynamic JavaScript generation or rule logic.
- `cardtable` is the only new package importing the game domain. It turns a private `PlayerView` into visible own-hand card boxes and per-card pick bindings. Each binding carries a Go-defined lift, move, return sequence and the result of Go's `LegalCards`. Opponents' private cards never enter the scene. The local 3D play button submits the matching existing HTMX form, retaining server token, fresh command ID, expected revision, and authoritative legal-action check.
- The canvas and accessible play control sit outside HTMX-swapped `#screen`. The scene arrives as base64-encoded JSON on the GoDom-authored screen; the browser adapter accepts only newer publication sequences. On a new state it cancels local motion and reconciles to the latest snapshot. Reload/reconnect constructs the current scene without an animation backlog. Existing HTML hand forms remain playable.

This milestone does not implement physics, falling rigid bodies, a complete mobile redesign, or a general animation timeline. Those remain U3/U4 work.

## Three.js dependency

Pinned `three@0.160.1`, locally embedded as `browser3d/vendor/three-0.160.1.module.js`. Downloaded from `https://cdn.jsdelivr.net/npm/three@0.160.1/build/three.module.js`; upstream package source is `https://github.com/mrdoob/three.js/tree/r160`. License is MIT, recorded at `browser3d/vendor/three-0.160.1.LICENSE`. SHA-256 of the module: `76dea8151bc9352aef3528b4262e249b2604f62543828328db978d060d61a495`; license: `852e0e8699169bf9f6fdc6bda3e682d078dcbc738b5d33e74df594721bff271d`.

## Manual phone route and controls

Open `https://trick.noremac.dev/game?room=YOUR_ROOM` on a phone. Join as host, choose one computer opponent (or add a bot in the lobby), and start. Touch a visible card in the 3D canvas to select it and see the short local lift/move/return motion. Press **Play selected card** below the canvas to submit the legal card. The HTML hand below remains available for normal play, including accessibility and full-game flow. Rotate or resize to check the canvas; reload to check current-state restoration. Manual feel and readability judgment belongs to phone playtesting.

## Verification and release

Focused schema and per-seat HTML tests validate versioning, rejected invalid inputs, own-hand only scene content, Go-authored motion, and legal controls. The bounded `3d-smoke` Chromium mode uses a 390×844 emulated phone viewport, pointer input at a projected real mesh, checks measured local motion and the authenticated play against a bot, verifies renderer identity/object count after HTMX updates, resizes to 320×700 and checks the backing cap, then reloads and checks current-state restoration. It runs under `xvfb-run` on this Pi because bare headless Chromium stalled during DevTools startup/GPU initialization.

### Pre-release Pi evidence (2026-11-12/13 UTC)

- Focused `scene`, `animation`, and `gameui` tests passed. `scripts/test-release.sh --browser-full` passed its focused `domain`, `cards`, `websocket`, `gameui` tests, including ten-hand HTTP/WebSocket fixtures, and root/browser builds. `git diff --check` passed.
- Final 3D browser smoke **passed** in 29.929s with actual rendered pointer selection, card movement, accepted bot play, update reconciliation, resize, and reconnect. Report: `/tmp/trick-u2-browser/20261113T074608-3d-smoke-3512935/result.json`.
- The final combined D1 full-game browser gate **failed** at the hand 1 boundary because Chromium DevTools stopped answering Alice's state evaluations and did not answer a fresh DevTools connection's `Runtime.enable`; Bob was connected at revision 1. Report: `/tmp/trick-u2-browser/20261113T074320-full-3510929/result.json`. An earlier run reached hand 2 and its failure capture showed both pages at the completed hand 2 boundary with no captured JavaScript exceptions; another run stalled during browser startup. The host had sustained load around 9–13 and full swap usage. These observations suggest an environment/Chromium stall, but the complete browser gate has **not passed**, so no claim of a successful full match is made.
- The Pi race gate is unverified for the previously documented ThreadSanitizer VMA limitation. No manual phone feel test was performed. U2 was **not merged, pushed to main, or deployed**; D2 main-only rollout and public HTTP/WSS verification remain pending a passing full gate. No unrelated services were changed.
- The existing deployment record and local `/healthz` still report U1 `2d10ae869ac98363caef7b33b97bfee0ea57c1b1`. A read-only public HTTPS `/healthz` request from this Pi timed out after eight seconds while DNS resolved to `165.227.55.254`; public WSS was not exercised because U2 was not deployed. This public reachability result is a separate current-environment limitation, not U2 deployment evidence.

### Ubuntu CI gate (2026-11-13)

[PR #1](https://github.com/n0remac/Trick-Evolution/pull/1) ran the full D1 gate on Ubuntu in [workflow run 43391461124](https://github.com/n0remac/Trick-Evolution/actions/runs/43391461124). The lightweight job passed. The full-browser job passed the focused Go and root build gate, the supported-host targeted race test, the exact ten-hand full-browser match in **14.906s**, and the 3D pointer/motion/bot-play/resize/reconnect smoke in **1.628s**. This resolves the release gate previously blocked by the Pi's Chromium DevTools stalls. Main integration and D2 deployment have not yet occurred at this evidence point.

The documentation follow-up head `8724b4ff3f924c6e69a0be6e31585e6623d7281b` passed the same lightweight and full-browser jobs in [workflow run 43392031845](https://github.com/n0remac/Trick-Evolution/actions/runs/43392031845) before merge.

### Main integration and D2 rollout (2026-11-13)

After confirming the latest `origin/main` was still the U1 baseline and an ancestor of the tested PR head, PR #1 was merged as `292b7a8e4db938ae3ba9e28d5b74b34bd7b733dd`. The installed D2 `--main` script fetched that exact main commit, passed its archived focused D1 gate and root build, installed the read-only release, and restarted the service. The first startup probe briefly saw connection refused; the documented retry passed local HTTP and two-client WebSocket start/reconnect. The deploy log recorded `deployed 292b7a8…` at `2026-11-13T00:22:28-08:00`.

The deployment record, local `/healthz`, public HTTPS `/healthz`, and `origin/main` all reported the full merge SHA. The release's `deploy-smoke` passed on `https://trick.noremac.dev` with exact-commit HTTP health and two-client WSS start/reconnect. The Trick service, tunnel, and deploy timer were active. No unrelated service was changed. Manual phone feel remains for the user to judge.
