package domain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func handOrder(first ...CardID) []CardID {
	order := canonicalOrder()
	for i, id := range first {
		for j := i; j < len(order); j++ {
			if order[j] == id {
				order[i], order[j] = order[j], order[i]
				break
			}
		}
	}
	return order
}

func handAction(t *testing.T, state MatchState, action Action) MatchState {
	t.Helper()
	next, _, err := Step(state, action, DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func finishHand(t *testing.T, state MatchState) MatchState {
	t.Helper()
	for state.Phase == PhasePlayingTrick {
		state = handAction(t, state, Action{Kind: ActionPlayCard, Seat: state.Turn, Card: legalCard(state, DefaultCatalog())})
	}
	return state
}

func TestExpandedHandStableDealAndCapacity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize, cfg.MaxHands = 2, 2
	cfg.InitialRules = []RuleID{RuleExpandedHand}
	state, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	if state.DealSize != 4 || state.TricksPerHand != 4 || len(state.Hands["a"]) != 4 || len(state.Hands["b"]) != 4 {
		t.Fatalf("first expanded deal: %+v", state)
	}
	state = finishHand(t, state)
	if len(state.Retained) != 0 {
		t.Fatal("Expanded Hand alone retained a card")
	}
	state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()})
	if state.DealSize != 4 || state.TricksPerHand != 4 || len(state.StandardDraw) != 44 {
		t.Fatalf("expansion accumulated or altered stock: size=%d tricks=%d stock=%d", state.DealSize, state.TricksPerHand, len(state.StandardDraw))
	}
	cfg.HandSize = 24
	if _, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{}); err != nil {
		t.Fatalf("26 cards each must fit exactly: %v", err)
	}
	cfg.HandSize = 25
	if _, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{}); err == nil || !strings.Contains(err.Error(), "effective hand size") {
		t.Fatalf("oversized expanded deal accepted: %v", err)
	}
	cfg.HandSize = 17
	if err := ValidateInitialSetup(cfg, 3, DefaultCatalog()); err == nil {
		t.Fatal("three-seat expanded deal over 52 was accepted")
	}
}

func TestKeepOneCarryPrivacyUniquenessAndReplay(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize, cfg.MaxHands = 2, 2
	cfg.InitialRules = []RuleID{RuleKeepOne}
	state := expansionDeal(t, cfg, "std:H:12", "std:H:13", "std:C:02", "std:C:03")
	initial := state
	actions := []Action{
		{Kind: ActionPlayCard, Seat: "b", Card: "std:C:02"},
		{Kind: ActionPlayCard, Seat: "a", Card: "std:C:03"},
		{Kind: ActionStartHand, Seat: "a", DeckOrder: handOrder("std:H:12", "std:H:13", "std:D:03", "std:D:02")},
		{Kind: ActionPlayCard, Seat: "a", Card: "std:D:03"},
		{Kind: ActionPlayCard, Seat: "b", Card: "std:D:02"},
	}
	for index, action := range actions {
		state = handAction(t, state, action)
		if index == 1 {
			if state.Phase != PhaseHandComplete || state.TricksPerHand != 1 || state.Retained["b"] != "std:H:12" || state.Retained["a"] != "std:H:13" {
				t.Fatalf("first leftovers: %+v", state.Retained)
			}
			public, err := ProjectPublic(state, DefaultCatalog())
			if err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(public)
			if strings.Contains(string(data), "std:H:12") || strings.Contains(string(data), "std:H:13") || public.Seats[0].RetainedCount != 1 || public.Seats[1].RetainedCount != 1 {
				t.Fatal("private retained cards leaked or counts missing")
			}
			viewA, _ := ProjectForSeat(state, DefaultCatalog(), "a")
			privateA, _ := json.Marshal(viewA)
			if viewA.Retained != "std:H:13" || strings.Contains(string(privateA), "std:H:12") {
				t.Fatal("seat saw opponent's retained card")
			}
		}
		if index == 2 {
			if state.DealSize != 2 || state.TricksPerHand != 1 || !reflect.DeepEqual(state.Hands["b"], []CardID{"std:H:12", "std:D:02"}) || !reflect.DeepEqual(state.Hands["a"], []CardID{"std:H:13", "std:D:03"}) || len(state.StandardDraw) != 48 {
				t.Fatalf("replenishment failed: hands=%+v stock=%d", state.Hands, len(state.StandardDraw))
			}
			seen := map[CardID]bool{}
			for _, hand := range state.Hands {
				for _, id := range hand {
					if seen[id] {
						t.Fatalf("duplicate dealt card %s", id)
					}
					seen[id] = true
				}
			}
			for _, id := range state.StandardDraw {
				if seen[id] {
					t.Fatalf("carried card remained in stock: %s", id)
				}
				seen[id] = true
			}
			if len(seen) != 52 || state.DealOwner["std:H:12"] != "b" || state.DealOwner["std:H:13"] != "a" {
				t.Fatal("new hand lost deck conservation or original ownership")
			}
		}
	}
	if state.Phase != PhaseGameComplete || len(state.Hands["a"]) != 1 || len(state.Captured["a"])+len(state.Captured["b"]) != 2 {
		t.Fatal("final leftover was played or captured")
	}
	replay, err := ReplayMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), initial.Shuffles)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range actions {
		replay = handAction(t, replay, action)
	}
	if !reflect.DeepEqual(state, replay) {
		t.Fatal("recorded shuffle replay diverged across retained deal")
	}
	encoded, _ := json.Marshal(state)
	var decoded MatchState
	if err := json.Unmarshal(encoded, &decoded); err != nil || !reflect.DeepEqual(state, decoded) {
		t.Fatalf("canonical state serialization failed: %v", err)
	}
}

func TestKeepOneAndExpandedCombinedAndMinimum(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize, cfg.MaxHands = 1, 2
	cfg.InitialRules = []RuleID{RuleKeepOne}
	if _, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{}); err == nil {
		t.Fatal("one-card Keep One hand accepted")
	}
	cfg.InitialRules = []RuleID{RuleExpandedHand, RuleKeepOne}
	state, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	if state.DealSize != 3 || state.TricksPerHand != 2 {
		t.Fatal("combined hand plan is wrong")
	}
	state = finishHand(t, state)
	state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()})
	if state.DealSize != 3 || state.TricksPerHand != 2 || len(state.Hands["a"]) != 3 || len(state.Hands["b"]) != 3 || len(state.StandardDraw) != 46 {
		t.Fatalf("combined replenishment wrong: %+v", state)
	}
}

func TestRetainedPitchOwnershipAndCapturedScoring(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize, cfg.MaxHands = 2, 2
	cfg.InitialRules = []RuleID{RuleSpadesTrump, RuleHighTrump, RuleLowTrump, RuleKeepLow, RuleNoPoints, RuleKeepOne, RuleAvoidQueenHearts, RuleAvoidHearts, RuleLastTrickBonus, RuleGamePoint}
	state := expansionDeal(t, cfg, "std:S:14", "std:S:02", "std:H:12", "std:H:13")
	state = expansionPlay(t, state, "b", "std:H:12")
	state = expansionPlay(t, state, "a", "std:H:13")
	if scoreOf(state, "a") != -3 || scoreOf(state, "b") != 1 || state.Retained["a"] != "std:S:02" || state.Retained["b"] != "std:S:14" {
		t.Fatalf("first hand scores or retained trump: %+v %+v", state.Seats, state.Retained)
	}
	public, err := ProjectPublic(state, DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(public)
	if strings.Contains(string(encoded), "std:S:02") || strings.Contains(string(encoded), "std:S:14") {
		t.Fatal("Pitch score breakdown exposed retained trump identity")
	}
	hiddenAwards := 0
	for _, award := range public.HandAwards {
		if (award.Rule == RuleHighTrump || award.Rule == RuleLowTrump) && award.HiddenCard && award.Card == "" {
			hiddenAwards++
		}
	}
	if hiddenAwards != 2 {
		t.Fatalf("retained Pitch awards were not itemized privately: %+v", public.HandAwards)
	}
	state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: handOrder("std:S:14", "std:S:02", "std:C:03", "std:C:02")})
	if state.DealOwner["std:S:14"] != "b" || state.DealOwner["std:S:02"] != "a" {
		t.Fatal("carried trump original ownership missing")
	}
	state = expansionPlay(t, state, "a", "std:C:03")
	state = expansionPlay(t, state, "b", "std:C:02")
	if scoreOf(state, "a") != 0 || scoreOf(state, "b") != 2 || len(state.Captured["a"])+len(state.Captured["b"]) != 2 {
		t.Fatalf("second hand counted unplayed trumps or hearts as captures: %+v %+v", state.Seats, state.HandAwards)
	}
	for _, award := range state.HandAwards {
		if award.Rule == RuleAvoidHearts || award.Rule == RuleAvoidQueenHearts || award.Rule == RuleGamePoint {
			t.Fatalf("uncaptured card scored: %+v", award)
		}
	}
}

func TestCarryCrossesKeepOneRemovalBoundary(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize, cfg.MaxHands = 2, 2
	cfg.InitialRules = []RuleID{RuleKeepOne}
	state, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state = finishHand(t, state)
	for i := range state.Field {
		if state.Field[i].DefinitionID == RuleKeepOne {
			state.Field[i].Removed = true
		}
	}
	state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()})
	if state.TricksPerHand != 2 || len(state.Carried) != 2 {
		t.Fatal("removed Keep One did not carry the previous leftovers once")
	}
	state = finishHand(t, state)
	if len(state.Retained) != 0 || len(state.Hands["a"]) != 0 || len(state.Hands["b"]) != 0 {
		t.Fatal("inactive Keep One continued retaining cards")
	}
}

func TestHandRevealTimingAndIneligibleCapacity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize, cfg.MaxHands, cfg.RevealEvery = 2, 2, 1
	state, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state.RuleDraw[0], state.RuleDraw[1] = state.RuleDraw[1], state.RuleDraw[0] // Reveal Keep One first.
	state = finishHand(t, state)
	if state.CompletedHands != 1 || len(state.Retained) != 0 || state.TricksPerHand != 2 || state.Field[len(state.Field)-1].DefinitionID != RuleKeepOne {
		t.Fatal("newly revealed Keep One changed the completed hand")
	}
	state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()})
	if state.DealSize != 2 || state.TricksPerHand != 1 || len(state.Carried) != 0 {
		t.Fatal("newly active Keep One incorrectly carried a prior card")
	}
	state = finishHand(t, state)
	if len(state.Retained) != 2 {
		t.Fatal("newly active Keep One did not retain cards")
	}

	cfg.InitialRules = nil
	state, err = NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state = finishHand(t, state)
	if state.DealSize != 2 || state.Field[len(state.Field)-1].DefinitionID != RuleExpandedHand {
		t.Fatal("Expanded Hand reveal changed the hand that just finished")
	}
	state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()})
	if state.DealSize != 4 || state.TricksPerHand != 4 {
		t.Fatal("revealed Expanded Hand did not change the next deal")
	}

	cfg.HandSize = 17
	state, err = NewMatch([]SeatID{"a", "b", "c"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state = finishHand(t, state)
	if state.Field[len(state.Field)-1].DefinitionID == RuleExpandedHand || state.RuleDraw[0] != RuleExpandedHand {
		t.Fatal("ineligible Expanded Hand was revealed or consumed")
	}
	state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()})
	if state.DealSize != 17 || state.TricksPerHand != 16 {
		t.Fatal("capacity skip changed deal or next eligible rule")
	}

	cfg.HandSize = 1
	state, err = NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state.RuleDraw[0], state.RuleDraw[1] = state.RuleDraw[1], state.RuleDraw[0]
	state = finishHand(t, state)
	if state.Field[len(state.Field)-1].DefinitionID == RuleKeepOne || state.RuleDraw[0] != RuleKeepOne {
		t.Fatal("ineligible one-card Keep One was revealed or consumed")
	}
}

func TestKeepOneLastPlayedTrickAndUncapturedLow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize, cfg.MaxHands = 3, 1
	cfg.InitialRules = []RuleID{RuleNoPoints, RuleKeepOne, RuleLastTrickBonus}
	state := expansionDeal(t, cfg, "std:H:14", "std:H:02", "std:D:02", "std:D:14", "std:C:02", "std:C:03")
	for _, play := range []Play{{"b", "std:H:14"}, {"a", "std:H:02"}, {"b", "std:D:02"}, {"a", "std:D:14"}} {
		state = expansionPlay(t, state, play.Seat, play.Card)
	}
	if state.Phase != PhaseGameComplete || state.TricksPerHand != 2 || scoreOf(state, "a") != 2 || scoreOf(state, "b") != 0 || len(state.HandAwards) != 1 || state.HandAwards[0].Rule != RuleLastTrickBonus || state.HandAwards[0].Trick != 2 {
		t.Fatalf("last played trick bonus wrong: %+v %+v", state.Seats, state.HandAwards)
	}

	cfg.HandSize = 2
	cfg.InitialRules = []RuleID{RuleSpadesTrump, RuleLowTrump, RuleNoPoints, RuleKeepOne}
	state = expansionDeal(t, cfg, "std:S:02", "std:S:03", "std:C:02", "std:C:03")
	state = expansionPlay(t, state, "b", "std:C:02")
	state = expansionPlay(t, state, "a", "std:C:03")
	if state.Retained["b"] != "std:S:02" || scoreOf(state, "a") != 0 || scoreOf(state, "b") != 0 {
		t.Fatalf("uncaptured low trump was awarded: %+v %+v", state.Seats, state.HandAwards)
	}
}

func TestHandEvolutionRejectsPriorCatalogRecords(t *testing.T) {
	catalog := DefaultCatalog()
	state, err := NewMatch([]SeatID{"a", "b"}, DefaultConfig(), catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	old := state
	old.CatalogVersion = 5
	if _, err := ProjectPublic(old, catalog); err == nil {
		t.Fatal("version 5 state projected under hand evolution rules")
	}
	if _, _, err := Step(old, Action{Kind: ActionPlayCard, Seat: old.Turn, Card: old.Hands[old.Turn][0]}, catalog); err == nil {
		t.Fatal("version 5 state executed under hand evolution rules")
	}
	records := append([]ShuffleRecord(nil), state.Shuffles...)
	records[1].Order = append([]string(nil), records[1].Order[2:]...) // Version 5 lacked the two Hand cards.
	if _, err := ReplayMatch([]SeatID{"a", "b"}, DefaultConfig(), catalog, records); err == nil {
		t.Fatal("version 5 rule deck replay accepted")
	}
}
