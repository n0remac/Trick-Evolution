package gameui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/n0remac/trick-evolution/domain"
)

func simulateBots(t *testing.T, seats int) ([]int, []int) {
	t.Helper()
	catalog := domain.DefaultCatalog()
	ids := make([]domain.SeatID, seats)
	for i := range ids {
		ids[i] = domain.SeatID(fmt.Sprintf("bot:%d", i+1))
	}
	state, err := domain.NewMatch(ids, domain.DefaultConfig(), catalog, domain.SeededShuffle(5101))
	if err != nil {
		t.Fatal(err)
	}
	state.Revision = 1
	var reveals []int
	for state.Phase != domain.PhaseGameComplete {
		var action domain.Action
		if state.Phase == domain.PhaseHandComplete {
			deck := domain.StandardDeck()
			domain.SeededShuffle(int64(5100+state.HandNumber+1)).Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
			order := make([]domain.CardID, len(deck))
			for i, card := range deck {
				order[i] = card.ID
			}
			action = domain.Action{Kind: domain.ActionStartHand, Seat: ids[0], DeckOrder: order}
		} else {
			view, err := domain.ProjectForSeat(state, catalog, state.Turn)
			if err != nil {
				t.Fatal(err)
			}
			card, err := chooseBotCard(view, catalog)
			if err != nil {
				t.Fatal(err)
			}
			action = domain.Action{Kind: domain.ActionPlayCard, Seat: state.Turn, Card: card}
		}
		var events []domain.Event
		state, events, err = domain.Step(state, action, catalog)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.Kind == domain.EventRuleRevealed {
				reveals = append(reveals, event.Hand)
			}
		}
	}
	scores := make([]int, seats)
	for i, seat := range state.Seats {
		scores[i] = seat.Score
	}
	return scores, reveals
}

func TestBotVsBotDeterministicMatch(t *testing.T) {
	for _, seats := range []int{2, 4} {
		scores, reveals := simulateBots(t, seats)
		again, againReveals := simulateBots(t, seats)
		want := map[int][]int{2: {25, 25}, 4: {10, 18, 10, 12}}[seats]
		if !reflect.DeepEqual(scores, want) || !reflect.DeepEqual(scores, again) || !reflect.DeepEqual(reveals, []int{3, 6, 9}) || !reflect.DeepEqual(reveals, againReveals) {
			t.Fatalf("%d bots were nondeterministic: %v/%v reveals %v/%v", seats, scores, again, reveals, againReveals)
		}
	}
}

func joinWithBots(t *testing.T, base, room, name, token string, bots, status int) testSession {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"room": room, "name": name, "token": token, "bots": bots})
	response, err := http.Post(base+"/game/join", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		t.Fatalf("join status %d, want %d", response.StatusCode, status)
	}
	if status != http.StatusOK {
		return testSession{}
	}
	var session testSession
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	return session
}

func TestBotSeatsAreTokenlessAndRoomCleanupCancelsMove(t *testing.T) {
	s, web := testServer(t)
	s.botDelay = 350 * time.Millisecond
	joinWithBots(t, web.URL, "bots", "Host", "", -1, http.StatusBadRequest)
	host := joinWithBots(t, web.URL, "bots", "Host", "", 1, http.StatusOK)
	r := s.rooms["bots"]
	r.mu.Lock()
	if len(r.seats) != 2 || !r.seats[1].Bot || r.seats[1].Token != "" || len(r.clients) != 0 {
		t.Fatal("bot has a token or fake connection")
	}
	r.mu.Unlock()
	guest := joinRoom(t, web.URL, "bots", "Guest", "", http.StatusOK)
	joinWithBots(t, web.URL, "bots", "Guest", guest.Token, 1, http.StatusForbidden)
	joinWithBots(t, web.URL, "bots", "Host", host.Token, 1, http.StatusOK)
	joinWithBots(t, web.URL, "bots", "Host", host.Token, 1, http.StatusConflict)
	// A second room keeps exactly one human WebSocket; its pending bot move
	// must be canceled when the disconnected room is pruned.
	host = joinWithBots(t, web.URL, "prune", "Host", "", 1, http.StatusOK)
	r = s.rooms["prune"]
	conn := dialRoom(t, web.URL, host)
	_ = readMessage(t, conn, "lobby")
	sendCommand(t, conn, command{Type: "start", ID: "start"})
	_ = readRevision(t, conn, 1)
	_ = conn.Close()
	deadline := time.Now().Add(time.Second)
	for {
		r.mu.Lock()
		clients := len(r.clients)
		r.mu.Unlock()
		if clients == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("human connection did not detach")
		}
		time.Sleep(5 * time.Millisecond)
	}
	r.mu.Lock()
	r.lastActivity = time.Now().Add(-roomIdleTTL - time.Minute)
	r.mu.Unlock()
	s.mu.Lock()
	s.pruneLocked()
	_, exists := s.rooms["prune"]
	s.mu.Unlock()
	if exists {
		t.Fatal("idle bot room was not pruned")
	}
	select {
	case <-r.done:
	default:
		t.Fatal("prune did not cancel worker")
	}
	time.Sleep(400 * time.Millisecond)
	r.mu.Lock()
	revision := r.state.Revision
	r.mu.Unlock()
	if revision != 1 {
		t.Fatalf("bot played after cleanup: revision %d", revision)
	}
}

func TestServerCloseCancelsPendingBotMove(t *testing.T) {
	s, web := testServer(t)
	s.botDelay = 250 * time.Millisecond
	host := joinWithBots(t, web.URL, "closing", "Host", "", 1, http.StatusOK)
	r := s.rooms["closing"]
	conn := dialRoom(t, web.URL, host)
	_ = readMessage(t, conn, "lobby")
	sendCommand(t, conn, command{Type: "start", ID: "start"})
	_ = readRevision(t, conn, 1)
	s.Close()
	time.Sleep(300 * time.Millisecond)
	r.mu.Lock()
	revision := r.state.Revision
	r.mu.Unlock()
	if revision != 1 {
		t.Fatalf("bot played after server close: revision %d", revision)
	}
}

func botView(t *testing.T, hand []domain.CardID, trick []domain.Play, low, follow, anySuit bool) domain.PlayerView {
	t.Helper()
	field := []domain.RuleInstance{{InstanceID: "high", DefinitionID: domain.RuleHighCardWins, Sequence: 0}}
	if low {
		field = append(field, domain.RuleInstance{InstanceID: "low", DefinitionID: domain.RuleLowCardWins, Sequence: len(field)})
	}
	if follow {
		field = append(field, domain.RuleInstance{InstanceID: "follow", DefinitionID: domain.RuleFollowSuit, Sequence: len(field)})
	}
	if anySuit {
		field = append(field, domain.RuleInstance{InstanceID: "any", DefinitionID: domain.RuleNoFollowRequired, Sequence: len(field)})
	}
	rules, err := domain.ResolveField(field, domain.DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	return domain.PlayerView{Seat: "bot:1", Hand: hand, Public: domain.PublicView{Phase: domain.PhasePlayingTrick, Turn: "bot:1", CurrentTrick: trick, Rules: rules}}
}

func TestBotHeuristicHighLowAndPlayOverrides(t *testing.T) {
	catalog := domain.DefaultCatalog()
	tests := []struct {
		name string
		view domain.PlayerView
		want domain.CardID
	}{
		{"high lead prefers strong long suit", botView(t, []domain.CardID{"std:H:14", "std:D:14", "std:D:13"}, nil, false, false, false), "std:D:14"},
		{"high wins cheaply", botView(t, []domain.CardID{"std:H:03", "std:H:10", "std:H:14"}, []domain.Play{{Seat: "human", Card: "std:H:09"}}, false, true, false), "std:H:10"},
		{"follow sheds weakest lead", botView(t, []domain.CardID{"std:H:03", "std:H:08", "std:C:02"}, []domain.Play{{Seat: "human", Card: "std:H:09"}}, false, true, false), "std:H:03"},
		{"no follow sheds off suit", botView(t, []domain.CardID{"std:H:03", "std:H:08", "std:C:02"}, []domain.Play{{Seat: "human", Card: "std:H:09"}}, false, true, true), "std:C:02"},
		{"low wins cheaply", botView(t, []domain.CardID{"std:H:02", "std:H:08", "std:H:12"}, []domain.Play{{Seat: "human", Card: "std:H:09"}}, true, true, false), "std:H:08"},
		{"low lead chooses low rank", botView(t, []domain.CardID{"std:H:02", "std:S:13"}, nil, true, false, false), "std:H:02"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := chooseBotCard(test.view, catalog)
			if err != nil || got != test.want {
				t.Fatalf("chose %s, want %s: %v", got, test.want, err)
			}
		})
	}
}
