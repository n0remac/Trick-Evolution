package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func expansionDeal(t *testing.T, cfg Config, first ...CardID) MatchState {
	t.Helper()
	catalog := DefaultCatalog()
	seed, err := NewMatch([]SeatID{"a", "b"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	order := append([]string(nil), seed.Shuffles[0].Order...)
	for i, id := range first {
		found := -1
		for j := i; j < len(order); j++ {
			if order[j] == string(id) {
				found = j
				break
			}
		}
		if found < 0 {
			t.Fatalf("card %s is repeated", id)
		}
		order[i], order[found] = order[found], order[i]
	}
	records := []ShuffleRecord{{Kind: StandardDeckKind, Order: order}, seed.Shuffles[1]}
	state, err := ReplayMatch([]SeatID{"a", "b"}, cfg, catalog, records)
	if err != nil {
		t.Fatal(err)
	}
	return state
}
func expansionPlay(t *testing.T, state MatchState, seat SeatID, card CardID) MatchState {
	t.Helper()
	next, _, err := Step(state, Action{Kind: ActionPlayCard, Seat: seat, Card: card}, DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	return next
}
func scoreOf(state MatchState, id SeatID) int {
	for _, seat := range state.Seats {
		if seat.ID == id {
			return seat.Score
		}
	}
	return 0
}

func TestCapturePenaltiesAloneAndStacked(t *testing.T) {
	for _, tc := range []struct {
		name   string
		rules  []RuleID
		want   int
		awards int
	}{
		{"queen", []RuleID{RuleAvoidQueenHearts}, -4, 2},
		{"hearts", []RuleID{RuleAvoidHearts}, -1, 3},
		{"stacked", []RuleID{RuleAvoidQueenHearts, RuleAvoidHearts}, -6, 4},
		{"stacked without trick point", []RuleID{RuleNoPoints, RuleAvoidQueenHearts, RuleAvoidHearts}, -7, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.HandSize = 1
			cfg.InitialRules = tc.rules
			state := expansionDeal(t, cfg, "std:H:12", "std:H:13")
			state = expansionPlay(t, state, "b", "std:H:12")
			state = expansionPlay(t, state, "a", "std:H:13")
			if got := scoreOf(state, "a"); got != tc.want || len(state.HandAwards) != tc.awards {
				t.Fatalf("score %d awards %+v", got, state.HandAwards)
			}
			if tc.name == "stacked" && (state.HandAwards[1].Rule != RuleAvoidQueenHearts || state.HandAwards[2].Rule != RuleAvoidHearts || state.HandAwards[1].Card != "std:H:12" || state.HandAwards[2].Card != "std:H:12") {
				t.Fatalf("QH reasons %+v", state.HandAwards)
			}
		})
	}
}

func TestLastTrickBonusOneTrickAndDifferentFromMostTricks(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize = 1
	cfg.InitialRules = []RuleID{RuleLastTrickBonus}
	state := expansionDeal(t, cfg, "std:H:02", "std:H:03")
	state = expansionPlay(t, state, "b", "std:H:02")
	state = expansionPlay(t, state, "a", "std:H:03")
	if scoreOf(state, "a") != 3 || len(state.HandAwards) != 2 || state.HandAwards[1].Points != 2 {
		t.Fatalf("one-trick bonus %+v %+v", state.Seats, state.HandAwards)
	}
	if next, _, err := Step(state, Action{Kind: ActionPlayCard, Seat: "a", Card: "std:C:02"}, DefaultCatalog()); err == nil || !reflect.DeepEqual(next, state) {
		t.Fatal("completed hand accepted a second scoring play")
	}
	cfg.HandSize = 3
	state = expansionDeal(t, cfg, "std:H:14", "std:H:02", "std:D:14", "std:D:02", "std:C:02", "std:C:14")
	for _, play := range []Play{{"b", "std:H:14"}, {"a", "std:H:02"}, {"b", "std:D:14"}, {"a", "std:D:02"}, {"b", "std:C:02"}, {"a", "std:C:14"}} {
		state = expansionPlay(t, state, play.Seat, play.Card)
	}
	if state.Seats[0].TricksWon != 1 || state.Seats[1].TricksWon != 2 || scoreOf(state, "a") != 3 || scoreOf(state, "b") != 2 || state.HandAwards[len(state.HandAwards)-1].Seat != "a" {
		t.Fatalf("bonus winner %+v %+v", state.Seats, state.HandAwards)
	}
}

func TestPenaltyPitchNoPointsAndGameAwardCompose(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize = 1
	cfg.InitialRules = []RuleID{RuleSpadesTrump, RuleHighTrump, RuleGamePoint, RuleNoPoints, RuleAvoidQueenHearts, RuleAvoidHearts, RuleLastTrickBonus}
	state := expansionDeal(t, cfg, "std:S:14", "std:H:12")
	state = expansionPlay(t, state, "b", "std:S:14")
	state = expansionPlay(t, state, "a", "std:H:12")
	if scoreOf(state, "b") != -2 || state.Seats[1].TricksWon != 1 {
		t.Fatalf("composed score %+v", state.Seats)
	}
	want := map[RuleID]int{RuleAvoidQueenHearts: -5, RuleAvoidHearts: -1, RuleLastTrickBonus: 2, RuleHighTrump: 1, RuleGamePoint: 1}
	if len(state.HandAwards) != len(want) {
		t.Fatalf("composed awards %+v", state.HandAwards)
	}
	for _, award := range state.HandAwards {
		if want[award.Rule] != award.Points {
			t.Fatalf("award %+v", award)
		}
	}
}

func TestUndealtQueenAndEarlyEndKeepOnlyCompletedTricks(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize = 1
	cfg.InitialRules = []RuleID{RuleAvoidQueenHearts}
	state := expansionDeal(t, cfg, "std:H:02", "std:H:03")
	state = expansionPlay(t, state, "b", "std:H:02")
	state = expansionPlay(t, state, "a", "std:H:03")
	if scoreOf(state, "a") != 1 || len(state.HandAwards) != 1 {
		t.Fatal("undealt queen penalized")
	}
	cfg.HandSize = 2
	cfg.InitialRules = []RuleID{RuleAvoidQueenHearts, RuleAvoidHearts, RuleLastTrickBonus}
	state = expansionDeal(t, cfg, "std:H:12", "std:H:13", "std:H:04", "std:C:02")
	state = expansionPlay(t, state, "b", "std:H:12")
	state = expansionPlay(t, state, "a", "std:H:13")
	if scoreOf(state, "a") != -6 {
		t.Fatalf("completed trick score %+v", state.Seats)
	}
	var err error
	state, _, err = Step(state, Action{Kind: ActionEndGame, Seat: "a"}, DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	public, err := ProjectPublic(state, DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if scoreOf(state, "a") != -6 || len(public.HandAwards) != 4 {
		t.Fatalf("abort score %+v awards %+v", state.Seats, public.HandAwards)
	}
	for _, award := range public.HandAwards {
		if award.Rule == RuleLastTrickBonus {
			t.Fatal("unfinished hand received bonus")
		}
	}
	data, _ := json.Marshal(public)
	if strings.Contains(string(data), "std:H:04") || strings.Contains(string(data), "std:C:02") {
		t.Fatal("unplayed card leaked")
	}
}

func TestScoringExpansionCompleteMatchReplay(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize = 2
	cfg.MaxHands = 3
	cfg.RevealEvery = 1
	cfg.InitialRules = []RuleID{RuleNoPoints, RuleAvoidQueenHearts, RuleAvoidHearts, RuleLastTrickBonus}
	catalog := DefaultCatalog()
	initial, err := NewMatch([]SeatID{"a", "b"}, cfg, catalog, SeededShuffle(881))
	if err != nil {
		t.Fatal(err)
	}
	state := initial
	actions := []Action{}
	bonuses := 0
	for state.Phase != PhaseGameComplete {
		var action Action
		if state.Phase == PhaseHandComplete {
			action = Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()}
		} else {
			action = Action{Kind: ActionPlayCard, Seat: state.Turn, Card: legalCard(state, catalog)}
		}
		state, _, err = Step(state, action, catalog)
		if err != nil {
			t.Fatal(err)
		}
		actions = append(actions, action)
		if state.Phase == PhaseHandComplete || state.Phase == PhaseGameComplete {
			for _, award := range state.HandAwards {
				if award.Rule == RuleLastTrickBonus {
					bonuses++
				}
			}
		}
	}
	if state.CompletedHands != 3 || bonuses != 3 {
		t.Fatalf("complete match hands=%d bonuses=%d", state.CompletedHands, bonuses)
	}
	replay, err := ReplayMatch([]SeatID{"a", "b"}, cfg, catalog, initial.Shuffles)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range actions {
		replay, _, err = Step(replay, action, catalog)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(state, replay) {
		t.Fatal("scoring replay diverged")
	}
}

func TestScoringExpansionRevealAppliesNextHand(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize = 1
	cfg.MaxHands = 2
	cfg.RevealEvery = 1
	catalog := DefaultCatalog()
	state, err := NewMatch([]SeatID{"a", "b"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range state.RuleDraw {
		if id == RuleAvoidQueenHearts {
			state.RuleDraw[0], state.RuleDraw[i] = state.RuleDraw[i], state.RuleDraw[0]
			break
		}
	}
	state = expansionPlay(t, state, "b", state.Hands["b"][0])
	state = expansionPlay(t, state, "a", state.Hands["a"][0])
	if state.CompletedHands != 1 || state.RuleDraw[0] == RuleAvoidQueenHearts || len(state.HandAwards) != 1 || state.HandAwards[0].Rule != RuleTrickPoint {
		t.Fatalf("first hand rescored by reveal: %+v", state.HandAwards)
	}
	order := canonicalOrder()
	for i, id := range []CardID{"std:H:12", "std:H:13"} {
		for j := i; j < len(order); j++ {
			if order[j] == id {
				order[i], order[j] = order[j], order[i]
				break
			}
		}
	}
	state, _, err = Step(state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: order}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	state = expansionPlay(t, state, "a", "std:H:12")
	state = expansionPlay(t, state, "b", "std:H:13")
	if scoreOf(state, "b") != -4 || len(state.HandAwards) != 2 || state.HandAwards[1].Rule != RuleAvoidQueenHearts {
		t.Fatalf("revealed rule did not score next hand: seats=%+v awards=%+v", state.Seats, state.HandAwards)
	}
}

func TestScoringExpansionRejectsPriorCatalogState(t *testing.T) {
	catalog := DefaultCatalog()
	state, err := NewMatch([]SeatID{"a", "b"}, DefaultConfig(), catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state.CatalogVersion = 4
	if _, err := ProjectPublic(state, catalog); err == nil {
		t.Fatal("old catalog state projected under new scoring semantics")
	}
	if _, _, err := Step(state, Action{Kind: ActionPlayCard, Seat: state.Turn, Card: state.Hands[state.Turn][0]}, catalog); err == nil {
		t.Fatal("old catalog state executed under new scoring semantics")
	}
	catalog.Version = 4
	if _, err := ReplayMatch([]SeatID{"a", "b"}, DefaultConfig(), catalog, state.Shuffles); err == nil {
		t.Fatal("old catalog replay accepted")
	}
}
