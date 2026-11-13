# F3 multiplayer integration

The browser game now uses `gameui.Server`, with one authoritative `domain.MatchState` per room. The old WebSocket command registry, process-global game maps, and mutable prototype handlers were removed. The original physical card catalog remains available at `/cards` and `/cards/sheet`.

## Room and session policy

- Room codes contain 1–40 letters, digits, `_`, or `-`. A room has two to four seats. The first joined seat is host and is the only seat that can start the game or deal the next hand. New seats cannot join after game start.
- `POST /game/join` issues a random 256-bit session token and a separate public seat ID. The browser keeps the token in per-tab `sessionStorage` and sends it only at join and WebSocket connection time. Refreshing the same tab or recovering a dropped socket rejoins the same seat and receives a fresh per-seat snapshot. A second connection with the same token replaces the first. Closing the tab loses this session token by design; there is no account recovery or persisted room state yet.
- A duplicate join with a known token returns the original seat and name. A disconnected seat stays in the room; play pauses if it is that seat's turn until it reconnects. Unknown tokens cannot join a started room or take another seat.
- Rooms with no connected clients are removed after two hours of inactivity. A periodic cleanup runs every ten minutes; joins also prune stale rooms. This is in-memory storage, so process restart ends every room.

## Command and publication contract

The full page loads once at `/game?room=CODE`. Vanilla JavaScript opens `/ws/game?room=CODE&token=TOKEN` and renders one JSON snapshot protocol for lobby and game states. There is no full-document WebSocket swap. Game snapshots contain `domain.ProjectForSeat` for that connection, public seat names and connection state, and recent **public** domain events. Other players' hands and future deck order are absent.

Commands are JSON objects with a client-generated action `id` and type `start`, `play` (plus `card`), or `next`. They have no room or player-ID authority fields; extra fields are rejected. The server binds the authenticated WebSocket to one room and seat. Accepted commands advance the room revision once and return an `ack`; duplicate IDs with the same payload return the original acknowledgement without replaying the action. Reusing an ID with different content returns `action_id_conflict`. Invalid domain actions return the stable domain error code without a state change.

Each room serializes transitions and snapshot publication independently. `domain.Step` runs under the room state lock; projection, JSON encoding, and nonblocking send queues run after that lock is released. A full queue closes the slow connection so its seat can reconnect. One slow room cannot hold up another. Browser and test WebSockets are limited to 2048-byte messages; an oversize message closes the connection. Browser origins must match the request host and scheme, and a valid session token is required even when the Origin header is absent.

The first deal is created by `domain.NewMatch`; later `next` commands supply a complete shuffled deck as explicit input to `domain.Step`. The server chooses those shuffles from independent cryptographic seeds and the domain records each resulting order. Invalid actions do not draw a new deck. The default F2 rules and ten-hand game length remain provisional.

## Run and verify

From the repository root:

```sh
GOCACHE=/tmp/trick-evolution-go-cache GOMODCACHE=/tmp/trick-evolution-mod-cache /usr/local/go/bin/go run .
```

Open `http://localhost:8080/game?room=demo` in two separate browser contexts, join both seats, then start from the host. The host deals each next hand after a completed hand. Rules reveal after completed hands 3, 6, and 9. `/cards` serves the printable catalog. Set `TRICK_EVOLUTION_ADDR=127.0.0.1:18080` to choose another listen address.

Focused checks: `go test ./gameui ./domain ./cards ./websocket ./html -count=1` with writable Go caches passed, and `go build .` passed. The WebSocket tests cover session binding, cross-room and forged identity rejection, replayed action IDs, reconnect, private-hand isolation, a complete three-hand path through the first reveal, lobby capacity, idle cleanup, payload limits, and independent rooms. A headless Chromium smoke pass with two tabs completed join → start → play → reveal → game finish → reload/reconnect without browser exceptions. A targeted `go test -race ./gameui -run TestConcurrentRooms -count=1` was attempted, but this host's ThreadSanitizer rejected its address layout (`unsupported VMA range`); it did not run the test. Run that command on a supported host before treating race coverage as complete.

## Remaining scope

There is no database persistence, cross-tab account recovery, spectator mode, rematch, or advanced sandbox rule composition. The printed bidding, team, trump, hidden-information, hand-size, and special-scoring cards remain unsupported in catalog version 1. The optional browser-log socket is separate from game rooms.
