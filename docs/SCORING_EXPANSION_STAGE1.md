# Scoring expansion — Stage 1
Approved 2026-11-15. Implement on main and deploy after release checks.

## Goal and scope
Add three distinct composable Score & Victory rule cards to the evolution deck and pregame starting-rule selector:
- Avoid Queen of Hearts: capturing QH costs 5 match points.
- Avoid Hearts: each captured heart costs 1 match point.
- Last Trick Bonus: the winner of the final completed trick of a completed hand earns 2 additional match points.

The two heart penalties stack: QH costs 6 when both are active. Other hearts cost 1. Negative scores are allowed. Each rule is independent, has no trump prerequisite, and coexists with Pitch awards and ordinary trick points. No Points for Tricks disables only ordinary trick points, not these penalties or the last-trick bonus. None of these three replaces another.

## Semantics
Use shared typed rule resolution and authoritative engine scoring, not UI-only effects.
Apply capture penalties exactly once for completed tricks, with separate itemized reasons for QH and hearts. Award final-trick bonus exactly once when the hand completes. An early End Game retains settled completed-trick scoring but must not award a final-trick bonus for an unfinished hand.
Undealt or unplayed cards cause no capture penalties. Rule reveals affect the next hand only; never rescore earlier captures. Retain existing default setup and cadence.
Document scoring timing consistently in card details, score breakdown and replay. Preserve catalog/version compatibility policy; explicitly reject incompatible records if needed.

## UI and AI
Show all three cards with concise full effects, independent prerequisites, and additive interactions. Add to pregame configuration validation, lobby sync/reconnect, active rule field and end-hand/game scoring explanation.
Rollouts must evaluate these effects through the same engine, including negative score margins. Adapt heuristic fallback to penalty-bearing tricks and final-trick bonus; do not leak hidden opponent cards or future draws and do not increase production search budgets.

## Acceptance and verification
Focused tests: each effect alone; QH stacking; coexistence with trick points on/off and Pitch awards; negative totals; one-trick hand; final winner versus most-tricks winner; undealt QH; early end; no duplicate settlement; reveal timing; replay; pregame save/start and reconnect.
Include a bounded complete-match fixture and seeded AI decisions showing penalty avoidance/bonus valuation where strategically warranted. Run repository release gate, go vet and JavaScript syntax checks as applicable. No prolonged test loops. Pi Chromium previously cannot initialize: do not repeat expensive attempts; record manual phone/layout checks for user.
Review git diff, commit milestone and implementation on main, push, use existing deployment flow, and verify public /healthz commit equals remote main and running release. Report commit, checks, deployment and remaining manual checks accurately.

## Explicit exclusions
Do not implement Expanded Hand or Keep One Card in this stage (approved later stage). Kitty is removed entirely from this milestone and deferred until bidding. No bidding, teams, new victory rules, duplicate suit penalty variants, or unrelated changes.
