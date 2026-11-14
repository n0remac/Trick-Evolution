# Basic Play and Rule Field: Stage 2

Stage 2 adds a reusable Game Rules review to the existing GoDom game page. It builds on the [Stage 1 rule model](BASIC_RULES_STAGE1.md). It does not add the Stage 3 table zones or new gameplay cards.

## Player flow

A player joining a lobby for the first time on the page sees Game Rules automatically. **Starting rules** shows the two visible field cards, Follow Suit and High Card Wins, plus hidden active baselines grouped under Hand, Play, Trick, and Score & Victory. The explanations include the server's actual configured deal size, hand limit, and reveal interval from the projected typed effects. The evolution explanation includes between-hand activation, exhaustion, and no reveal after the final hand.

**Return to lobby** closes the review without sending a game command. The host's lobby **Start game** button opens the same review with a primary **Start game** action. That action uses the existing authenticated WebSocket `start` command and action ID; the server still checks host identity and seat count. Guests see the host status and a return action. No ready vote or waiting state is added. If the host starts while a guest reads, the live content changes to the current game rules and **Back to game** replaces the lobby action.

A visible **Rules** button remains in the top controls in the lobby, during tricks, between hands, and after completion. In a match, the review defaults to **Current rules**. **Starting rules** always shows the original setup. **Changes** lists introduced rules and replaced starting entries with their sources and statuses. Superseded and prerequisite-inactive rules are excluded from Current rules; an unchanged match shows “No rule changes yet.” Future rule deck order and opponent hands are never rendered in the review.

## Implementation

`domain.StartingField` is shared by match setup and `StartingRules`, so lobby preview and the first deal use the same catalog instances. `rulesReviewContent` receives the server's `Config` for lobby preview or the seat's public rule projection for a started game. It renders an HTMX out-of-band GoDom fragment inside a persistent native `<dialog>`. The dialog shell and 3D canvas stay mounted while `#screen` and rules content swap. Only review state, tab selection, scroll position, and focus are local JavaScript state. Opening or closing rules does not send gameplay commands, reset the 3D scene, interrupt the presentation queue, or pause bots.

The modal uses native focus containment and Escape handling, labelled tabs with arrow/Home/End navigation, a persistent close control, and focus restoration to the opener. The table is inert to pointer input while the dialog is open, and the runtime's selection callbacks also reject input. Incoming projections continue to update the screen and review while open; after reconnect the existing presentation reset treats the snapshot as restoration rather than a fresh reveal.

## Validation

- Focused Go tests compare lobby preview with actual match setup under a nondefault config, verify the two visible starters and hidden baseline explanations, host-only start controls, current versus starting versus changes content, hidden baseline replacement, inactive dependency messaging, private-data absence, and completed-game reconnect projection.
- `scripts/test-release.sh` passed the focused Go packages, ten-hand HTTP/WebSocket fixtures, and root/browser-runner builds. `node --check gameui/game.js` and `git diff --check` also passed.
- A bounded, single-page portrait `rules-smoke` Chromium mode was added for automatic review, host review then start, keyboard tabs/focus, reopening, a scored play after close, and completed-game reconnect. It is exposed as `scripts/test-release.sh --browser-rules-smoke` or `go run ./cmd/browser-e2e -mode rules-smoke`. Local attempts did not complete: Chromium DevTools stalled or timed out during browser context creation and after the host start on this Pi. Its assertions therefore remain unverified by a passing browser run. The host/guest transition is covered by focused live WebSocket tests.
- Repeated full browser loops, physical phone/touch testing, and assistive-technology testing were not performed. Desktop/mobile viewport and keyboard behavior still need a completed browser run and manual check.

## Stage 3 handoff

`FieldCategory` already maps to Hand, Play, Trick, and Score & Victory. Stage 3 can place the current field cards and rule review affordance in the new table zones without changing rule resolution or the review's public data contract. Keep the persistent dialog shell outside replaceable game fragments so table animation and local selection continue across live updates.
