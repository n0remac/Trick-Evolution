# Pitch and Whist expansion: phase 1

Phase 1 adds fixed partnerships and three Whist mechanics. Bidding, target-score victory, and named-game recognition remain later work. The default finite setup remains five cards, one point per trick, individual scores, no trump, and the original 18-card evolution draw. New cards can be selected in Game Rules before a finite match; selecting Partner Across also places Restore Individual Scores in that match's draw. Endless uses the expanded curated deck.

## Partnerships and score attribution

Partner Across requires exactly four seats. Seats 0 and 2 form Team 1; seats 1 and 3 form Team 2. A team's members are frozen at each deal. A guest taking over a bot seat inherits that seat position and its current match ledger, so its team does not change during the hand. A rule reveal or removal changes scoring only when the next hand is dealt. Removing Partner Across, or applying Restore Individual Scores, returns to individual scoring on that next deal.

Every award keeps one attributed seat and updates that seat's personal score and, while partnerships are active, its team score by the same number of points. Trick points and Last Trick Bonus go to the trick winner. Avoid Hearts and Avoid Queen of Hearts penalties go to the captor. High goes to the original holder, Low to the captor or original holder under Keep Low, and Jack to its captor. Game compares **the sum of both partners' captured-card tallies**, and awards one point to the winning team's first seated member; a team tie awards nothing. The first seated member also receives the Whist majority award on behalf of its team. Each hand scorecard shows both team totals and per-seat attribution.

Personal scores and Endless run scores earned before a partnership remain with their seats. Team scores begin at zero when a partnership interval starts; they accumulate only points awarded during that interval. Restoring individual scoring keeps each player's attributed points and does not distribute or copy the team ledger. If Partner Across is selected again, a new team ledger starts at zero. Historical scorecards retain the roster and teams from their hands. Endless score milestones continue to use personal attribution; the active team ledger is shown separately. Joining two Endless runs copies each runner's personal history, with no retroactive team pooling.

## Whist mechanics

- **Whist Turn-Up Trump:** after dealing, reveal the dealer's last dealt card to everyone. Its suit is trump for the hand, and the card remains in the dealer's hand until played. The revealed card is a public fact for bots and replay. It replaces No Trump, Spades Are Trump, or Pitch the Trump.
- **Whist Full Deal:** deal `floor(52 / seats)` cards to each seat. Any remainder is out of play. Four seats get 13 cards; three get 17 with one out; two get 26. It cannot combine with Expanded Hand, Keep One Card, or Six-Card Hand.
- **Whist Tricks Above Half:** ordinary points per trick stop. At a completed hand's end, a team scores `max(0, team tricks - floor(total tricks / 2))`. With individual scoring, a player scores `max(0, own tricks - floor(total tricks / seat count))`, a proportional individual threshold. A partial hand earns no majority award. Other awards and penalties still apply.
- **Six-Card Hand:** deal six cards to each player on each hand while active. It cannot combine with Full Deal, Expanded Hand, or Keep One Card.

Endless offers Partner Across independently from bidding or trump rules starting at round 6, once four seats are present and a positive scoring evolution beyond the baseline trick point is active (or Whist majority scoring is active). It is never offered with fewer seats. Restore Individual Scores can follow an active partnership. Full Deal and Six-Card Hand cannot be drafted while conflicting hand modifiers are active. Removing a controlling rule falls back to its baseline at the next deal; prior hand scorecards and ledgers remain intact.

The public view exposes only team membership, team scores, and the single turned trump card. Bot rollouts use those public facts and their own hand; the turned card is fixed in the dealer's sampled hand. No opponent's other unplayed cards are revealed.
