package cardtable

import (
	"math"
	"testing"

	"github.com/n0remac/trick-evolution/domain"
	"github.com/n0remac/trick-evolution/scene"
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
	if got := len(payload.Presentations[1].Motions); got != 2 {
		t.Fatalf("collection moves %d cards, want 2", got)
	}
}

func TestCollectionTargetsEveryWinnerSeat(t *testing.T) {
	ids := []domain.SeatID{"a", "b", "c", "d"}
	plays := []domain.Play{{Seat: "a", Card: "std:H:02"}, {Seat: "b", Card: "std:C:03"}}
	for total := 2; total <= 4; total++ {
		for winner := 0; winner < total; winner++ {
			view := domain.PlayerView{Seat: "a"}
			for _, id := range ids[:total] {
				view.Public.Seats = append(view.Public.Seats, domain.PublicSeat{ID: id})
			}
			event := domain.Event{Kind: domain.EventTrickWon, Seat: ids[winner], Plays: plays}
			payload, err := Build(view, domain.DefaultCatalog(), event)
			if err != nil {
				t.Fatal(err)
			}
			motions := payload.Presentations[0].Motions
			if len(motions) != len(plays) {
				t.Fatalf("%d seats, winner %s: %d motions", total, ids[winner], len(motions))
			}
			position := seatPosition(winner, total)
			for _, motion := range motions {
				end := motion.Steps[len(motion.Steps)-1].To.Position
				if end[0] != position[0] || end[2] != position[2] {
					t.Fatalf("%d seats, winner %s: target %v, want %v", total, ids[winner], end, position)
				}
			}
		}
	}
}

func TestSeatScoreUsesEndlessRun(t *testing.T) {
	view := domain.PlayerView{Seat: "a"}
	view.Public.Mode = domain.ModeEndless
	view.Public.Seats = []domain.PublicSeat{{ID: "a", Score: 24}, {ID: "b", Score: 19}}
	view.Public.ShortGameRound = 2
	view.Public.ShortGameScores = map[domain.SeatID]int{"a": 4, "b": 6}
	view.Public.RunScores = map[domain.SeatID]int{"a": 24, "b": 19}
	payload, err := Build(view, domain.DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if payload.Seats[0].Score != 24 || payload.Seats[0].ScoreLabel != "Run score" || payload.Seats[1].Score != 19 {
		t.Fatalf("run seat scores: %+v", payload.Seats)
	}
}

func TestHandSortAndStack(t *testing.T) {
	view := domain.PlayerView{Hand: []domain.CardID{"std:S:04", "std:H:14", "std:H:02", "std:C:09", "std:D:03"}}
	want := []domain.CardID{"std:H:02", "std:H:14", "std:D:03", "std:C:09", "std:S:04"}
	payload, err := Build(view, domain.DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	nodes := make(map[string]scene.Node)
	for _, node := range payload.Scene.Nodes {
		nodes[node.ID] = node
	}
	frontDepth := math.Inf(-1)
	for i, card := range payload.Selections {
		node := nodes[card.PickID]
		if card.Card != want[i] {
			t.Fatalf("slot %d has %s, want %s", i, card.Card, want[i])
		}
		pitch := node.Transform.Rotation[0]
		depth := func(p scene.Vec3) float64 { return p[1]*math.Cos(pitch) + p[2]*math.Sin(pitch) }
		current := depth(node.Transform.Position)
		if current-frontDepth <= node.Size[1] {
			t.Fatalf("card %s intersects its left neighbor", card.Card)
		}
		frontDepth = current
	}
	for _, card := range payload.Selections {
		lift := card.Animation.Steps[0].To
		depth := lift.Position[1]*math.Cos(lift.Rotation[0]) + lift.Position[2]*math.Sin(lift.Rotation[0])
		if depth-frontDepth <= nodes[card.PickID].Size[1] {
			t.Fatalf("lifted card %s intersects the top of the stack", card.Card)
		}
		if card.PlayTarget.Position != playPosition(0, len(payload.Seats)) {
			t.Fatalf("tap target for %s misses the local table slot", card.Card)
		}
		remaining := make([]domain.CardID, 0, len(view.Hand)-1)
		for _, id := range view.Hand {
			if id != card.Card {
				remaining = append(remaining, id)
			}
		}
		later, err := Build(domain.PlayerView{Hand: remaining}, domain.DefaultCatalog())
		if err != nil {
			t.Fatal(err)
		}
		if len(card.AfterPlay) != len(later.Selections) {
			t.Fatalf("tap on %s closes %d cards, want %d", card.Card, len(card.AfterPlay), len(later.Selections))
		}
		laterNodes := make(map[string]scene.Transform)
		for _, node := range later.Scene.Nodes {
			laterNodes[node.ID] = node.Transform
		}
		for _, target := range card.AfterPlay {
			want, ok := laterNodes[target.Target]
			if !ok || target.To != want {
				t.Fatalf("tap on %s leaves %s outside its authoritative hand pose", card.Card, target.Target)
			}
		}
	}
}
