# Trick Evolution revival plan

## Goal and scope

Revive the multiplayer trick-taking game around its defining loop: after every 3–5 hands, a rule card enters play. Rules generally accumulate; a card can explicitly override an earlier rule. This plan treats a *hand* as one deal played through its tricks, matching the current code's “round” loop. Confirm the exact cadence, initial rules, and override wording during F1; precise conflict resolution is deliberately deferred. The first playable version should use a small, coherent rule subset, not the whole printed catalog.

## Current architecture and evidence

| Area | What exists |
| --- | --- |
| HTTP and UI | `main.go` starts one `net/http` server, registers `cards.GameUI`, and redirects `/` to `/game`. `cards/cards.go` serves join/lobby pages and server-rendered game HTML with HTMX; `cards/cards.js` sends drag-and-drop plays. `html/html.go` is a general Go HTML builder and CDN-based layout. |
| Game state | `cards/cardrules.go` defines mutable `Game`, phases, actions, hooks, and a `RuleEngine`. `cards/cards.go` holds process-global room/lobby maps and performs dealing, play, scoring, phase changes, and broadcasts inside websocket command handlers. |
| Cards and rules | `cards/trick.go` seeds a 52-card standard deck and a substantial set of printable rule/phase card descriptions in a locked in-memory store. Its `/cards` routes are defined by `Trick(mux)` but are not registered in `main.go`. `RulesList()` has one executable rule, “High Card Wins”; printed rule cards do not activate it or any other effect. |
| Transport | `websocket/websocket.go` has a global room hub, JSON command registry, client read/write pumps, and room/player-targeted HTML messages. The room and player ID are supplied in URLs and command payloads. |
| Other code | `db/` contains generic storage/search helpers and the only `_test.go` file. The running game does not import this package. |

### Verified findings from code inspection

- A new game deals five cards each from `getStandardDeck()` without shuffling; that function iterates a Go map. Later hands rebuild and shuffle with a time-based seed (`cards/cards.go`, `cards/cardrules.go`). This makes deals hard to reproduce and the first deal unintentionally dependent on map iteration.
- There is no active-rule list, rule-card draw cadence, score total, or game-over implementation. `IsGameOver()` always returns false. `CurrentTurn` and `Started` are present but unused. The only executable winner rule compares rank across all suits, while the catalog's “High Card Wins” text specifies the lead suit (`cards/cardrules.go`, `cards/trick.go`).
- The play command has no built-in turn/phase check. `ValidateAction` currently iterates zero `HookOnPlay` rules, so it accepts any action before the handler checks whether the card is in the claimed player's hand. The discard command removes a card without phase or turn validation. `ApplyEffects` results and most `TriggerHook` results are ignored (`cards/cards.go`, `cards/cardrules.go`).
- The start command can deal five more cards to an existing game on a repeated request. Its insufficient-deck early return occurs after `mu.Lock()` without unlocking, which can permanently block later handlers. Lobby reads in start/render paths occur without the mutex used for lobby writes (`cards/cards.go`).
- Command handlers trust the payload's `room` and `playerId`, use unchecked string assertions, and do not bind them to the websocket client's room/ID. The hub targets messages by these IDs, but does not authorize commands. Malformed or forged commands can panic or act on another player's hand (`cards/cards.go`, `websocket/websocket.go`).
- The only tests are `db/search_test.go`; two depend on relative `data/recipes/thewok` and `data/search.bleve` paths and print errors instead of asserting outcomes. There are no game, rule, UI, or websocket tests. A focused `go test ./cards ./websocket ./html` completed with `[no test files]`. One `go test ./...` attempt stopped during setup because the default Go module/build caches were read only; it did not provide a full-suite result.

### Suspected issues to verify with focused tests or a manual browser pass

- Sending a full HTML document as a websocket swap into the lobby element may not reliably initialize the game script or maintain the expected DOM. Verify the join → start → first play flow in a real browser before choosing the final fragment protocol.
- Global mutex protection is inconsistent, and broadcasts occur while holding the game mutex. This likely creates races and can stall unrelated rooms under slow clients; confirm with targeted concurrent tests once room ownership is refactored.
- Lobby polling, reconnects, duplicate joins, and client disconnects have no explicit lifecycle policy. Their visible behavior needs a small integration test, not assumptions from static inspection.

## Retain and replace

| Retain / adapt | Replace / isolate |
| --- | --- |
| Standard card types, ranks/suits, and catalog wording as design input; server-rendered card faces and the simple room/join flow. | Random UUIDs as rule identities; mix of printable descriptions and executable rules. Use stable rule IDs and an explicit playable subset. |
| Go HTTP/WebSocket stack and HTMX-style fragments if the browser smoke test supports them. | Game mutations, validation, and phase progression inside websocket handlers. The server should call one engine transition and render its result. |
| In-memory games for the first local playable release. | Process-global maps as the domain API; use a room/session owner with explicit locking and lifecycle. Add persistence only when a concrete requirement appears. |
| Generic `html` helpers where useful. | Implicit side effects from hooks and `map[string]interface{}` effect parameters; use typed actions/effects and explicit transition order. |

Do not migrate `db/` into the game merely because it exists. Trim or replace its unused dependencies only after the new engine's storage needs are known.

## Proposed rule and transition design

Keep one small, versioned, data-driven catalog of `RuleDefinition` records: stable `ID`, display title/text, category, trigger/timing, parameters, effect kind, and optional `overrides` IDs or group. A rule-card instance in a game references a catalog ID; it does not contain a Go closure. Validate the catalog at startup for unique IDs, known effect kinds, valid parameters, and valid override references. Custom `/cards` authoring remains a separate print/design workflow until authored cards can be validated against executable effect kinds. Do not promise that arbitrary prose is playable.

The game state should hold seats/players, hands, deck, trick plays, dealer/leader/current turn, hand and trick numbers, scores, active rule IDs in activation order, and any small rule-specific choices. Keep the standard deck separate from the rule deck. A deterministic setup accepts a seed or injected shuffle source; record chosen rule cards and choices in state so replaying the same actions yields the same result. A complete immutable event log or generic rule language is unnecessary for F1–F3.

Use a single entry point resembling `Step(state, action) -> (nextState, events, error)`. It validates membership, ownership, phase, turn, and active restrictions before changing state. It copies the state it changes, applies typed effects in a documented order, advances the trick/hand, then emits public and player-private events for rendering. The transport maps a connection to one seat and room, calls `Step` under that room's lock, and never accepts a claimed player ID as authority. Reject invalid actions without changing state. UI projection must hide opponents' hands.

The catalog can record explicit override relationships now. F1 needs only a deterministic way to activate and display rules and to mark an explicitly overridden rule inactive. F2 defines precedence for the first chosen playable subset, with tests for those exact interactions. Broader conflicts among catalog cards remain an open design decision; do not silently infer semantics from their prose.

## Staged roadmap

### F1 — foundation, domain model, and rule catalog

1. **F1a: baseline and vocabulary.** Record a short playable ruleset for one hand, define “hand” vs “trick,” minimum/maximum seats, turn order, scoring baseline, and the 3–5-hand rule-card cadence as a configuration/policy choice. Choose a small first rule subset (for example, Follow Suit and Low Card Wins) and write fixture states. **Accept:** a maintainer can describe a legal action and expected state for every phase; open conflict cases are listed, not guessed.
2. **F1b: domain and catalog.** Introduce stable card/rule IDs, typed state/actions, standard and rule decks, active-rule order, and catalog validation. Preserve existing catalog text for print display while marking supported executable entries. **Accept:** one seed produces a repeatable deal and catalog draw; unknown/duplicate rule IDs fail validation; a fixture demonstrates activation after the selected 3–5-hand interval and an explicit override.

**Tests:** table tests for card/deck uniqueness, deterministic shuffle, state setup, catalog validation, activation interval boundaries, and override bookkeeping. No server or browser needed yet.

**F1 implementation status (2026-11-12): complete on `feature/f1-gameplay-foundation`.** The provisional vocabulary, legal action/phase contract, ordering, fixture, and unresolved semantics are in [F1_GAMEPLAY_FOUNDATION.md](F1_GAMEPLAY_FOUNDATION.md). The new `domain/` package provides stable standard-card and rule IDs, typed state/actions, recorded seeded setup and replay, a validated version 1 catalog with all printed entries explicitly classified, an ordered rule field, explicit override projection, and 3–5-hand reveal bookkeeping (default 3). The `cards/` catalog, printable tooling, and live multiplayer prototype are unchanged. Focused evidence: `GOCACHE=/tmp/trick-evolution-go-cache GOMODCACHE=/tmp/trick-evolution-mod-cache /usr/local/go/bin/go test ./domain -count=1` passed. The live game does not yet use this package; F2 must implement a pure `Step`, verify hand completion before calling `RecordCompletedHand`, define a game end condition, and add typed events plus public/private views. Open rule semantics are listed in the F1 document; unsupported print cards are not executable.

### F2 — pure deterministic game engine and rule effects

1. **F2a: base hand.** Implement `Step` for start/deal, legal play, trick winner, score, next lead, and hand completion using the base rules. **Accept:** fixed action sequences complete a hand, conserve cards, award exactly one trick winner per completed trick, and reject out-of-turn/non-owned cards without mutation.
2. **F2b: accumulating effects.** Apply the F1 subset through typed validation, winner, and scoring hooks; activate a rule card at the cadence and honor its explicit override. **Accept:** the same seed and action sequence produce identical state/events; two compatible rules remain active together; the chosen override disables only its named target; supported combinations have documented outcomes.
3. **F2c: boundaries.** Add a deliberate game-end condition, stable error codes, and state-to-view projection. **Accept:** the engine can finish a configured game, expose public state and one player's private hand, and never reveal another hand.

**Tests:** concise table-driven transition tests and a few complete scripted games. Include rejection/no-mutation checks, rule interaction fixtures, and a deterministic replay check. Use race testing only for the room wrapper in F3, not for the pure engine.

**F2 implementation status (2026-11-12): complete on `feature/f2-pure-game-engine`.** [F2_ENGINE.md](F2_ENGINE.md) records the transition order, supported rule interactions, ten-hand default end condition, public/private projections, and F3 handoff. `domain.Step` handles legal play, trick and score awards, hand progression, rule reveals, next-hand deals from an explicit recorded deck order, and game completion with shared winners on a tie. Accepted actions advance a monotonic revision and emit public typed events; invalid actions return stable error codes with no state mutation. Focused evidence: `GOCACHE=/tmp/trick-evolution-go-cache GOMODCACHE=/tmp/trick-evolution-mod-cache /usr/local/go/bin/go test ./domain -count=1` passed. The existing multiplayer prototype and printable catalog remain intact. F3 must bind seats to sessions, manage rooms and command retries, supply deck orders for subsequent hands, and test the browser update protocol. Unsupported print rules remain listed in catalog version 1.

### F3 — multiplayer UI integration and hardening

1. **F3a: adapter and views.** Make HTTP/WebSocket handlers authenticate commands to a room/seat, call `Step`, and send fragments or snapshots derived from view projections. Show active rules, current turn, trick result, scores, and next-hand rule reveal. **Accept:** two browser clients can join, start, play several hands through a rule reveal, and see only their own cards.
2. **F3b: lifecycle and safety.** Specify host/start permission, player count, reconnect behavior, duplicate messages, room cleanup, payload limits, origin policy, and slow-client handling; move locking to per-room ownership. **Accept:** malformed/cross-room commands return errors without panic or state change; a disconnected client can rejoin by the chosen policy; one slow room does not block another.
3. **F3c: release check.** Remove dead code and unused dependencies made obsolete by the integration, document local run steps, and fix browser issues found in a short manual pass. **Accept:** a fresh checkout builds; focused unit/integration tests pass; one real-browser smoke path covers join → start → play → rule reveal → finish/reconnect.

**Tests:** `httptest`/websocket integration cases for command authorization and broadcast privacy, one targeted `-race` run for concurrent rooms, and a brief manual browser smoke test. Avoid repeated full-suite or long automated browser loops.

**F3 implementation status (2026-11-12): complete on `feature/f3-multiplayer-integration`.** [F3_MULTIPLAYER.md](F3_MULTIPLAYER.md) documents the authoritative room adapter, bearer-session reconnect policy, JSON snapshot protocol, host/start rules, per-room locking and revisions, command deduplication, origin and payload limits, idle cleanup, local run steps, and remaining product scope. The old game handlers and unused global WebSocket hub were removed; `/cards` and printable sheet tooling remain available. Focused `go test ./gameui ./domain ./cards ./websocket ./html -count=1` and `go build .` passed with writable caches. A two-tab headless Chromium pass completed join → start → play → first reveal → game finish → reload/reconnect without JavaScript exceptions. The targeted `-race` test could not execute on this host because ThreadSanitizer reported `unsupported VMA range`; it remains a verification task for a supported host. The unsupported catalog rule types and provisional defaults are unchanged.

## Risks, dependencies, and first slice

- **Rules are design material, not specifications.** Several catalog entries imply bids, teams, trump, hidden information, or special scoring. F1 must pick and document a narrow executable subset before F2 effects. Keep unresolved conflicts visible in the catalog instead of inventing behavior.
- **Cadence and game length interact.** A game must last enough hands to reveal at least one rule. Pick a configurable 3–5-hand interval and a game-end condition together; test the first reveal and later accumulation.
- **Transport trust and UI behavior matter early.** The current payload identity and full-document websocket update need focused integration and browser checks before the F3 adapter is considered complete.
- **Build/test environment:** Go 1.24 is declared in `go.mod`; the game packages compile, while the full suite needs writable Go caches and the `db` tests may need their relative fixtures. Keep cache paths outside the repository and do one bounded baseline run when dependencies are available.

**First recommended implementation slice:** F1a plus the smallest part of F1b: define typed card IDs/state/actions and a stable catalog for the base winner rule and one new rule, then demonstrate a seeded five-card hand fixture and activation at the chosen interval. Stop at domain tests. This gives F2 a concrete contract without changing live handlers prematurely.

## Review addendum — implementation guardrails (2026-11-12)

These refinements clarify implementation contracts without deciding unresolved game-design questions.

### Separate card definitions, instances, and resolved rules

- **Definition:** stable catalog ID, printable text, supported behavior kind and validated parameters. This is versioned game content, not mutable match state.
- **Instance:** a particular rule card introduced during a match, with its definition ID and introduction sequence; retain instances in field order even when inactive.
- **Resolved rule set:** a deterministic projection of field instances and explicit override metadata. Evaluate this projection for an action; do not permanently erase older cards as a side effect of resolving precedence. Define how overrides interact only for the initial supported subset in F2b.
- Avoid using a broad category as an implicit override rule: categories are for grouping/validation, while override relationships must be explicit.

### Make the transition boundary genuinely deterministic

- Prefer a transition that receives all externally chosen outcomes (shuffled deck, rule draw, optional player choices) as explicit inputs or already-recorded state. If a seeded generator is used, keep its state within the match or advance it only at documented boundaries; don't seed from wall-clock time inside `Step`.
- Return typed domain events, not network messages or rendered HTML. Events may be projected to per-player views by the adapter; event payloads must not accidentally include private hands in public broadcasts.
- Invalid actions must return an error with *no mutation* to the input state, including deck order, RNG state, and rule-field order. Test this invariant.
- Define deterministic ordering for validation, effect evaluation, trick award, phase advancement, and rule introduction. Document the ordering for each supported rule, not for hypothetical future cards.

### Keep F1 and F2 vertically testable

- F1a should explicitly document the chosen initial default (e.g. rule reveal every 3 completed hands) while keeping the cadence configurable for 3–5; distinguish a **trick** (one card per player) from a **hand** (one deal played through all tricks). Treat the default as a provisional implementation choice, not a newly agreed game rule.
- Start with a single baseline winner rule plus one additive rule and one override example. Do not require all catalog entries to become executable in F1.
- The first end-to-end engine fixture should play a complete hand with fixed cards, then repeat enough hands to trigger one rule reveal. Check card conservation, legal turn sequence, trick ownership, score changes, and public/private views.
- If a rule needs an interaction the engine does not yet model (bidding, team assignment, custom trump selection), mark it **unsupported**, rather than treating its printed description as executable behavior.

### F3 multiplayer contract

- Bind seat identity to the connection/session on the server; room ID and player ID in a client command are never proof of authorization.
- Give each accepted command a monotonic room revision or action ID so duplicate/retried commands cannot play a card twice. Choose a small reconnect policy and test it before expanding multiplayer features.
- Keep network and rendering operations outside the room state lock when practical; publish immutable snapshots/events after a successful transition.
- Add one targeted browser smoke test early in F3a for full-document versus fragment swapping, then choose a single update protocol. Do not rebuild the entire UI merely to adopt a framework.

### Scope boundaries and completion evidence

- Preserve the existing card catalog and printable card tooling as historical/design assets. Avoid importing unrelated `db/` or generic HTML framework code into the domain engine.
- Each milestone should leave a small documented fixture or test command and a concise list of known unsupported rule types. A completed milestone is not equivalent to having implemented every catalog card.
- Use fast, focused Go tests in the agent loop; reserve expensive browser or exhaustive interaction checks for a short manual test by the maintainer.

## Post-F2 review and product direction (2026-11-12)

### Product vision confirmed

Trick Evolution is ultimately a **trick-taking sandbox and learning game**, not just a single random-rules party game. In Evolution Mode, players start with an approachable rule such as High Card Wins and encounter randomly ordered new mechanics over time. The game should teach what each mechanic does and how combinations produce more sophisticated games. Players may eventually create or select arbitrary compatible rulesets, recreate known trick-taking games, or deliberately choose chaotic combinations. These are long-term goals, not promises that the F2 engine already supports them.

The game should recognize when the **fully resolved** active rules and relevant setup (deck, player count, hand size, bidding, partnerships, trump, scoring, win condition, etc.) **exactly match a specified variant** of an established game, and notify players without forcing the game to stop evolving. Pitch is an example target, but its variants differ: do not label a partial resemblance an exact Pitch match. Optionally show partial matches as educational hints, clearly distinguished from exact matches. Traditional games should eventually be representable as named, validated ruleset presets using the same underlying mechanics as sandbox play; avoid one hard-coded engine per named game.

For future work, keep **rule execution** separate from **game recognition** and **teaching content**. A canonical normalized ruleset/configuration projection can serve both preset comparison and save/share functionality. Rule definitions need stable semantic identities and parameters; a list of display titles is insufficient for equivalence. Game-recognition profiles must specify variant, required and excluded mechanics, and compatible setup. Rule randomization should eventually be constrained by dependencies and player comprehension, not merely an unconstrained shuffle of advanced mechanics. Preserve an optional chaotic mode.

### F2 implementation review (code and Happier transcript)

- Confirmed commit `9f8e540` on `feature/f2-pure-game-engine`. Happier reported focused tests passing for `./domain ./cards ./websocket ./html`; an independent `go test ./domain -count=1` also passed during review.
- `domain.Step` implements play and next-hand transitions, turn/ownership/phase validation, lead-suit high/low winner resolution, Follow Suit and No Follow Required, trick capture, one-point-per-trick scoring, hand completion, cadence reveals, and configurable game completion. It clones state and returns typed public events with revisions and stable error codes. `ProjectPublic` and `ProjectForSeat` separate public state from a seat's hand.
- Happier found and fixed an override-order edge case: No Follow Required can appear before Follow Suit in a shuffled rule deck. `ResolveField` treats explicit overrides independently of reveal order.
- The existing `cards/` and WebSocket prototype still mutate their own game state. The F2 engine is **not integrated into a playable browser session** until F3.

### Review cautions / follow-up tests

1. **Override semantics are provisional.** `ResolveField` applies overrides from every present definition even if the overriding instance itself is inactive. That is workable for the current no-chain subset but must be specified before adding override chains, repeated instances, or custom sandbox rules.
2. **Rule effect composition is deliberately narrow.** `activeEffects` permits only one active play restriction and one winner effect; this does not yet support the combinatorial rule sandbox. Future extension should define compatible effect families, precedence, and explicit incompatibilities rather than silently accepting combinations.
3. **Current defaults are placeholders, not intended game-design commitments:** five cards per hand, one point per trick, ten hands total, and a reveal every three hands. They make F2 testable but won't model bidding/trump/partnership games such as Pitch.
4. **Validation depth:** `Step` checks configuration/catalog/field plus action legality, but does not fully validate arbitrary corrupted match-state invariants (e.g., every card appears exactly once) on every transition. The server should own trusted state; add focused state-integrity tests where persistence/reconnect or external state loading enters the picture.
5. **Performance:** `cardByID` linearly scans a newly constructed standard deck per lookup. This is fine for the current small game but can be replaced by a stable ID lookup if larger simulations or deck variants make it material.
6. **Replay scope:** setup shuffle records and subsequent `start_hand` deck orders are recorded; full action history/command deduplication and persisted replay are not implemented. Do not conflate deterministic transitions with a durable event log.

### Recommended next sequence

- **F3:** Connect authoritative room/seat ownership to `domain.Step`; derive per-player snapshots; use revision/deduplication; test reconnect and privacy; smoke-test the browser manually. Keep the initial rule subset intact so the integration is bounded.
- **After a playable F3 vertical slice:** establish rule semantics and composability needed for sandbox presets, introduce rule dependencies and educational descriptions, then implement one named-game recognition profile with exact-versus-partial matching. Expand mechanics incrementally (e.g., trump, scoring, bidding), each with fixtures and a playable lesson. Avoid attempting all traditional trick games at once.

See [post-F3 roadmap](POST_F3_ROADMAP.md) for complete-game E2E tests, permanent dev Pi deployment, and the sandbox/learning milestones.
