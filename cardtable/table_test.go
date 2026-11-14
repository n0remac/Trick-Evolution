package cardtable

import (
	"testing"

	"github.com/n0remac/trick-evolution/domain"
)

func TestTableSceneAndCompletedTrickPresentation(t *testing.T) {
	catalog := domain.DefaultCatalog()
	state, err := domain.NewMatch([]domain.SeatID{"alice", "bot"}, domain.DefaultConfig(), catalog, domain.SeededShuffle(7))
	if err != nil {
		t.Fatal(err)
	}
	play := func() []domain.Event {
		view, err := domain.ProjectForSeat(state, catalog, state.Turn)
		if err != nil {
			t.Fatal(err)
		}
		legal, err := domain.LegalCards(view, catalog)
		if err != nil {
			t.Fatal(err)
		}
		next, events, err := domain.Step(state, domain.Action{Kind: domain.ActionPlayCard, Seat: state.Turn, Card: legal[0]}, catalog)
		if err != nil {
			t.Fatal(err)
		}
		state = next
		return events
	}
	events := play()
	view, err := domain.ProjectForSeat(state, catalog, "alice")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := Build(view, catalog, events...)
	if err != nil {
		t.Fatal(err)
	}
	if err := payload.Scene.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(payload.Selections) != len(view.Hand) || len(payload.Presentations) != 1 || payload.Presentations[0].Kind != domain.EventCardPlayed {
		t.Fatalf("unexpected table projection: %+v", payload)
	}
	own := map[domain.CardID]bool{}
	for _, id := range view.Hand {
		own[id] = true
	}
	for _, s := range payload.Selections {
		if !own[s.Card] {
			t.Fatalf("non-owned selection %s", s.Card)
		}
	}
	foundSeat, foundPlayed, foundBack := false, false, false
	for _, n := range payload.Scene.Nodes {
		if n.ID == "seat_1" {
			foundSeat = true
		}
		if n.ID == cardID(events[0].Card) {
			foundPlayed = true
		}
		if n.ID == "back_1_0" {
			foundBack = true
		}
		if len(n.ID) >= 5 && n.ID[:5] == "hand_" && (n.Transform.Position[0] < -1.25 || n.Transform.Position[0] > 1.25) {
			t.Fatalf("hand card falls outside the portrait fan: %s at %v", n.ID, n.Transform.Position)
		}
	}
	if !foundSeat || !foundPlayed || !foundBack {
		t.Fatalf("missing seat, played card or private back: %v %v %v", foundSeat, foundPlayed, foundBack)
	}
	events = play()
	view, err = domain.ProjectForSeat(state, catalog, "alice")
	if err != nil {
		t.Fatal(err)
	}
	payload, err = Build(view, catalog, events...)
	if err != nil {
		t.Fatal(err)
	}
	if payload.PresentationScene == nil || len(payload.Presentations) != 2 {
		t.Fatalf("completed trick lacks hold scene/events")
	}
	if err := payload.PresentationScene.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(payload.PresentationScene.Nodes) != len(payload.Scene.Nodes)+2 {
		t.Fatalf("completed cards were not retained for collection")
	}
}
