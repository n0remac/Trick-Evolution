package domain

import (
	"reflect"
	"strings"
	"testing"
)

func TestPitchScorecardMathBonusAndPrivacy(t *testing.T) {
	state, rules := scoringState(t, RuleSpadesTrump, RuleHighTrump, RuleLowTrump, RuleJackTrump, RuleGamePoint, RuleLastTrickBonus, RuleNoPoints)
	state.HandNumber, state.Round, state.TrickNumber = 4, 2, 5
	state.HandAwards = []HandAward{{Rule: RuleAvoidHearts, Title: "Avoid Hearts", Seat: "a", Card: "std:H:10", Trick: 2, Points: -1}}
	scoreLastTrickBonus(&state, rules, 1)
	if err := settleHandScores(&state, rules); err != nil {
		t.Fatal(err)
	}
	card := makeScorecard(state, rules, false)
	if card.HandNumber != 4 || card.Round != 2 || card.Players[0].Total != 2 || card.Players[1].Total != 3 {
		t.Fatalf("pitch totals: %+v", card)
	}
	if !reflect.DeepEqual(card.Players[0].GameTally.Cards, []GameTallyCard{{"10♥", 10}, {"10♦", 10}}) || card.Players[0].GameTally.Total != 20 || card.Players[1].GameTally.Total != 5 {
		t.Fatalf("game tally: %+v", card.Players)
	}
	for _, id := range []RuleID{RuleHighTrump, RuleLowTrump, RuleJackTrump, RuleGamePoint, RuleLastTrickBonus} {
		if !scorecardHasAward(card, id) {
			t.Fatalf("missing %s", id)
		}
	}
	state.Retained = map[SeatID]CardID{"a": "std:S:14"}
	card = makeScorecard(state, rules, false)
	for _, award := range card.Players[0].Awards {
		if award.Rule == RuleHighTrump && (!award.HiddenCard || award.Card != "") {
			t.Fatalf("retained trump leaked: %+v", award)
		}
	}
	copy := cloneScorecard(card)
	copy.Players[0].Awards[0].Points = 99
	if reflect.DeepEqual(copy, card) {
		t.Fatal("scorecard clone aliases awards")
	}
}

func TestScorecardGameTieAndUndealtJack(t *testing.T) {
	state, rules := scoringState(t, RuleSpadesTrump, RuleLowTrump, RuleKeepLow, RuleGamePoint, RuleJackTrump)
	state.Captured["a"] = []CardID{"std:S:02", "std:H:10", "std:S:14", "std:C:11"}
	state.Captured["b"] = []CardID{"std:D:10", "std:C:13", "std:C:12"}
	delete(state.DealOwner, "std:S:11")
	state.DealOwner["std:C:11"] = "a"
	if err := settleHandScores(&state, rules); err != nil {
		t.Fatal(err)
	}
	card := makeScorecard(state, rules, false)
	if scorecardHasAward(card, RuleGamePoint) || card.Players[0].GameTally.Total != 15 || card.Players[1].GameTally.Total != 15 {
		t.Fatalf("tie result: %+v", card)
	}
	joined := ""
	for _, note := range card.Notes {
		joined += note.Text
	}
	if !strings.Contains(joined, "tied at 15") || !strings.Contains(joined, "Trump Jack was not dealt") {
		t.Fatalf("missing no-award reasons: %+v", card.Notes)
	}
}

func TestScorecardEarlyEndUsesCompletedTricksOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize = 2
	cfg.InitialRules = []RuleID{RuleSpadesTrump, RuleHighTrump, RuleLowTrump, RuleGamePoint, RuleLastTrickBonus}
	state, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = Step(state, Action{Kind: ActionEndGame, Seat: "a"}, DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	card := state.LastScorecard
	if card == nil || !card.Partial || card.Players[0].GameTally != nil || scorecardHasAward(card, RuleLastTrickBonus) || scorecardHasAward(card, RuleGamePoint) {
		t.Fatalf("early end settled unfinished hand: %+v", card)
	}
}

func TestScorecardFreezesBeforeFiniteRevealAndNextDeal(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize, cfg.RevealEvery = 1, 1
	catalog := DefaultCatalog()
	state, err := NewMatch([]SeatID{"a", "b"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state.RuleDraw = []RuleID{RuleNoPoints}
	state = finishHand(t, state)
	if state.LastScorecard == nil || !scorecardHasAward(state.LastScorecard, RuleTrickPoint) || state.Field[len(state.Field)-1].DefinitionID != RuleNoPoints {
		t.Fatalf("scorecard was reinterpreted by reveal: %+v", state.LastScorecard)
	}
	state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()})
	view, err := ProjectPublic(state, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if view.LastScorecard == nil || view.LastScorecard.HandNumber != 1 || !scorecardHasAward(view.LastScorecard, RuleTrickPoint) {
		t.Fatalf("previous scorecard lost after deal: %+v", view.LastScorecard)
	}
}

func TestEndlessSeatReplacementKeepsPreviousScorecardRoster(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize = ModeEndless, 3
	state, err := NewMatch([]SeatID{"runner", "bot:1"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state = finishHand(t, state)
	prior := cloneScorecard(state.LastScorecard)
	next, err := ReplaceEndlessBotSeat(state, "bot:1", "friend")
	if err != nil {
		t.Fatal(err)
	}
	if next.Seats[1].ID != "friend" || !reflect.DeepEqual(next.LastScorecard, prior) || next.LastScorecard.Players[1].Seat != "bot:1" {
		t.Fatalf("previous hand was reassigned to a new player: %+v", next.LastScorecard)
	}
}
