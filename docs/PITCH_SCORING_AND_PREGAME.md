# Pitch scoring foundation and room rules

Catalog version 4 adds a composable four-point Pitch scoring foundation. This is not a complete traditional Pitch game: bidding, contracts, teams, Jokers, off-suit trumps, penalties, and victory changes are outside this milestone.

## Scoring

The default remains one point per captured trick. **No Points for Tricks** replaces that scoring rule; **Points per Trick** can later restore it. High Trump Scores, Low Trump Scores, Trump Jack Scores, and Game Point are independent awards that add to the active trick scoring rule. Hand awards settle once, after the final completed trick and before a rule reveal. A newly revealed rule first applies to the next deal. Ending a game midhand keeps points from completed tricks and previously settled hands; it does not settle the unfinished hand.

* **High Trump Scores:** one point to the original holder of the highest trump dealt into players' hands. Printed rank is ace high, independent of the trick's High or Low Card Wins rule.
* **Low Trump Scores:** one point for the lowest dealt trump. The hidden Capture Low baseline awards it to the captor. Keep Low replaces that policy with original holder ownership; a later Capture Low can replace Keep Low. Both explicit ownership cards require Low Trump Scores.
* **Trump Jack Scores:** one point to the captor of the trump Jack if it was dealt. No award if it remains in stock.
* **Game Point:** one point to the unique player with the highest captured-card tally. All suits count: ten 10, ace 4, king 3, queen 2, jack 1, others zero. A tie for highest gives no point. This tally is separate from match points and needs no trump.

The dealt cards define *in play*, including cards still held at settlement; unused stock does not. High, Low, and Jack require fixed or pitched trump. If no trump was dealt, those awards do nothing. The hand's original ownership is private during play. The public itemized breakdown appears only after completed-hand settlement, with the rule, recipient, relevant card or tally, and point. The last hand breakdown remains available at game completion. Replay uses recorded shuffle orders and the selected room config. Match records from catalog versions 1–3 require explicit migration and are rejected under version 4.

## Pregame room settings

The host can edit **Cards per player** (1–26, limited by a 52-card deck and final seat count), **Completed hands between reveals** (1–100), and initial visible rule cards in Game Rules before starting. Controls group supported cards by Hand, Play, Trick, and Score & Victory and show descriptions. The server checks prerequisite order, rejects conflicting alternatives, and validates again at authoritative start with the final seat count. The lobby broadcasts settings to every joined client and on reconnect. The in-game Rules page shows the chosen starting field read-only. Reset restores the production defaults: five cards per player, one reveal every three completed hands, ten hands, Follow Suit and High Card Wins, one point per trick, no trump. An interval longer than the match simply produces no reveal. Initially selected evolution cards are removed from the future draw; later valid replacements remain eligible.

## Verification

Focused domain tests cover individual and combined awards, printed-rank independence, ownership policies, Game ties, undealt trump, no trick points, privacy before settlement, conflicts, draw exclusion, and replay. A bounded two-client WebSocket fixture covers a complete customized match and room settings sync, authorization, and lock. Existing ten-hand fixtures check the expanded deck, replay, privacy, cadence, and bot continuation against the authoritative engine. `scripts/test-release.sh`, `go vet`, and `node --check` are the release gate. Pi Chromium has previously failed before DevTools startup; at most one bounded attempt is appropriate, with phone and visual review left to a browser-capable device.
