# F1 gameplay foundation (provisional)

This is an implementation contract for F2, not a final interpretation of the printed rule cards. The existing multiplayer prototype and physical card catalog remain separate.

## Vocabulary and first playable rules

- **Seat:** one player position, with a stable match-local ID. Two through four unique seats sit in caller order. Clockwise means increasing seat index, wrapping to zero.
- **Trick:** each seat plays exactly one card. The first card sets the lead suit. The winner leads the next trick.
- **Hand:** one deal played through all its tricks. The default deal is five cards per seat, hence five tricks. A score of one point per won trick is the provisional baseline; scores and trick counts accumulate across hands.
- **Dealer and lead:** seat zero deals the first hand; the seat immediately clockwise leads. The next dealer rotates clockwise after a hand. F2 will implement that rotation.
- **Rule reveal:** after every three *completed* hands by default, draw one card from the rule deck. Valid configured intervals are three, four, or five completed hands. A reveal applies to the next hand, never retroactively to the hand that triggered it. If the rule deck is empty, no new rule is added.

The initial field contains **High Card Wins**. Among cards in the lead suit, the highest rank wins; ace is high. **Follow Suit** requires a player holding the lead suit to play it. **Low Card Wins** explicitly overrides High Card Wins and selects the lowest rank in the lead suit. **No Follow Required** explicitly overrides Follow Suit and allows any suit. Off-suit cards cannot win under either supported winner rule. In a standard deck there are no equal ranks within one suit, so the winner is unique.

The three draw cards are Follow Suit, Low Card Wins, and No Follow Required. Their order is shuffled and recorded. Rules accumulate in field order. An explicit override only marks an earlier named definition inactive in the resolved projection. The older instance stays in the field. The printed Score by Tricks card remains unsupported as a draw because one point per trick is already the provisional baseline.

## Phases and legal actions

| Phase | Authorized action | Expected state change in F2 |
| --- | --- | --- |
| Awaiting start | Host starts the next hand after enough seats have joined. | Deal one card at a time clockwise from left of dealer; set leader and current turn; enter playing trick. Host authority is an F3 session rule. |
| Playing trick | Current seat plays a card in that seat's hand, satisfying the active suit restriction. | Remove that card, append a public play, advance turn. On the last play, resolve the winner from active winner rule, award one trick and point, set the next lead, then either begin the next trick or complete the hand. |
| Hand complete | No player card action. | Increment completed hands, reveal at the configured boundary, then allow the next start. F1's `RecordCompletedHand` handles only the count and reveal bookkeeping; F2 must establish that the hand completed first. |
| Game complete | No action. | The end condition remains an F2c decision. |

Validation order for F2: seat membership, phase, turn, card ownership, then active play restrictions. Accepted play removes the card, records it, evaluates the active winner rule when all seats have played, awards the trick and point, advances the trick or hand, then introduces a rule at a hand boundary. A rejected action must leave state and random/deck order untouched. A full `Step` implementation, events, and private/public view projections remain F2 work.

## Deterministic F1 fixture

`domain.NewMatch` takes caller-ordered seat IDs, a validated config and catalog, and an injected shuffle source. A seeded source yields the same 52-card deck order, three-card rule deck order, and first five-card deal. Both resulting orders are stored in `MatchState.Shuffles`; `ReplayMatch` validates and reconstructs the same setup without an RNG. Seat zero is dealer, seat one leads, and the deal proceeds clockwise one card at a time. The state holds hands, draw piles, current trick, score and trick counters, hand/trick numbers, and the ordered rule field.

`RecordCompletedHand` is a narrow bookkeeping function: call it exactly once after an F2 transition verifies a hand is finished. It returns a new state and optional rule instance. `ResolveField` derives active status and `OverriddenBy` instance IDs without mutating the field. The F1 tests use fixed draw orders to show reveals at the first and second interval boundaries, Follow Suit plus the baseline coexisting, No Follow Required disabling only Follow Suit, and Low Card Wins disabling only High Card Wins.

## Open decisions for F2

- Define a game length that lasts through at least one reveal and the desired behavior after the three supported draw cards are exhausted.
- Confirm lead/dealer rotation and whether baseline scoring should remain one point per trick.
- Define precedence if future cards override an overriding card, or if duplicate definitions can enter the field. The F1 draw deck contains one instance of each rule and no override chains.
- Decide the exact behavior and tie rules for future trump, bidding, team, hidden-information, hand-size, and special-scoring cards. All such printed entries are explicitly unsupported in catalog version 1.
- Implement `Step`, typed events, invalid-action/no-mutation guarantees, and public/private projections in F2 before using this state from the multiplayer adapter.
