# Basic Play and Rule Field: Stage 1

Stage 1 changes the game model and its existing table compatibility. It does not add the pregame Game Rules screen, a persistent Rules button, or Hand / Play / Trick / Score & Victory table zones.

## Starting match

A match starts with exactly two visible rule cards, in field order: **Follow Suit** (Play) and **High Card Wins** (Trick). The default evolution deck is empty until Set 2. A reveal boundary with no eligible card changes nothing, and the final hand never produces a reveal. The draw order is still shuffled and recorded, including an empty order, so setup and action replay remain deterministic.

The other starting rules are active, hidden field entries. They are public through `PublicView.Rules` (full ordered history) and `PublicView.Effective` (current mechanic/value/source/number). `PublicView.ActiveField` contains only active visible cards for the current table. The numeric values for hand size, fixed hand limit, and reveal interval come from `Config` through the effective projection; default values are defined only by `DefaultConfig`. Unused standard cards remain in `StandardDraw` as match state and are out of play for the hand. No private hand or future deck order enters a public projection.

| Baseline mechanic | Stage 1 effect |
| --- | --- |
| Deck, hand size, unused cards | Standard 52; configured deal size; undealt cards out of play |
| Scores and turn order | Individual scores; clockwise play |
| First and later leads | Left of rotating dealer; trick winner leads next |
| Trump and ties | No trump; earlier play wins an equal-rank tie |
| Trick points, hand limit, victory | One point per trick; configured fixed limit; highest score with shared tied victory |
| Reveal | Configured interval, between hands, only while an eligible card remains |

## Typed rules and resolution

Catalog version 2 gives supported rules a field category, typed effects, optional prerequisite IDs, explicit replacement target IDs, and starting visibility. Unsupported print catalog entries remain descriptive and cannot enter a draw. There is no executable script language. The supported compatibility fixtures **Low Card Wins** and **No Follow Required** remain outside the default evolution deck.

Field categories are Hand, Play, Trick, and Score & Victory, matching the planned Stage 3 zones. They organize presentation and never decide replacement precedence.

Three mechanics are independent: `play_restriction` decides which held cards may be played, `winner_eligibility` decides which played cards may win, and `rank_comparison` chooses a winner among eligible cards. Follow Suit supplies mandatory following and lead-suit eligibility. No Follow Required allows any held card but retains lead-suit eligibility. High Card Wins compares eligible cards from ace down to two; Low Card Wins reverses that comparison. Thus an off-suit ace cannot beat a lead-suit two without a future trump eligibility effect. Equal ranks keep the earlier play; the standard no-trump deck normally has a unique eligible rank.

`ResolveRules` is the authoritative ordered interpreter. A later instance replaces the current controller only when its definition explicitly names that controller's definition (or repeats that definition). Independent effects coexist regardless of field category. Earlier evolution instances remain in history as superseded and do not revive. Marking the latest instance removed restores the starting rule for that mechanic, never an older superseded evolution. This is resolution semantics for future removal cards; Stage 1 adds no removal action. An instance whose prerequisite is no longer active reports `prerequisite_inactive`; the relevant starting controller supplies the fallback. Hidden active baselines satisfy prerequisites.

Replacement is tracked per mechanic. A future card that replaces only Follow Suit's winner eligibility leaves its legal-card restriction active. `ResolvedRule.ActiveEffects` makes that partial status visible; `Active` is true while any effect remains. A dependent effect can reactivate if its starting prerequisite returns.

At a reveal boundary, `RecordCompletedHand` scans the remaining ordered draw for the first eligible, nonduplicate entry. Ineligible entries stay in the draw for later boundaries. If none qualifies, the boundary ends safely with no reveal. The new effect applies to the next hand only. The same resolution feeds engine validation and trick awards, public and seat views, legal card highlighting, bot observations, heuristic choices, and rollout worlds.

## Compatibility

`CatalogVersion` is 2. Version 1 match states fail the engine and projection version check, and version 1 rule shuffle records fail `ReplayMatch` against the new catalog. They are rejected explicitly instead of being interpreted under the changed meaning of High Card Wins, Follow Suit, and overrides. Current in-memory rooms use the new catalog; Stage 1 supplies no migration of old persisted records. The former `Timing`, `PlayEffect`, and `WinnerEffect` metadata remains for old code consumers but is cross-validated against typed effects; typed resolution controls gameplay.

The existing rule strip renders `ActiveField`, so hidden baselines and superseded history do not flood the table. The full public history and current effective list remain available for the Stage 2 rules UI. Stage 3 can place the same visible cards into its table zones without changing resolution.

## Validation

- Focused domain tests cover the two visible starters, all baseline values, deterministic shuffle/replay, no private deck exposure, legal following, off-suit ineligibility, high/low comparison, replacement in both directions, baseline restoration, retained history, hidden and disappearing prerequisites, skipped draws, empty deck, and final-hand behavior.
- A bounded four-hand integration fixture checks a reveal after hand three and low comparison in hand four. The existing ten-hand engine and black-box WebSocket fixtures cover scoring, dealer and turn order, ties, reconnect, replay, and private-hand isolation under the empty default evolution deck.
- Run `scripts/test-release.sh` for the focused Go packages and root/browser-runner build. Browser and touch checks remain a manual validation task after the later UI stages; no repeated full-browser loop is required on this Pi.

Local evidence on 2026-11-14: `go test ./domain -count=1` passed, `scripts/test-release.sh` passed (domain, cards, websocket, gameui, root and browser-runner builds), and `git diff --check` passed. The race gate and Chromium smoke/full loops were not run in this Stage 1 pass; those are optional release modes and the Pi has a documented ThreadSanitizer VMA limitation.

Historical F1/F2 and D1 documents describe the pre-Stage-1 evolution deck and deterministic score fixtures. This document supersedes those rule/deck statements for the current branch.
