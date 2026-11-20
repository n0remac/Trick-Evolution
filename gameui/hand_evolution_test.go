package gameui

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/n0remac/trick-evolution/domain"
)

func TestHandRulesRoomSaveStartPrivacyAndReconnect(t *testing.T) {
	s, server := testServer(t)
	host := joinRoom(t, server.URL, "hand-rules", "Host", "", http.StatusOK)
	guest := joinRoom(t, server.URL, "hand-rules", "Guest", "", http.StatusOK)
	a, b := dialRoom(t, server.URL, host), dialRoom(t, server.URL, guest)
	_ = readMessage(t, a, "lobby")
	_ = readMessage(t, b, "lobby")
	tooLarge := `{"handSize":25,"revealEvery":3,"initialRules":["rule.expanded_hand"]}`
	sendCommand(t, a, command{Type: "configure", ID: "too-large", Settings: tooLarge})
	if failure := readMessage(t, a, "error"); failure.Code != "invalid_config" || !strings.Contains(failure.Message, "effective hand size") {
		t.Fatalf("expanded capacity error was hidden: %+v", failure)
	}
	settings := `{"handSize":2,"revealEvery":3,"initialRules":["rule.expanded_hand","rule.keep_one_card"]}`
	sendCommand(t, b, command{Type: "configure", ID: "guest-hand-edit", Settings: settings})
	if failure := readMessage(t, b, "error"); failure.Code != "invalid_config" {
		t.Fatalf("guest edited hand rules: %+v", failure)
	}
	sendCommand(t, a, command{Type: "configure", ID: "save-hand", Settings: settings})
	_ = readMessage(t, a, "ack")
	for _, conn := range []*websocket.Conn{a, b} {
		lobby := readMessage(t, conn, "lobby")
		if lobby.Config == nil || lobby.Config.HandSize != 2 || !reflect.DeepEqual(lobby.Config.InitialRules, []domain.RuleID{domain.RuleExpandedHand, domain.RuleKeepOne}) {
			t.Fatalf("hand rules were not saved: %+v", lobby.Config)
		}
	}
	guest = joinRoom(t, server.URL, "hand-rules", "Guest", guest.Token, http.StatusOK)
	b = dialRoom(t, server.URL, guest)
	if lobby := readMessage(t, b, "lobby"); lobby.Config == nil || len(lobby.Config.InitialRules) != 2 {
		t.Fatalf("reconnect lost hand rules: %+v", lobby.Config)
	}
	_ = readMessage(t, a, "lobby")
	_ = readMessage(t, a, "lobby")
	sendCommand(t, a, command{Type: "start", ID: "start-hand"})
	_ = readMessage(t, a, "ack")
	states := []serverMessage{readRevision(t, a, 1), readRevision(t, b, 1)}
	if states[0].View.Public.DealSize != 4 || states[0].View.Public.TricksPerHand != 3 || len(states[0].View.Hand) != 4 || len(states[1].View.Hand) != 4 {
		t.Fatalf("saved hand rules did not initialize: %+v", states[0].View.Public)
	}
	field := renderRuleField(states[0].View.Public, domain.DefaultCatalog()).Render()
	if !strings.Contains(field, "Expanded Hand") || !strings.Contains(field, "Keep One Card") {
		t.Fatal("hand cards missing from field")
	}
	r := s.rooms["hand-rules"]
	r.mu.Lock()
	for _, id := range r.state.RuleDraw {
		if id == domain.RuleExpandedHand || id == domain.RuleKeepOne {
			r.mu.Unlock()
			t.Fatal("starting hand cards remained in evolution deck")
		}
	}
	r.mu.Unlock()
	for play := 0; play < 6; play++ {
		idx := 0
		if states[0].View.Public.Turn == guest.Seat {
			idx = 1
		}
		legal, err := domain.LegalCards(*states[idx].View, domain.DefaultCatalog())
		if err != nil || len(legal) == 0 {
			t.Fatalf("hand rule play has no legal card: %v", err)
		}
		sendCommand(t, []*websocket.Conn{a, b}[idx], command{Type: "play", ID: fmt.Sprintf("hand-%d", play), Card: legal[0]})
		ack := readMessage(t, []*websocket.Conn{a, b}[idx], "ack")
		states[0], states[1] = readRevision(t, a, ack.Revision), readRevision(t, b, ack.Revision)
	}
	hostRetained, guestRetained := states[0].View.Retained, states[1].View.Retained
	if states[0].View.Public.Phase != domain.PhaseHandComplete || hostRetained == "" || guestRetained == "" || states[0].View.Public.Seats[0].RetainedCount != 1 || states[0].View.Public.Seats[1].RetainedCount != 1 {
		t.Fatal("hand did not retain one private card per seat")
	}
	publicJSON, _ := json.Marshal(states[0].View.Public)
	guestJSON, _ := json.Marshal(states[1].View)
	if strings.Contains(string(publicJSON), string(hostRetained)) || strings.Contains(string(publicJSON), string(guestRetained)) || strings.Contains(string(guestJSON), string(hostRetained)) {
		t.Fatal("retained card leaked across seats")
	}
	html := gamePanel(states[0], host.Seat, domain.DefaultCatalog()).Render()
	if !strings.Contains(html, "Your unplayed") || !strings.Contains(html, "Next hand: 4 cards per player, 3 played tricks") {
		t.Fatal("hand-end carryover explanation missing")
	}
	sendCommand(t, a, command{Type: "next", ID: "next-hand"})
	ack := readMessage(t, a, "ack")
	states[0], states[1] = readRevision(t, a, ack.Revision), readRevision(t, b, ack.Revision)
	if states[0].View.Carried != hostRetained || states[1].View.Carried != guestRetained || len(states[0].View.Hand) != 4 || len(states[1].View.Hand) != 4 || states[0].View.Public.TricksPerHand != 3 {
		t.Fatal("next hand did not replenish retained cards")
	}
	r.mu.Lock()
	seen := map[domain.CardID]bool{}
	for _, hand := range r.state.Hands {
		for _, id := range hand {
			if seen[id] {
				r.mu.Unlock()
				t.Fatalf("duplicate next-hand card %s", id)
			}
			seen[id] = true
		}
	}
	for _, id := range r.state.StandardDraw {
		if seen[id] {
			r.mu.Unlock()
			t.Fatalf("carried card remained in stock: %s", id)
		}
		seen[id] = true
	}
	r.mu.Unlock()
	if len(seen) != 52 {
		t.Fatal("room next deal did not conserve the deck")
	}
	guest = joinRoom(t, server.URL, "hand-rules", "Guest", guest.Token, http.StatusOK)
	b = dialRoom(t, server.URL, guest)
	if restored := readMessage(t, b, "state"); restored.View == nil || restored.View.Carried != guestRetained || restored.View.Public.DealSize != 4 {
		t.Fatal("reconnect lost private carried card or hand plan")
	}
	sendCommand(t, a, command{Type: "configure", ID: "late-hand", Settings: settings})
	if failure := readMessage(t, a, "error"); failure.Code != "invalid_config" {
		t.Fatalf("midgame hand edit accepted: %+v", failure)
	}
}

func TestCarryHandRolloutCompletesWithoutHiddenOpponentState(t *testing.T) {
	catalog := domain.DefaultCatalog()
	cfg := domain.DefaultConfig()
	cfg.HandSize, cfg.MaxHands = 2, 2
	cfg.InitialRules = []domain.RuleID{domain.RuleExpandedHand, domain.RuleKeepOne, domain.RuleLastTrickBonus}
	state, err := domain.NewMatch([]domain.SeatID{"human", "bot:1"}, cfg, catalog, domain.SeededShuffle(717))
	if err != nil {
		t.Fatal(err)
	}
	for state.Phase == domain.PhasePlayingTrick {
		view, err := domain.ProjectForSeat(state, catalog, state.Turn)
		if err != nil {
			t.Fatal(err)
		}
		card, err := chooseBotCard(view, catalog)
		if err != nil {
			t.Fatal(err)
		}
		state, _, err = domain.Step(state, domain.Action{Kind: domain.ActionPlayCard, Seat: state.Turn, Card: card}, catalog)
		if err != nil {
			t.Fatal(err)
		}
	}
	order := make([]domain.CardID, 52)
	for i, card := range domain.StandardDeck() {
		order[i] = card.ID
	}
	state, _, err = domain.Step(state, domain.Action{Kind: domain.ActionStartHand, Seat: "human", DeckOrder: order}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if state.Turn != "human" {
		t.Fatal("fixture expected human to open the carried hand")
	}
	leadView, _ := domain.ProjectForSeat(state, catalog, "human")
	lead, err := domain.LegalCards(leadView, catalog)
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = domain.Step(state, domain.Action{Kind: domain.ActionPlayCard, Seat: "human", Card: lead[0]}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	view, err := domain.ProjectForSeat(state, catalog, "bot:1")
	if err != nil {
		t.Fatal(err)
	}
	leadCard, _ := domain.CardDetails(lead[0])
	observation := BotObservation{View: view, Played: []ObservedPlay{{Seat: "human", Card: lead[0], LeadSuit: leadCard.Suit, PlayRestriction: domain.ValueFollow}}}
	prepared, err := prepareObservation(observation, catalog)
	if err != nil {
		t.Fatal(err)
	}
	world, err := prepared.sampleWorld(context.Background(), rand.New(rand.NewSource(71)), 3000, catalog)
	if err != nil || world.DealSize != 4 || world.TricksPerHand != 3 || world.DealOwner[view.Carried] != "bot:1" || world.Carried["human"] == "" || world.DealOwner[world.Carried["human"]] != "human" {
		t.Fatalf("sampled carried hand is invalid: %+v %v", world, err)
	}
	for world.Phase == domain.PhasePlayingTrick {
		turnView, err := domain.ProjectForSeat(world, catalog, world.Turn)
		if err != nil {
			t.Fatal(err)
		}
		card, err := chooseBotCard(turnView, catalog)
		if err != nil {
			t.Fatal(err)
		}
		world, _, err = domain.Step(world, domain.Action{Kind: domain.ActionPlayCard, Seat: world.Turn, Card: card}, catalog)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(world.Hands["human"]) != 1 || len(world.Hands["bot:1"]) != 1 || len(world.Retained) != 2 {
		t.Fatal("sampled rollout played the retained last card")
	}
	budget := RolloutBudget{SamplesPerCard: 1, MaxSteps: 100, MaxSampleNodes: 3000, MaxWallTime: time.Second, Objective: ObjectiveMatchMargin}
	first, err := decideRollout(context.Background(), observation, catalog, 101, budget)
	if err != nil || first.CompletedSamples != 1 {
		t.Fatalf("carried-hand rollout did not complete: %+v %v", first, err)
	}
	state.Hands["human"][0], state.StandardDraw[0] = state.StandardDraw[0], state.Hands["human"][0]
	otherView, err := domain.ProjectForSeat(state, catalog, "bot:1")
	if err != nil || !reflect.DeepEqual(view, otherView) {
		t.Fatal("hidden opponent change altered observation")
	}
	second, err := decideRollout(context.Background(), BotObservation{View: otherView, Played: observation.Played}, catalog, 101, budget)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("hidden cards changed rollout: %+v / %+v %v", first, second, err)
	}
}

func TestHandRuleDetailsAndHiddenPitchAwardRendering(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.InitialRules = []domain.RuleID{domain.RuleExpandedHand, domain.RuleKeepOne}
	state, catalog := reviewFixture(t, cfg)
	view := fieldView(t, state, catalog)
	field := renderRuleField(view.Public, catalog).Render()
	for _, text := range []string{"Expanded Hand", "Keep One Card", "does not accumulate", "one unplayed card"} {
		if !strings.Contains(field, text) {
			t.Fatalf("tappable Hand card detail omitted %q", text)
		}
	}
	selector := reviewHTML(t, reviewMessage(nil), "host", catalog, domain.DefaultConfig())
	for _, id := range []domain.RuleID{domain.RuleExpandedHand, domain.RuleKeepOne} {
		if !strings.Contains(selector, `data-initial-rule="`+string(id)+`"`) {
			t.Fatalf("pregame selector omitted %s", id)
		}
	}
	view.Public.Phase = domain.PhaseHandComplete
	view.Public.DealSize, view.Public.TricksPerHand = 7, 6
	view.Public.HandAwards = []domain.HandAward{{Rule: domain.RuleHighTrump, Title: "High Trump Scores", Seat: "guest", HiddenCard: true, Points: 1}}
	view.Public.LastScorecard = &domain.Scorecard{HandNumber: view.Public.HandNumber, Players: []domain.ScorecardPlayer{{Seat: "host"}, {Seat: "guest", Total: 1, Awards: view.Public.HandAwards}}}
	view.Retained = "std:C:02"
	message := reviewMessage(&view)
	html := gamePanel(message, "host", catalog).Render()
	if !strings.Contains(html, "retained card hidden") || !strings.Contains(html, "Your unplayed 2♣ stays in your next hand") || !strings.Contains(html, "Next hand: 7 cards per player, 6 played tricks") {
		t.Fatal("private carryover or redacted award was not explained")
	}
}

func TestExpandedHandRevalidatesFinalRoomSeats(t *testing.T) {
	_, server := testServer(t)
	host := joinRoom(t, server.URL, "expanded-seat-limit", "Host", "", http.StatusOK)
	_ = joinRoom(t, server.URL, "expanded-seat-limit", "Guest", "", http.StatusOK)
	conn := dialRoom(t, server.URL, host)
	_ = readMessage(t, conn, "lobby")
	settings := `{"handSize":17,"revealEvery":3,"initialRules":["rule.expanded_hand"]}`
	sendCommand(t, conn, command{Type: "configure", ID: "save-expanded-limit", Settings: settings})
	_ = readMessage(t, conn, "ack")
	_ = readMessage(t, conn, "lobby")
	_ = joinRoom(t, server.URL, "expanded-seat-limit", "Third", "", http.StatusOK)
	_ = readMessage(t, conn, "lobby")
	sendCommand(t, conn, command{Type: "start", ID: "invalid-expanded-start"})
	if failure := readMessage(t, conn, "error"); failure.Code != "invalid_action" || !strings.Contains(failure.Message, "effective hand size") {
		t.Fatalf("final seats did not revalidate expanded deal: %+v", failure)
	}
}
