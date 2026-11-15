# Hand evolution — Stage 2

Approved 2026-11-15. Implement on main and deploy through the existing main-only gate.

## Scope

Add two independent Hand-category rule cards to the evolution deck and pregame selector: **Expanded Hand** and **Keep One Card**. The default starting field and five-card deal remain unchanged. Kitty, bidding, extra discard phases, and other hand cards are excluded.

## Hand contract

Expanded Hand sets the effective hand size to the configured cards per player plus two while active at a deal. It is a modifier, so repeated hands do not add another two. A revealed card first affects the next deal. The effective size must fit the standard 52-card deck for the final seat count; reject invalid starting selections and skip an ineligible evolution reveal. Keep One Card requires an effective size of at least two so every hand has a played trick.

Keep One Card fixes the current hand to `effective size - 1` complete tricks. Each player chooses the remaining card by which cards they play. The one unplayed card becomes that player's private retained card at hand end. On the next deal it is included in that player's new hand, and one fewer fresh card is dealt to that player. A newly activated Keep One Card has no prior carryover; a card retained under a rule that is subsequently removed still crosses that one boundary, then an inactive hand plays all its cards. The final hand's leftover is unplayed, uncaptured, and unused. A new match starts with no retained cards.

Each deal records its effective size, played-trick limit, and carried/retained identities in canonical state. Deal from a recorded full-deck shuffle after filtering carried IDs, preserving the shuffled order of the other cards. Equal hand sizes and uniqueness must hold. The new hand's original ownership includes carried cards for Pitch High/Low, while Game Point and heart penalties count captured cards only. Last Trick Bonus applies to the actual final played trick. Do not score unplayed or undealt cards as captures. When a Pitch award names an unplayed retained trump, the shared score breakdown masks the card identity while retaining the award, recipient, and points.

## Visibility, UI, and AI

Public views expose the effective deal size, played-trick limit, and counts of carried/retained cards; only a seat's own view exposes its retained card identity. Hand-complete text explains that a card remains and crosses to the next deal. Game Rules and tappable compact field cards show both new effects and capacity limits. Saved starting selections are authoritative and excluded from future draws. The rollout sampler reconstructs the current full deal and original ownership using only public play history, public counts, and its own hand. It does not receive live opponent hands or hidden retained cards. Simulations use the same terminal condition through `domain.Step`, without changing production budgets.

## Verification

Focused deterministic tests cover each card alone and combined, first/new/final hands, multiple transitions and uniqueness, custom sizes and capacity, invalid starting and ineligible reveal, replacement/removal boundaries, Last Trick Bonus, Stage 1 capture penalties, Pitch ownership with carried cards, replay/version rejection, privacy and serialization, pregame room save/start/reconnect, and bots completing configured hands. Run `scripts/test-release.sh`, scoped Go vet, and JavaScript syntax checks. Browser tests remain bounded; phone layout and touch inspection is manual on this Pi.

## Implementation and release evidence

Catalog version 6 adds exactly these two supported Hand rules. Version 5 match states fail the catalog check; version 5 rule shuffle records fail replay because their deck lacks both cards. The default field, five-card deal, reveal interval, and ten-hand match remain unchanged. The host's saved settings are checked against the final room seat count at start. An ineligible Hand card is skipped during a reveal and stays in the rule draw for a later valid boundary.

The engine freezes `DealSize` and `TricksPerHand` at each deal, keeps private `Carried` and `Retained` card maps, and stores the full shuffle order for replay. Public views show counts and the frozen plan; a player's own view adds only that player's carried or retained identity. Canonical Pitch awards can refer to an unplayed retained trump, but the public score breakdown masks that card identity. The bot sampler reconstructs a synthetic carried hand from public counts and play history plus its own cards, and finishes via the authoritative engine without changing search budgets.

Focused domain and room tests, `scripts/test-release.sh`, scoped `go vet`, `node --check gameui/game.js`, and `git diff --check` passed on the Pi. Chromium browser checks were omitted because prior bounded attempts could not initialize DevTools; manual phone layout and touch checks remain for playtesting.
