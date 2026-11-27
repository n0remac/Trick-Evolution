package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPartnershipBotSeatTakeoverAndStateRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeEndless
	cfg.InitialRules = []RuleID{RulePartnership, RuleWhistTrump}
	state, err := NewMatch([]SeatID{"human", "bot:1", "bot:2", "bot:3"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state.TeamScores[0] = 4
	state.RunScores["bot:2"] = 9
	state.LastScorecard = &Scorecard{HandNumber: 1, Teams: []ScorecardTeam{{Team: 0, Seats: []SeatID{"human", "bot:2"}, Total: 4}}}
	replaced, err := ReplaceEndlessBotSeat(state, "bot:2", "guest")
	if err != nil || replaced.HandTeams["guest"] != 0 || replaced.TeamScores[0] != 4 || replaced.RunScores["guest"] != 9 || len(replaced.Hands["guest"]) == 0 {
		t.Fatalf("takeover lost frozen team or seat ledger: %+v %v", replaced.HandTeams, err)
	}
	if replaced.LastScorecard.Teams[0].Seats[1] != "bot:2" {
		t.Fatal("historical scorecard roster changed")
	}
	encoded, err := json.Marshal(replaced)
	if err != nil {
		t.Fatal(err)
	}
	var restored MatchState
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.HandTeams, replaced.HandTeams) || !reflect.DeepEqual(restored.TeamScores, replaced.TeamScores) || restored.TurnedTrumpCard != replaced.TurnedTrumpCard || !reflect.DeepEqual(restored.LastScorecard, replaced.LastScorecard) {
		t.Fatal("state persistence lost phase-one facts")
	}
	if _, err := ProjectPublic(restored, DefaultCatalog()); err != nil {
		t.Fatalf("restored public projection: %v", err)
	}
}

func TestRuleRemovalAtBoundaryKeepsHistoricalHand(t *testing.T) {
	state := fourSeatMatch(t, RulePartnership, RuleWhistTrump)
	state.Phase = PhaseHandComplete
	state.LastScorecard = &Scorecard{HandNumber: 1, Teams: []ScorecardTeam{{Team: 0, Seats: []SeatID{"a", "c"}, Total: 2}}}
	for _, seat := range state.Seats {
		state.Hands[seat.ID] = nil
	}
	for i := range state.Field {
		if state.Field[i].DefinitionID == RulePartnership || state.Field[i].DefinitionID == RuleWhistTrump {
			state.Field[i].Removed = true
		}
	}
	next, _, err := Step(state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()}, DefaultCatalog())
	if err != nil || next.HandTeams != nil || next.TeamScores != nil || next.TurnedTrumpCard != "" || next.TrumpSuit != "" || next.LastScorecard.Teams[0].Total != 2 {
		t.Fatalf("removal changed current or historical facts: %+v %v", next, err)
	}
}

func fourSeatMatch(t *testing.T, rules ...RuleID) MatchState {
	t.Helper()
	cfg := DefaultConfig()
	cfg.InitialRules = rules
	state, err := NewMatch([]SeatID{"a", "b", "c", "d"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func appendPhaseRule(state *MatchState, id RuleID) {
	state.Field = append(state.Field, RuleInstance{InstanceID: "test:" + string(id), DefinitionID: id, Sequence: len(state.Field), IntroducedAfterHand: state.CompletedHands})
}

func TestPartnershipActivationRestorationAndFrozenHistory(t *testing.T) {
	state := fourSeatMatch(t)
	state.Seats[0].Score, state.Seats[2].Score = 7, 11
	state.Phase = PhaseHandComplete
	for _, seat := range state.Seats {
		state.Hands[seat.ID] = nil
	}
	appendPhaseRule(&state, RulePartnership)
	next, _, err := Step(state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()}, DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(next.HandTeams, map[SeatID]int{"a": 0, "b": 1, "c": 0, "d": 1}) || next.TeamScores[0] != 0 || next.Seats[0].Score != 7 || next.Seats[2].Score != 11 {
		t.Fatalf("partnership pooled history or changed seats: %+v %+v", next.HandTeams, next.TeamScores)
	}
	awardPoints(&next, HandAward{Rule: RuleGamePoint, Seat: "c", Points: 1})
	if next.TeamScores[0] != 1 || next.Seats[0].Score != 7 || next.Seats[2].Score != 12 {
		t.Fatal("team award lost attribution")
	}
	old := cloneState(next)
	next.Phase = PhaseHandComplete
	for _, seat := range next.Seats {
		next.Hands[seat.ID] = nil
	}
	appendPhaseRule(&next, RuleRestoreIndividual)
	restored, _, err := Step(next, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()}, DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if restored.HandTeams != nil || restored.TeamScores != nil || restored.Seats[2].Score != 12 || old.TeamScores[0] != 1 {
		t.Fatal("restoration changed historical scores")
	}
	removed := cloneState(state)
	removed.Field[len(removed.Field)-1].Removed = true
	if _, err := ResolveRules(removed.Field, DefaultCatalog(), removed.Config); err != nil {
		t.Fatalf("removed partnership broke field: %v", err)
	}
}

func TestWhistFullDealTurnUpAndReplay(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InitialRules = []RuleID{RuleWhistDeal, RuleWhistTrump}
	state, err := NewMatch([]SeatID{"a", "b", "c", "d"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	if state.DealSize != 13 || len(state.StandardDraw) != 0 || state.TurnedTrumpCard != "std:S:14" || state.TrumpSuit != Spades || state.DealOwner[state.TurnedTrumpCard] != state.Dealer {
		t.Fatalf("bad first Whist deal: size=%d remainder=%d turned=%s trump=%s", state.DealSize, len(state.StandardDraw), state.TurnedTrumpCard, state.TrumpSuit)
	}
	public, err := ProjectPublic(state, DefaultCatalog())
	if err != nil || public.TurnedTrumpCard != state.TurnedTrumpCard || public.Seats[0].HandCount != 13 {
		t.Fatalf("public turn up: %+v %v", public, err)
	}
	replayed, err := ReplayMatch([]SeatID{"a", "b", "c", "d"}, cfg, DefaultCatalog(), state.Shuffles)
	if err != nil || !reflect.DeepEqual(replayed.Hands, state.Hands) || replayed.TurnedTrumpCard != state.TurnedTrumpCard {
		t.Fatalf("replay: %v", err)
	}
	state.Phase = PhaseHandComplete
	for _, seat := range state.Seats {
		state.Hands[seat.ID] = nil
	}
	next, _, err := Step(state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()}, DefaultCatalog())
	if err != nil || next.Dealer != "b" || next.TurnedTrumpCard != next.Hands["b"][12] || next.DealOwner[next.TurnedTrumpCard] != "b" {
		t.Fatalf("rotated turn up: dealer=%s turned=%s err=%v", next.Dealer, next.TurnedTrumpCard, err)
	}
	three := cfg
	three.InitialRules = []RuleID{RuleWhistDeal, RuleWhistTrump}
	s3, err := NewMatch([]SeatID{"a", "b", "c"}, three, DefaultCatalog(), identityShuffle{})
	if err != nil || s3.DealSize != 17 || len(s3.StandardDraw) != 1 || s3.TurnedTrumpCard != "std:S:13" || s3.StandardDraw[0] != "std:S:14" {
		t.Fatalf("three-seat remainder: %d %d %v", s3.DealSize, len(s3.StandardDraw), err)
	}
}

func TestWhistIndividualMajorityAndHandCombinations(t *testing.T) {
	cfg := DefaultConfig()
	cfg.InitialRules = []RuleID{RuleWhistMajority, RuleWhistDeal}
	state, err := NewMatch([]SeatID{"a", "b", "c"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	for _, seat := range state.Seats {
		state.Captured[seat.ID] = nil
	}
	for i, card := range StandardDeck()[:51] {
		switch {
		case i < 24:
			state.Captured["a"] = append(state.Captured["a"], card.ID)
		case i < 39:
			state.Captured["b"] = append(state.Captured["b"], card.ID)
		default:
			state.Captured["c"] = append(state.Captured["c"], card.ID)
		}
	}
	rules, err := ResolveRules(state.Field, DefaultCatalog(), state.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := settleHandScores(&state, rules); err != nil || state.Seats[0].Score != 3 || state.Seats[1].Score != 0 || state.Seats[2].Score != 0 {
		t.Fatalf("individual majority: %+v %v", state.Seats, err)
	}
	for _, ids := range [][]RuleID{{RuleWhistDeal, RuleExpandedHand}, {RuleWhistDeal, RuleKeepOne}, {RuleSixCardHand, RuleExpandedHand}, {RuleSixCardHand, RuleWhistDeal}} {
		bad := DefaultConfig()
		bad.InitialRules = ids
		if _, err := NewMatch([]SeatID{"a", "b", "c", "d"}, bad, DefaultCatalog(), identityShuffle{}); err == nil {
			t.Fatalf("accepted incompatible hand rules %v", ids)
		}
	}
	six := DefaultConfig()
	six.InitialRules = []RuleID{RuleSixCardHand}
	s, err := NewMatch([]SeatID{"a", "b", "c", "d"}, six, DefaultCatalog(), identityShuffle{})
	if err != nil || s.DealSize != 6 {
		t.Fatalf("six-card hand: %d %v", s.DealSize, err)
	}
}

func TestWhistMajorityUsesTeamTricksAndGameUsesTeamTally(t *testing.T) {
	state := fourSeatMatch(t, RulePartnership, RuleWhistMajority, RuleGamePoint)
	// Team A/C captured 8 tricks, B/D captured 5. Game tally is deliberately
	// split so team A/C wins even though B has the highest individual tally.
	state.TricksPerHand = 13
	for _, seat := range state.Seats {
		state.Captured[seat.ID] = nil
	}
	priorityA := []CardID{"std:C:10", "std:S:10", "std:H:14", "std:C:13", "std:C:12"}
	priorityB := []CardID{"std:H:10", "std:D:10", "std:D:14", "std:C:14", "std:H:13", "std:D:13"}
	used := map[CardID]bool{}
	for _, id := range priorityA {
		state.Captured["a"] = append(state.Captured["a"], id)
		used[id] = true
	}
	for _, id := range priorityB {
		state.Captured["b"] = append(state.Captured["b"], id)
		used[id] = true
	}
	for _, card := range StandardDeck() {
		if used[card.ID] {
			continue
		}
		switch {
		case len(state.Captured["a"]) < 16 && gameCardValue(card.Rank) == 0:
			state.Captured["a"] = append(state.Captured["a"], card.ID)
		case len(state.Captured["b"]) < 20 && gameCardValue(card.Rank) == 0:
			state.Captured["b"] = append(state.Captured["b"], card.ID)
		default:
			state.Captured["c"] = append(state.Captured["c"], card.ID)
		}
	}
	if len(state.Captured["a"]) != 16 || len(state.Captured["b"]) != 20 || len(state.Captured["c"]) != 16 {
		t.Fatal("bad fixture")
	}
	rules, err := ResolveRules(state.Field, DefaultCatalog(), state.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := settleHandScores(&state, rules); err != nil {
		t.Fatal(err)
	}
	if state.TeamScores[0] != 3 || state.TeamScores[1] != 0 {
		t.Fatalf("team scores: %v awards: %+v", state.TeamScores, state.HandAwards)
	}
	gameAwards := 0
	for _, award := range state.HandAwards {
		if award.Rule == RuleGamePoint {
			gameAwards++
		}
	}
	if gameAwards != 1 {
		t.Fatalf("Game awarded %d times", gameAwards)
	}
}

func TestExistingAwardsShareTeamLedgerWithOriginalAttribution(t *testing.T) {
	state := fourSeatMatch(t, RulePartnership, RuleSpadesTrump, RuleHighTrump, RuleLowTrump, RuleJackTrump, RuleAvoidHearts, RuleAvoidQueenHearts, RuleLastTrickBonus)
	state.DealOwner = map[CardID]SeatID{"std:S:14": "c", "std:S:02": "a", "std:S:11": "d"}
	state.Captured = map[SeatID][]CardID{"a": nil, "b": {"std:S:02", "std:S:11"}, "c": nil, "d": nil}
	state.CurrentTrick = []Play{{Seat: "a", Card: "std:H:12"}}
	rules, err := ResolveRules(state.Field, DefaultCatalog(), state.Config)
	if err != nil {
		t.Fatal(err)
	}
	if err := scoreCapturedTrick(&state, rules, 0); err != nil {
		t.Fatal(err)
	}
	scoreLastTrickBonus(&state, rules, 1)
	if err := settleHandScores(&state, rules); err != nil {
		t.Fatal(err)
	}
	if state.TeamScores[0] != -5 || state.TeamScores[1] != 4 || state.Seats[0].Score != -6 || state.Seats[1].Score != 4 || state.Seats[2].Score != 1 {
		t.Fatalf("award team/seat ledgers: teams=%v seats=%+v awards=%+v", state.TeamScores, state.Seats, state.HandAwards)
	}
	card := makeScorecard(state, rules, false)
	if len(card.Teams) != 2 || card.Teams[0].Total != -5 || card.Teams[1].Total != 4 {
		t.Fatalf("team scorecard: %+v", card.Teams)
	}
}

func TestPartnershipCuratedEligibilityIsIndependent(t *testing.T) {
	state := fourSeatMatch(t)
	state.Config.Mode = ModeEndless
	state.Round, state.RoundHand = 6, 0
	state.RuleDraw = []RuleID{RulePartnership}
	if _, ok := eligibleEndlessRule(state, DefaultCatalog(), RulePartnership); ok {
		t.Fatal("baseline scoring alone is not an established scoring evolution")
	}
	appendPhaseRule(&state, RuleGamePoint)
	if _, ok := eligibleEndlessRule(state, DefaultCatalog(), RulePartnership); !ok {
		t.Fatal("four-seat established scoring should qualify")
	}
	if got := endlessOptions(state, DefaultCatalog()); len(got) == 0 || got[0] != RulePartnership {
		t.Fatalf("partnership absent: %v", got)
	}
	state.Round = 5
	if _, ok := eligibleEndlessRule(state, DefaultCatalog(), RulePartnership); ok {
		t.Fatal("early partnership")
	}
	state.Round = 6
	state.Seats = state.Seats[:3]
	if _, ok := eligibleEndlessRule(state, DefaultCatalog(), RulePartnership); ok {
		t.Fatal("three-seat partnership")
	}
}
