# F2 pure game engine

The `domain` package now runs a complete provisional match without transport or rendering. The existing `cards/` handlers still run their original prototype; F3 will replace their game mutations with `domain.Step`.

## Match and action contract

- `NewMatch` creates and deals the first hand from an injected shuffle source. Setup records both the full standard order and the rule order. `ReplayMatch` reconstructs that setup from those records.
- `Step(state, action, catalog)` is pure from the caller's perspective. It returns a copied next state, public typed events, or a `TransitionError` with a stable code. On any error it returns the original state and no events; the caller's state, decks, rule field, and shuffle records remain untouched.
- A play action carries a seat and a card ID. The server adapter must supply the seat from the authenticated connection, not from a claimed client ID. A start-hand action is accepted only from seat zero while a hand is complete. It carries a full ordered 52-card deck chosen outside `Step`; `Step` validates and records that order before dealing. It never reads wall-clock time or advances an RNG.
- Each accepted action increases `Revision` once. Every event from that action carries the new revision. F3 can use this for ordered publication, while command deduplication and reconnect policy remain F3 responsibilities.
- The default is five cards per hand, reveal after every three completed hands, and game end after ten completed hands. `MaxHands` is configurable from `RevealEvery + 1` through 100. A final-hand reveal is skipped because no later hand can use it. The highest total score wins; tied leaders share the win. There is no score target or elimination rule.

## Transition order and supported interactions

For a play, `Step` checks catalog/state validity, seat membership, phase, current turn, ownership, then active play restrictions before copying state. **Follow Suit** requires the lead suit when available. **No Follow Required** explicitly disables Follow Suit whenever both definitions are on the field, regardless of which arrived first. With neither active, any owned card may be played.

An accepted card is removed from its owner's hand and appended to the public trick; `card_played` is emitted. After one play per seat, the active winner rule compares only cards of the lead suit. **High Card Wins** is active initially; **Low Card Wins** explicitly disables it once revealed. Off-suit cards cannot win. The winner captures every trick card, gains one trick and one point, then leads the next trick. `trick_won` includes the visible plays and score. No additional scoring effects are supported.

After the last trick, `hand_completed` is emitted. The completed-hand count advances, then a rule is introduced if the cadence boundary is reached and there is a card left in the rule deck. `rule_revealed` follows `hand_completed`; the rule applies to the *next* hand. If the configured final hand has finished, `game_completed` follows instead and includes the winning seat IDs. The field retains every instance in activation order; `ResolveField` provides the active projection and explicit `OverriddenBy` IDs without deleting old instances. The supported deck has no duplicate definitions or override chains; those interactions remain undefined for future catalogs.

`start_hand` rotates the dealer clockwise, chooses the next clockwise leader, deals one card at a time from the supplied full deck, clears prior captured cards, and emits `hand_started`. Scores and cumulative trick counts remain. Cards are conserved within each hand across hands, current trick, captured piles, and standard draw pile.

## Views and events

`ProjectPublic` exposes phase, revision, turn, scores, hand counts, captured counts, visible trick plays, active rule projection, and final winners. It contains no private hand or future deck order. `ProjectForSeat` adds only that seat's hand. Events contain only public plays, results, reveals, and hand/game boundaries. An adapter should send the public projection and events to every seat and only its corresponding private hand to each connection.

## Verification and F3 handoff

Focused command: `GOCACHE=/tmp/trick-evolution-go-cache GOMODCACHE=/tmp/trick-evolution-mod-cache /usr/local/go/bin/go test ./domain -count=1`. The fixtures cover fixed full-hand scoring and card conservation, turn and ownership rejection without mutation, Follow Suit and both explicit overrides, reveals at hands 3/6/9, a full ten-hand deterministic replay, game completion, deck validation, and public/private projection.

F3 still needs per-room ownership and locking, trusted seat binding, command deduplication, reconnect policy, browser-tested fragments, and a source of shuffled full deck orders for subsequent `start_hand` actions. The printed cards involving bids, teams, trump, hidden information, hand-size changes, and special scoring remain unsupported. The default game length and score baseline are implementation choices to confirm in playtesting.
