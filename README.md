# trick-evolution

The current game interface is rendered with shared GoDom and updated through HTMX WebSockets. See [U1: shared GoDom migration](docs/U1_GODOM_MIGRATION.md) and [the UI roadmap](docs/UI_GODOM_3D_ROADMAP.md). The [revival plan](docs/REVIVAL_PLAN.md) records the earlier architecture and stages.

See [Basic Play and Rule Field Stage 1](docs/BASIC_RULES_STAGE1.md) for the rule model, [Stage 2](docs/BASIC_RULES_STAGE2.md) for the Game Rules review, and [Stage 3](docs/BASIC_RULES_STAGE3.md) for the table rule field. See [Pitch scoring and pregame rules](docs/PITCH_SCORING_AND_PREGAME.md) for room settings and Pitch awards, [Scoring expansion Stage 1](docs/SCORING_EXPANSION_STAGE1.md) for three additional score cards, and [Hand evolution Stage 2](docs/HAND_EVOLUTION_STAGE2.md) for Expanded Hand and Keep One Card. [Trump and Play Restrictions with AI](docs/TRUMP_PLAY_AI.md) describes the earlier trump and bot milestone.

[Pitch and Whist phase 1](docs/PITCH_WHIST_PHASE1.md) defines opposite-seat teams, score attribution, the Whist deal, turn-up trump, majority scoring, and six-card hands.

## Play locally

```sh
GOCACHE=/tmp/trick-evolution-go-cache GOMODCACHE=/tmp/trick-evolution-mod-cache /usr/local/go/bin/go run .
```

Open `http://localhost:8080/game?room=demo` in two separate browser contexts. The first player hosts; once both have joined, the host starts the game. The printable card catalog is at `http://localhost:8080/cards`. Set `TRICK_EVOLUTION_ADDR` to use a different listen address.

**Play Endless** starts the main continuous run with one bot. It opens with three-card hands and adds Expanded Hand Size for round two. Rule drafts and automatic follow-ups use the active rule field, while every three rounds produce a score milestone without resetting the personal run score. Compatible solo runs can join at between-hand boundaries; the live table also supports invitations. The separate room-code flow remains available for finite games.

To play alone, choose one to three computer opponents when creating a room, or use **Add bot** in the lobby. In Game Rules, the host can choose hand size, reveal interval, and starting rules for this room, then reset to defaults. The host can use **End game** after starting; completed tricks and already settled hands determine the final scores. Bots use bounded [A2 Monte Carlo rollouts](docs/A2_MONTE_CARLO_DESIGN.md), with the [A1 heuristic](docs/A1_HEURISTIC_OPPONENTS.md) as fallback. See [lobby and end-game controls](docs/LOBBY_AND_END_GAME.md) for details.

See [the F3 multiplayer guide](docs/F3_MULTIPLAYER.md) for reconnect policy, game controls, and focused tests.

## Release checks

Run `scripts/test-release.sh` for the focused Go tests and root build. Add `--browser-smoke` for a short real-browser check or `--browser-full` for the complete ten-hand UI gate. See [D1 release gates](docs/D1_RELEASE_GATES.md) for browser prerequisites, artifacts, CI, and verification limits.
