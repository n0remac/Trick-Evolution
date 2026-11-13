# A2 design: bounded Monte Carlo rollout opponents

**Design only. A1 does not implement sampling or rollouts.** Keep the A1 heuristic as the fallback and rollout policy.

## Information boundary

The A2 decision entry point should accept an immutable bot observation, the versioned rule catalog, an injected random seed, limits, and a cancellation context. The observation contains the bot's `PlayerView` and a public per-hand play history. The room must build that history from accepted public `card_played` events, retaining lead suit and whether Follow Suit was active **at each play**; later bot decisions use the room's recorded public history. The decision entry point must not accept the live `MatchState`, other `Hands`, `StandardDraw`, `RuleDraw`, or full shuffle records. Tests should vary real hidden opponent cards while holding the observation and seed fixed and require the same choice.

For a current-hand sample, start from the 52 stable standard IDs. Remove the bot's current hand and every card publicly played in this hand, including the current trick. Assign the remaining unknown cards to opponents according to their public `HandCount`; leave the rest as undealt. Infer a void suit for a seat only when it played off-suit while Follow Suit was active, since No Follow Required permits off-suit play despite holding the lead suit. Apply each inferred void to cards still assigned to that seat. Randomize the unknown pool with a seeded generator, then use bounded randomized backtracking to satisfy counts and void constraints. Reject duplicate cards, impossible counts, or contradictory observations. After a configured number of failed attempts, fall back to A1 rather than weakening constraints or reading real hands.

Build a **synthetic** `MatchState` for each sample from public scores, dealer/leader/turn, current trick, visible active rule field, bot hand, sampled opponent hands, and a synthetic standard-deck remainder. Reconstruct captured card IDs and trick counts only from the recorded public play/winner history. No field in that state may be copied from the live hidden hands or future decks. The default A2 horizon stops at the end of the current hand. Because the final `domain.Step` may reveal a rule at hands 3/6/9, supply a synthetic `RuleDraw` sampled from supported, unrevealed catalog entries consistent with public `RulesRemaining`; the reveal cannot affect points already scored in that hand. A later match-horizon extension would also have to sample future standard/rule orders independently from public remaining catalog information; it must never reuse the live `RuleDraw` or recorded future shuffle.

## Candidate evaluation

1. Enumerate candidate cards with `domain.LegalCards(observation.PlayerView, catalog)`. If there is one card, play it without sampling.
2. Use the **same sampled hidden deal for every candidate** in a comparison round. For each candidate, apply its action with pure `domain.Step`, then finish the hand with legal A1-style policies applied to each simulated seat's own synthetic projection.
3. Score each completed rollout by the bot's hand points and by its match score margin at the hand boundary: bot cumulative score minus the highest opponent cumulative score. The default objective is expected match margin; a configuration flag may choose expected hand points. Select the largest mean, then break ties by A1 preference and stable card ID.
4. Evaluate candidates round-robin and use only complete comparison rounds, so a step-budget stop cannot give one candidate more samples. Record sample count, seed, objective, budget stop reason, and chosen card in a private diagnostic record that contains no sampled or real hidden hands.

Start with a bounded default such as 32 complete samples per legal action, at most 5,000 total `Step` calls, at most 100 ms of wall time per decision, and a fixed cap on sampling attempts per deal. Make all limits configurable with safe maxima. The sample/step budgets determine the normal result; the wall-time cap is a safety stop that returns the deterministic A1 fallback rather than a partially sampled winner. Check cancellation before each sample and every rollout step; cancel when room/game closes or the revision/turn changes. Tests use fixed seeds and step/sample budgets without relying on wall-clock timing. Production seeds should be generated independently and recorded for replay. If no full comparison round completes, or a sample cannot satisfy the observation, return the A1 choice. Never run a background search after the turn has changed.

## A2 acceptance tests

- Sampler conserves the 52-card set, public plays, own hand, opponent counts, and inferred void suits; No Follow Required never creates a false void.
- Repeating observation, catalog, seed, and fixed budgets produces identical candidate scores and choice. Changing the live hidden hands alone has no effect.
- Every simulated action passes `domain.Step`; 2- and 4-seat fixed fixtures reach the hand boundary with valid score totals and no unsupported rule behavior.
- Budget, cancellation, stale revision, impossible sample, and zero-complete-round paths terminate promptly with the legal A1 fallback.
- Black-box room tests continue to show no opponent cards or synthetic samples in human payloads or logs.
