# trick-evolution

See [the revival plan](docs/REVIVAL_PLAN.md) for the current architecture, findings, and staged roadmap.

## Play locally

```sh
GOCACHE=/tmp/trick-evolution-go-cache GOMODCACHE=/tmp/trick-evolution-mod-cache /usr/local/go/bin/go run .
```

Open `http://localhost:8080/game?room=demo` in two separate browser contexts. The first player hosts; once both have joined, the host starts the game. The printable card catalog is at `http://localhost:8080/cards`. Set `TRICK_EVOLUTION_ADDR` to use a different listen address.

See [the F3 multiplayer guide](docs/F3_MULTIPLAYER.md) for reconnect policy, game controls, and focused tests.

## Release checks

Run `scripts/test-release.sh` for the focused Go tests and root build. Add `--browser-smoke` for a short real-browser check or `--browser-full` for the complete ten-hand UI gate. See [D1 release gates](docs/D1_RELEASE_GATES.md) for browser prerequisites, artifacts, CI, and verification limits.
