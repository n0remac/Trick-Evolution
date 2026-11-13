# trick-evolution

See [the revival plan](docs/REVIVAL_PLAN.md) for the current architecture, findings, and staged roadmap.

## Play locally

```sh
GOCACHE=/tmp/trick-evolution-go-cache GOMODCACHE=/tmp/trick-evolution-mod-cache /usr/local/go/bin/go run .
```

Open `http://localhost:8080/game?room=demo` in two separate browser contexts. The first player hosts; once both have joined, the host starts the game. The printable card catalog is at `http://localhost:8080/cards`. Set `TRICK_EVOLUTION_ADDR` to use a different listen address.

See [the F3 multiplayer guide](docs/F3_MULTIPLAYER.md) for reconnect policy, game controls, and focused tests.
