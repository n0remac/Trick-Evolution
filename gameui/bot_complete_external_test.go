package gameui_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/n0remac/trick-evolution/domain"
	"github.com/n0remac/trick-evolution/gameui"
)

type botSeatSummary struct {
	ID                domain.SeatID `json:"id"`
	Bot, Host, Online bool
}

type botWireMessage struct {
	Type     string
	ID       string
	Code     string
	Revision uint64
	Seats    []botSeatSummary
	View     *domain.PlayerView
	Events   []domain.Event
	Raw      []byte `json:"-"`
}

func TestBotCompleteGameBlackBox(t *testing.T) {
	for _, bots := range []int{1, 3} {
		t.Run(fmt.Sprintf("human_plus_%d_bots", bots), func(t *testing.T) {
			var deals atomic.Int64
			catalog := domain.DefaultCatalog()
			server, err := gameui.NewServerWithConfig(domain.DefaultConfig(), func() domain.ShuffleSource {
				return domain.SeededShuffle(5100 + deals.Add(1))
			})
			if err != nil {
				t.Fatal(err)
			}
			web := httptest.NewServer(server.Handler())
			t.Cleanup(web.Close)
			t.Cleanup(server.Close)
			join := func(token string, botCount int, status int) matchSession {
				body, _ := json.Marshal(map[string]any{"room": "botmatch", "name": "Human", "token": token, "bots": botCount})
				response, err := http.Post(web.URL+"/game/join", "application/json", bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				if response.StatusCode != status {
					t.Fatalf("join HTTP %d, want %d", response.StatusCode, status)
				}
				if status != http.StatusOK {
					return matchSession{}
				}
				var session matchSession
				if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
					t.Fatal(err)
				}
				if session.Token == "" || session.Seat == "" {
					t.Fatal("human has no session")
				}
				return session
			}
			host := join("", bots, http.StatusOK)
			join("bot:1", 0, http.StatusUnauthorized)
			u, _ := url.Parse(web.URL)
			address := "ws://" + u.Host + "/ws/game?room=botmatch&token=" + url.QueryEscape("bot:1")
			_, response, err := websocket.DefaultDialer.Dial(address, nil)
			if err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
				t.Fatalf("bot ID used as websocket token: %v (%v)", err, response)
			}
			dial := func() *websocket.Conn {
				address := "ws://" + u.Host + "/ws/game?room=botmatch&token=" + url.QueryEscape(host.Token)
				conn, response, err := websocket.DefaultDialer.Dial(address, http.Header{"Origin": []string{web.URL}})
				if err != nil {
					t.Fatalf("dial: %v (%v)", err, response)
				}
				return conn
			}
			conn := dial()
			defer func() { _ = conn.Close() }()
			read := func() botWireMessage {
				_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
				_, raw, err := conn.ReadMessage()
				if err != nil {
					t.Fatalf("read bot match: %v", err)
				}
				var message botWireMessage
				if err := json.Unmarshal(raw, &message); err != nil {
					t.Fatal(err)
				}
				message.Raw = raw
				return message
			}
			lobby := read()
			if lobby.Type != "lobby" || len(lobby.Seats) != bots+1 || !lobby.Seats[0].Host || lobby.Seats[0].Bot || bytes.Contains(lobby.Raw, []byte(host.Token)) {
				t.Fatalf("wrong lobby or token leak: %+v", lobby)
			}
			ids := make([]domain.SeatID, len(lobby.Seats))
			for i, seat := range lobby.Seats {
				ids[i] = seat.ID
				if i > 0 && (!seat.Bot || !seat.Online || seat.ID != domain.SeatID(fmt.Sprintf("bot:%d", i))) {
					t.Fatalf("bad bot seat: %+v", seat)
				}
			}
			serial := 0
			send := func(kind string, card domain.CardID) {
				serial++
				command := map[string]any{"type": kind, "id": fmt.Sprintf("human-%d", serial)}
				if card != "" {
					command["card"] = card
				}
				if err := conn.WriteJSON(command); err != nil {
					t.Fatal(err)
				}
			}
			send("start", "")
			var expected domain.MatchState
			var revision uint64
			var revealHands []int
			lastCommandAt := ^uint64(0)
			reconnected := false
			deadline := time.Now().Add(35 * time.Second)
			for time.Now().Before(deadline) {
				message := read()
				if message.Type != "state" {
					continue
				}
				if message.View == nil {
					t.Fatal("state omitted view")
				}
				if message.Revision == revision+1 {
					var wantEvents []domain.Event
					if revision == 0 {
						expected, err = domain.NewMatch(ids, domain.DefaultConfig(), catalog, domain.SeededShuffle(5101))
						if err != nil {
							t.Fatal(err)
						}
						expected.Revision = 1
						wantEvents = []domain.Event{{Kind: domain.EventHandStarted, Revision: 1, Hand: 1, Seat: expected.Leader}}
					} else if len(message.Events) > 0 && message.Events[0].Kind == domain.EventHandStarted {
						deck := domain.StandardDeck()
						domain.SeededShuffle(int64(5100+expected.HandNumber+1)).Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
						order := make([]domain.CardID, len(deck))
						for i, card := range deck {
							order[i] = card.ID
						}
						expected, wantEvents, err = domain.Step(expected, domain.Action{Kind: domain.ActionStartHand, Seat: host.Seat, DeckOrder: order}, catalog)
						if err != nil {
							t.Fatal(err)
						}
					} else if len(message.Events) > 0 && message.Events[0].Kind == domain.EventCardPlayed {
						play := message.Events[0]
						expected, wantEvents, err = domain.Step(expected, domain.Action{Kind: domain.ActionPlayCard, Seat: play.Seat, Card: play.Card}, catalog)
						if err != nil {
							t.Fatal(err)
						}
					} else {
						t.Fatalf("revision %d has no recognized action: %+v", message.Revision, message.Events)
					}
					if !reflect.DeepEqual(message.Events, wantEvents) {
						t.Fatalf("revision %d events differ: got %+v, want %+v", message.Revision, message.Events, wantEvents)
					}
					revision = message.Revision
					for _, event := range message.Events {
						if event.Kind == domain.EventRuleRevealed {
							revealHands = append(revealHands, event.Hand)
						}
					}
				} else if message.Revision != revision {
					t.Fatalf("revision jumped %d -> %d", revision, message.Revision)
				}
				wantView, err := domain.ProjectForSeat(expected, catalog, host.Seat)
				if err != nil || !reflect.DeepEqual(*message.View, wantView) {
					t.Fatalf("revision %d projection mismatch: %v", revision, err)
				}
				for _, seat := range expected.Seats[1:] {
					for _, card := range expected.Hands[seat.ID] {
						if bytes.Contains(message.Raw, []byte(`"`+string(card)+`"`)) {
							t.Fatalf("hidden bot card %s leaked at revision %d", card, revision)
						}
					}
				}
				if expected.Phase == domain.PhaseGameComplete {
					scores := make([]int, len(expected.Seats))
					for i, seat := range expected.Seats {
						scores[i] = seat.Score
					}
					wantScores := map[int][]int{1: {17, 33}, 3: {5, 22, 12, 11}}[bots]
					if !reflect.DeepEqual(scores, wantScores) {
						t.Fatalf("final scores %v, want %v", scores, wantScores)
					}
					if expected.CompletedHands != 10 || revision != uint64(10*5*(bots+1)+10) {
						t.Fatalf("incomplete match: hand=%d revision=%d", expected.CompletedHands, revision)
					}
					if !reflect.DeepEqual(revealHands, []int{3, 6, 9}) {
						t.Fatalf("reveal hands %v", revealHands)
					}
					total := 0
					best := -1
					for _, seat := range expected.Seats {
						total += seat.Score
						if seat.Score > best {
							best = seat.Score
						}
					}
					var winners []domain.SeatID
					for _, seat := range expected.Seats {
						if seat.Score == best {
							winners = append(winners, seat.ID)
						}
					}
					if total != 50 || !reflect.DeepEqual(message.View.Public.Winners, winners) {
						t.Fatalf("bad final score/winners: %+v", message.View.Public)
					}
					_ = conn.Close()
					join(host.Token, 0, http.StatusOK)
					conn = dial()
					final := read()
					if final.Type != "state" || final.Revision != revision || !reflect.DeepEqual(*final.View, wantView) {
						t.Fatal("completed match reconnect changed state")
					}
					send("play", "std:H:02")
					for {
						rejected := read()
						if rejected.Type == "error" {
							if rejected.Revision != revision {
								t.Fatal("post-game action advanced revision")
							}
							return
						}
					}
				}
				if expected.Phase == domain.PhaseHandComplete && expected.CompletedHands == 4 && !reconnected {
					_ = conn.Close()
					join(host.Token, 0, http.StatusOK)
					conn = dial()
					snapshot := read()
					if snapshot.Type != "state" || snapshot.Revision != revision || !reflect.DeepEqual(*snapshot.View, wantView) {
						t.Fatal("midgame reconnect changed state")
					}
					reconnected = true
				}
				if revision == lastCommandAt {
					continue
				}
				if expected.Phase == domain.PhaseHandComplete {
					send("next", "")
					lastCommandAt = revision
				} else if expected.Turn == host.Seat {
					legal, err := domain.LegalCards(*message.View, catalog)
					if err != nil || len(legal) == 0 {
						t.Fatalf("human legal cards: %v", err)
					}
					send("play", legal[0])
					lastCommandAt = revision
				}
			}
			t.Fatal("bot match exceeded 35-second bound")
		})
	}
}
