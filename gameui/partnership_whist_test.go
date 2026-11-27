package gameui

import (
	"bytes"
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/n0remac/trick-evolution/cardtable"
	"github.com/n0remac/trick-evolution/domain"
)

func TestFourSeatWhistPartnershipRoomAndReconnect(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.InitialRules = []domain.RuleID{domain.RulePartnership, domain.RuleWhistDeal, domain.RuleWhistTrump}
	s, err := NewServerWithConfig(cfg, func() domain.ShuffleSource { return identityShuffle{} })
	if err != nil {
		t.Fatal(err)
	}
	web := httptest.NewServer(s.Handler())
	t.Cleanup(web.Close)
	t.Cleanup(s.Close)
	players := []testSession{}
	for _, name := range []string{"North", "East", "South", "West"} {
		players = append(players, joinRoom(t, web.URL, "whist-teams", name, "", http.StatusOK))
	}
	connections := make([]*websocket.Conn, 4)
	for i, player := range players {
		connections[i] = dialRoom(t, web.URL, player)
		_ = readMessage(t, connections[i], "lobby")
	}
	sendCommand(t, connections[0], map[string]any{"type": "start", "id": "start-teams"})
	_ = readMessage(t, connections[0], "ack")
	var first domain.PublicView
	for i, conn := range connections {
		message := readMessage(t, conn, "state")
		if message.View == nil {
			t.Fatal("missing private projection")
		}
		public := message.View.Public
		if len(public.HandTeams) != 4 || public.TeamScores[0] != 0 || public.TeamScores[1] != 0 || public.DealSize != 13 || public.TurnedTrumpCard == "" || len(message.View.Hand) != 13 {
			t.Fatalf("seat %d got incomplete partnership Whist view: %+v", i, public)
		}
		if i == 0 {
			first = public
		} else if !reflect.DeepEqual(first.HandTeams, public.HandTeams) || first.TurnedTrumpCard != public.TurnedTrumpCard {
			t.Fatal("views disagree on public team or turn-up")
		}
		raw, _ := json.Marshal(public)
		for j := range connections {
			if j == i {
				continue
			}
			r := s.rooms["whist-teams"]
			r.mu.Lock()
			hand := append([]domain.CardID(nil), r.state.Hands[players[j].Seat]...)
			r.mu.Unlock()
			for _, card := range hand {
				if card != public.TurnedTrumpCard && bytes.Contains(raw, []byte(`"`+string(card)+`"`)) {
					t.Fatalf("opponent card leaked: %s", card)
				}
			}
		}
	}
	_ = connections[2].Close()
	reconnected := dialRoom(t, web.URL, players[2])
	message := readMessage(t, reconnected, "state")
	if message.View == nil || !reflect.DeepEqual(message.View.Public.HandTeams, first.HandTeams) || message.View.Public.TurnedTrumpCard != first.TurnedTrumpCard {
		t.Fatal("reconnect lost frozen public facts")
	}
}

func TestBotPreservesPartnerWinningTrick(t *testing.T) {
	view := botView(t, []domain.CardID{"std:H:02", "std:H:14"}, []domain.Play{{Seat: "human", Card: "std:H:10"}}, false, true, false)
	view.Public.HandTeams = map[domain.SeatID]int{"human": 0, "bot:1": 0, "other": 1, "fourth": 1}
	card, err := chooseBotCard(view, domain.DefaultCatalog())
	if err != nil || card != "std:H:02" {
		t.Fatalf("bot overtook partner with %s: %v", card, err)
	}
}

func TestTeamIndicatorsAndScorecard(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.InitialRules = []domain.RuleID{domain.RulePartnership, domain.RuleWhistTrump}
	catalog := domain.DefaultCatalog()
	state, err := domain.NewMatch([]domain.SeatID{"a", "b", "c", "d"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	view, err := domain.ProjectForSeat(state, catalog, "a")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := cardtable.Build(view, catalog)
	if err != nil {
		t.Fatal(err)
	}
	partners := 0
	for _, seat := range payload.Seats {
		if seat.Partner {
			partners++
			if seat.Team != 1 || seat.ID != "c" {
				t.Fatalf("wrong partner indicator: %+v", seat)
			}
		}
	}
	if partners != 1 {
		t.Fatalf("found %d partners", partners)
	}
	scorecard := &domain.Scorecard{HandNumber: 1, Teams: []domain.ScorecardTeam{{Team: 0, Seats: []domain.SeatID{"a", "c"}, Total: 3, GameTally: 26}, {Team: 1, Seats: []domain.SeatID{"b", "d"}, Total: 1, GameTally: 20}}}
	html := renderScorecard(scorecard, func(id domain.SeatID) string { return string(id) }, "scorecard-visible").Render()
	if !strings.Contains(html, "Team 1") || !strings.Contains(html, "Game tally 26") || !strings.Contains(html, "a &amp; c") {
		t.Fatalf("team scorecard missing: %s", html)
	}
	message := serverMessage{Type: "state", Room: "test", View: &view, Seats: []seatSummary{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}, {ID: "c", Name: "C"}, {ID: "d", Name: "D"}}, Sequence: 1}
	screen := renderScreen(message, "a", catalog, cfg)
	if !strings.Contains(screen, "Turned trump:") || !strings.Contains(screen, "Team 1") {
		t.Fatal("team or public trump absent from screen")
	}
}

func TestRolloutSamplesPublicTurnUpInDealerHand(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.InitialRules = []domain.RuleID{domain.RuleWhistDeal, domain.RuleWhistTrump, domain.RulePartnership}
	catalog := domain.DefaultCatalog()
	state, err := domain.NewMatch([]domain.SeatID{"a", "b", "c", "d"}, cfg, catalog, domain.SeededShuffle(12))
	if err != nil {
		t.Fatal(err)
	}
	view, err := domain.ProjectForSeat(state, catalog, "b")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := prepareObservation(BotObservation{View: view}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	world, err := prepared.sampleWorld(context.Background(), rand.New(rand.NewSource(24)), 5000, catalog)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range world.Hands[state.Dealer] {
		found = found || id == state.TurnedTrumpCard
	}
	if !found || world.DealOwner[state.TurnedTrumpCard] != state.Dealer || world.TeamScores[0] != 0 || len(world.HandTeams) != 4 {
		t.Fatalf("sample lost public dealer card or teams: %+v", world)
	}
}
