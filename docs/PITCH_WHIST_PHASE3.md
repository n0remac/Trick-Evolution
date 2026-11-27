# Pitch and Whist expansion: phase 3

Phase 3 adds target games and known-game recognition. Catalog version 10 preserves the finite default: five cards, individual scoring, one point per trick, no trump, an 18-card evolution draw, and a ten-hand highest-score finish. The new cards are optional starting rules in Game Rules and enter the full Endless deck. Predictive Spades bidding, Rotate Trump, and advanced Pitch variants remain follow-ups.

## Target victory

First to 5 and First to 21 replace each other and Highest Score Wins. The current game's score starts at zero for each active individual or team. It includes final net hand awards, including negative penalties and Set the Bidder's replacement penalty, exactly once. Historical personal scores, the current partnership ledger, and Endless run scores are separate. Target checks happen after a **completed hand** and contract settlement, never midtrick or during an auction. If multiple sides reach the target on the same hand, the highest interval score wins; equal leaders share victory. A side below the target does not win, even if every other side is negative.

Win on Your Bid requires both an active Competitive Auction and a target rule. On the crossing hand, the auction winner's individual or frozen team must make its contract and be among the highest target-score sides. If tied at the highest score, only the qualifying bidder side wins. A failed bidder cannot qualify. A non-bidding side crossing first or holding a strictly higher score delays victory. All-pass dealer fallback is a contract and can qualify when made. Set the Bidder is independent of this gate; without it, a failed contract retains ordinary awards but still cannot satisfy Win on Your Bid. Removing an auction or target while Win on Your Bid is active is rejected until that dependent card is removed. Replacing First to 5 with First to 21 leaves the bid condition intact.

An explicit target controls finite game completion instead of the configured hand limit. The host can end the match at any time; only completed tricks count, and unfinished hand awards and contracts remain unsettled. This is the safety exit if evolving rules make the target unreachable. The 100-hand configuration validation remains for the legacy hand-limit setting, but it is **not** a hidden cap on target games. Without an explicit target, the configured finite hand limit and highest-score tie behavior stay in force.

Endless target victory produces a numbered game result and a completed-hand scorecard, then the run continues with the same players and their personal/run totals. A new zeroed game interval begins at the next deal. Changing score ownership, the active roster/team arrangement, target, or Win on Your Bid starts a fresh interval at the next deal; no points are pooled retroactively. A guest taking over a bot seat keeps that seat's game interval and team membership. Host removal and draft replacement occur only between hands. The latest game result remains available for reconnect and score review. The older three-round score milestone is suspended while a target is active, preventing competing victory notices. Its partial scores are discarded on entering a target game; after target removal it restarts as a fresh milestone at the next deal. Personal and run totals are never cleared by either interval reset. TeamScores remains the independent partnership ledger and resets when a new partnership starts or the team arrangement changes.

## Known games

Recognition uses resolved effective mechanics plus four actual seats and the frozen deal size. It never relies on the labels of active cards. It ignores only bookkeeping baselines (deck/hand-limit/reveal) and the hidden default bid metric when no auction exists; any extra play or scoring mechanic prevents a match. Entry is announced once when a new deal first matches, and the current name remains in the top bar. Leaving a match clears recognition so a later re-entry can be announced again.

| Name shown | Required effective configuration |
| --- | --- |
| Classic short Whist (without honours) | Four seats, 13 played cards each, opposite-seat teams, turn-up trump, follow suit, high card wins, only tricks above half score, first to 5, no auction or extra awards. This is the supported short-Whist ruleset; honours are absent. |
| Individual Whist-style variant | The same hand and scoring mechanics, with individual scores instead of teams. It is not labelled classic Whist. |
| Four-point partnership Pitch variant | Four seats, six played cards each, opposite-seat teams, first lead sets trump, follow suit, no ordinary trick points, High/Low/Jack/Game awards, Capture Low, point auction with legal bids 2–4, Set the Bidder, first to 21, and Win on Your Bid. |
| Four-point cutthroat Pitch variant | The same six-card mechanics with individual scores. |

Pitch is labelled a **variant** because the supported engine has no draw/discard phase or forced trump lead. Replacing point bids with trick bids, removing Set the Bidder, changing Low ownership, adding heart penalties or a last-trick bonus, or changing legal play blocks recognition. Historical scorecard labels do not affect it.

## Persistence, UI, and bots

The public view exposes interval scores, target, bid requirement, game result, and recognition. GoDom displays target progress in the mobile score control, the latest result in the scorecard and status notice, and a persistent recognized-game label. Reconnect sends those same public fields with each player's private hand. Accepted actions still use server revisions and command IDs; the browser never awards a target win locally. Version-nine snapshots use `MigratePhase2Snapshot`: it appends the three new Endless draw cards and shuffle entries in stable order without changing dealt cards, past awards, personal/team/run scores, or scorecards. Version-eight snapshots first use `MigratePhase1Snapshot`, which now advances through both migrations. Old snapshots are rejected by ordinary Step and projection until migrated.

Bot auction bids remain based on public bounds and the bot's own hand. Rollout play now compares the active target interval and prioritizes a simulated target win, including team wins; it still samples no opponent private cards. The simple bid heuristic does not predict partner holdings or calculate a probabilistic contract success rate.

## Remaining limits

Finite target matches have no automatic anti-stall hand cap; the host end action is the escape when nobody can score. A closed finite room sends a final snapshot and then rejects new joins, so reconnect applies while the room is active. Recognition covers only the four configurations above and deliberately excludes other regional Whist/Pitch scoring and auction variants. Predictive Spades bidding, Rotate Trump, draw/discard Pitch, Jokers, and off-suit trump rules require separate design and tests.
