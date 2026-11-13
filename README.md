# trick-evolution

The current game interface is rendered with shared GoDom and updated through HTMX WebSockets. See [U1: shared GoDom migration](docs/U1_GODOM_MIGRATION.md) and [the UI roadmap](docs/UI_GODOM_3D_ROADMAP.md). The [revival plan](docs/REVIVAL_PLAN.md) records the earlier architecture and stages.

## Play locally

```sh
GOCACHE=/tmp/trick-evolution-go-cache GOMODCACHE=/tmp/trick-evolution-mod-cache /usr/local/go/bin/go run .
```

Open `http://localhost:8080/game?room=demo` in two separate browser contexts. The first player hosts; once both have joined, the host starts the game. The printable card catalog is at `http://localhost:8080/cards`. Set `TRICK_EVOLUTION_ADDR` to use a different listen address.

To play alone, choose one to three computer opponents when creating a room, or use **Add bot** in the lobby. The host can use **End game** after starting; completed tricks determine the final scores. Bots use bounded [A2 Monte Carlo rollouts](docs/A2_MONTE_CARLO_DESIGN.md), with the [A1 heuristic](docs/A1_HEURISTIC_OPPONENTS.md) as fallback. See [lobby and end-game controls](docs/LOBBY_AND_END_GAME.md) for details.

See [the F3 multiplayer guide](docs/F3_MULTIPLAYER.md) for reconnect policy, game controls, and focused tests.

## Release checks

Run `scripts/test-release.sh` for the focused Go tests and root build. Add `--browser-smoke` for a short real-browser check or `--browser-full` for the complete ten-hand UI gate. See [D1 release gates](docs/D1_RELEASE_GATES.md) for browser prerequisites, artifacts, CI, and verification limits.
