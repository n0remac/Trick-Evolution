# A1 heuristic opponents

A human can create a room with one to three computer opponents using the join form, or the host can add bots in the lobby before starting. The join API accepts `bots` from 0 to 3. Only the creator or a returning host can add them; the four-seat limit still applies. Bots are displayed in the lobby and scoreboard, but have no session token or WebSocket client. A bot ID cannot authenticate a WebSocket or join request. The human host starts the match and deals each next hand after the completed-hand pause.

## Decision boundary and behavior

The room worker copies `domain.PlayerView` for the current bot and passes **only that view and the rule catalog** to the pure `chooseBotCard` function. The view contains public turn, trick, scores, active rules, and the bot's own hand. The chooser never receives `MatchState`, other hands, the undealt deck, or unrevealed rule order. `domain.LegalCards` uses the same play-restriction helper as `domain.Step`; it honors the active Follow Suit rule and No Follow Required override.

The heuristic is intentionally small and deterministic:

1. On lead, play the strongest card under the active High/Low winner rule; prefer a longer suit when ranks tie.
2. When following, use the cheapest legal card that currently beats the visible lead-suit winner.
3. If none can win, shed the weakest legal card. Under No Follow Required this may be off-suit.
4. Break all remaining ties by stable card ID. No RNG is used for choices.

One worker runs per started bot room. It waits 60 ms before each bot move, outside room locks, then rechecks room closure, turn, and revision under the existing dispatch/state locks before applying `domain.Step`. A new human action wakes it; consecutive bot turns continue in the same worker. Completing the game ends the worker. Pruning an idle room or closing the server cancels pending waits. Rooms and match state remain in memory, with the same restart loss as F3/D2.

## Verification and limits

Focused tests cover legal cards with Follow/No Follow overrides; High/Low and discard choices; tokenless bots, host-only addition, capacity, room cleanup and server-close cancellation. Black-box HTTP/WebSocket fixtures with one human plus one or three bots drive all ten hands and assert every revision, full projected public/private state against deterministic deals, no hidden bot card in human payloads, reveals at hands 3/6/9, exact final scores/winners, midgame and completed-game reconnect, and post-game rejection. Pure two- and four-bot ten-hand fixtures assert deterministic outcomes. `bot-smoke` in the Chromium runner creates a bot room through the actual UI and completes a scored trick with one browser context.

Local evidence on 2026-11-12: `scripts/test-release.sh` passed, including the original D1 2-/4-human fixtures and new 1-human+1-bot and 1-human+3-bot fixtures (about 13.6 seconds for `gameui`). Exact seeded final scores are 17–33 and 5–22–12–11; pure 2-/4-bot scores are 25–25 and 10–18–10–12. `go run ./cmd/browser-e2e -mode bot-smoke -browser /usr/bin/chromium` passed in 1.067 seconds, and the existing `-mode smoke` passed in 1.243 seconds. `scripts/test-release.sh --browser-bot-smoke` is the opt-in combined gate. Browser artifacts for this run are under `/tmp/trick-a1-browser-artifacts`. The full ten-hand browser suite and Pi race test were not rerun; the focused Go fixtures cover ten-hand bot behavior, and D1 recorded the Pi ThreadSanitizer VMA limitation.

This is a rules-aware heuristic, not an inference or search opponent. It uses only the currently visible trick and its own hand, so it may commit a card that later opponents beat. It does not estimate unseen cards, remember void suits, bid, choose a difficulty level, persist rooms, or deal the next hand for an absent host. The A2 search design is in [A2_MONTE_CARLO_DESIGN.md](A2_MONTE_CARLO_DESIGN.md); no rollout search is implemented in A1.
