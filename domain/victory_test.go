package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func targetMatch(t *testing.T, mode MatchMode, rules ...RuleID) MatchState {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize, cfg.MaxHands, cfg.RevealEvery = mode, 1, 1, 100
	cfg.InitialRules = rules
	s, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFiniteTargetControlsEndInsteadOfDefaultHandLimit(t *testing.T) {
	s := targetMatch(t, ModeFinite, RuleFirstTo5)
	for hands := 0; s.Phase != PhaseGameComplete && hands < 15; hands++ {
		s = finishHand(t, s)
		if s.Phase != PhaseGameComplete {
			if s.CompletedHands < 1 || s.GameResult != nil {
				t.Fatalf("premature game result after %d hands", s.CompletedHands)
			}
			s = handAction(t, s, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()})
		}
	}
	if s.Phase != PhaseGameComplete || s.CompletedHands <= s.Config.MaxHands || s.GameResult == nil || s.GameResult.Target != 5 {
		t.Fatalf("target did not end finite match: phase %s, hands %d, result %+v", s.Phase, s.CompletedHands, s.GameResult)
	}
	if !reflect.DeepEqual(matchWinners(s), s.GameResult.Winners) {
		t.Fatal("finite winners used historical scores")
	}
}

func TestTargetCrossingsBidQualificationAndSignedScores(t *testing.T) {
	for _, tc := range []struct {
		name     string
		scores   map[SeatID]int
		awards   []HandAward
		contract *Contract
		bidOnly  bool
		want     []SeatID
	}{
		{"simultaneous_tie", map[SeatID]int{"a": 4, "b": 4}, []HandAward{{Seat: "a", Points: 1}, {Seat: "b", Points: 1}}, nil, false, []SeatID{"a", "b"}},
		{"higher_crossing_wins", map[SeatID]int{"a": 4, "b": 4}, []HandAward{{Seat: "a", Points: 1}, {Seat: "b", Points: 2}}, nil, false, []SeatID{"b"}},
		{"negative_cannot_cross", map[SeatID]int{"a": -6, "b": 4}, []HandAward{{Seat: "a", Points: -1}}, nil, false, nil},
		{"losing_bidder", map[SeatID]int{"a": 4, "b": 4}, []HandAward{{Seat: "b", Points: 1}}, &Contract{Bidder: "a", Bid: 2, Made: false, Team: -1}, true, nil},
		{"nonbidding_leader", map[SeatID]int{"a": 4, "b": 4}, []HandAward{{Seat: "a", Points: 1}, {Seat: "b", Points: 2}}, &Contract{Bidder: "a", Bid: 2, Made: true, Team: -1}, true, nil},
		{"bidder_wins_tied_crossing", map[SeatID]int{"a": 4, "b": 4}, []HandAward{{Seat: "a", Points: 1}, {Seat: "b", Points: 1}}, &Contract{Bidder: "a", Bid: 2, Made: true, Team: -1}, true, []SeatID{"a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rules := []RuleID{RuleFirstTo5}
			if tc.bidOnly {
				rules = []RuleID{RuleAuction, RuleFirstTo5, RuleWinOnBid}
			}
			s := targetMatch(t, ModeFinite, rules...)
			s.GameScores, s.HandAwards = tc.scores, tc.awards
			if tc.contract != nil {
				s.Auction = &AuctionState{Contract: tc.contract}
			}
			r, _ := ResolveRules(s.Field, DefaultCatalog(), s.Config)
			result := settleTargetGame(&s, r)
			if result == nil && tc.want != nil || result != nil && !reflect.DeepEqual(result.Winners, tc.want) {
				t.Fatalf("result %+v, want %v", result, tc.want)
			}
		})
	}
}

func TestTargetTeamAndEndlessRunPreservation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize, cfg.RevealEvery = ModeEndless, 1, 100
	cfg.InitialRules = []RuleID{RulePartnership, RuleFirstTo5}
	s, err := NewMatch([]SeatID{"a", "b", "c", "d"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	s.RunScores["a"], s.Seats[0].Score = 12, 12
	s.GameTeamScores[0] = 4
	s.HandAwards = []HandAward{{Seat: "c", Points: 1}}
	r, _ := ResolveRules(s.Field, DefaultCatalog(), s.Config)
	result := settleTargetGame(&s, r)
	if result == nil || !reflect.DeepEqual(result.Winners, []SeatID{"a", "c"}) || s.RunScores["a"] != 12 || s.Seats[0].Score != 12 {
		t.Fatalf("team target changed run history: %+v", s)
	}
	s.GameResult = result
	syncGameContext(&s, r)
	if s.GameNumber != 2 || s.GameTeamScores[0] != 0 || s.GameScores["c"] != 0 || s.RunScores["a"] != 12 || s.Seats[0].Score != 12 {
		t.Fatal("new Endless game failed to reset only interval scores")
	}
	if s.ShortGameRound != 0 || s.ShortGameResult != nil {
		t.Fatal("target competed with three-round milestone")
	}
	encoded, _ := json.Marshal(s)
	var restored MatchState
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.GameResult, restored.GameResult) || !reflect.DeepEqual(s.GameScores, restored.GameScores) {
		t.Fatal("target result lost on reconnect snapshot")
	}
}

func TestKnownGameRecognitionExactMechanics(t *testing.T) {
	whist := []RuleID{RuleWhistDeal, RuleWhistTrump, RuleWhistMajority, RulePartnership, RuleFirstTo5}
	cfg := DefaultConfig()
	cfg.InitialRules, cfg.RevealEvery = whist, 100
	s, err := NewMatch([]SeatID{"a", "b", "c", "d"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	if s.KnownGame != "Classic short Whist (without honours)" || s.DealSize != 13 {
		t.Fatalf("Whist recognition: %q", s.KnownGame)
	}
	if s.RecognitionNotice == "" {
		t.Fatal("entering Whist was not announced")
	}
	rules, _ := ResolveRules(s.Field, DefaultCatalog(), s.Config)
	syncGameContext(&s, rules)
	if s.RecognitionNotice != "" {
		t.Fatal("recognition notice repeated without a transition")
	}
	s = finishHand(t, s)
	if s.LastScorecard == nil || s.CompletedHands != 1 {
		t.Fatal("Whist full hand failed")
	}
	variant := cfg
	variant.InitialRules = []RuleID{RuleWhistDeal, RuleWhistTrump, RuleWhistMajority, RuleFirstTo5}
	v, err := NewMatch([]SeatID{"a", "b", "c", "d"}, variant, DefaultCatalog(), identityShuffle{})
	if err != nil || v.KnownGame != "Individual Whist-style variant" {
		t.Fatalf("individual Whist: %q %v", v.KnownGame, err)
	}
	cfg.InitialRules = append(append([]RuleID(nil), whist...), RuleGamePoint)
	extra, err := NewMatch([]SeatID{"a", "b", "c", "d"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil || extra.KnownGame != "" {
		t.Fatalf("false positive with extra scoring: %q %v", extra.KnownGame, err)
	}
	pitch := []RuleID{RuleSixCardHand, RulePitchTrump, RuleNoPoints, RuleHighTrump, RuleLowTrump, RuleJackTrump, RuleGamePoint, RuleAuction, RuleSetBidder, RulePartnership, RuleFirstTo21, RuleWinOnBid}
	cfg.InitialRules = pitch
	p, err := NewMatch([]SeatID{"a", "b", "c", "d"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil || p.KnownGame != "Four-point partnership Pitch variant" {
		t.Fatalf("partnership Pitch: %q %v", p.KnownGame, err)
	}
	if p.Auction.Min != 2 || p.Auction.Max != 4 {
		t.Fatalf("Pitch auction %+v", p.Auction)
	}
	p = finishHand(t, p)
	if p.LastScorecard == nil || p.CompletedHands != 1 {
		t.Fatal("Pitch full hand failed")
	}
	cfg.InitialRules = append(append([]RuleID(nil), pitch[:9]...), pitch[10:]...)
	cut, err := NewMatch([]SeatID{"a", "b", "c", "d"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil || cut.KnownGame != "Four-point cutthroat Pitch variant" {
		t.Fatalf("cutthroat Pitch: %q %v", cut.KnownGame, err)
	}
	cfg.InitialRules = append([]RuleID(nil), pitch[:len(pitch)-1]...)
	falsePitch, err := NewMatch([]SeatID{"a", "b", "c", "d"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil || falsePitch.KnownGame != "" {
		t.Fatalf("Pitch without bid constraint: %q %v", falsePitch.KnownGame, err)
	}
}

func TestPhaseTwoSnapshotMigrationAndReplay(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeEndless
	s, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	s = finishHand(t, s)
	old := cloneState(s)
	old.CatalogVersion, old.GameNumber, old.GameRuleKey = 9, 0, ""
	old.GameScores, old.GameTeamScores, old.GameResult = nil, nil, nil
	newCard := map[RuleID]bool{RuleFirstTo5: true, RuleFirstTo21: true, RuleWinOnBid: true}
	draw := old.RuleDraw[:0]
	for _, id := range old.RuleDraw {
		if !newCard[id] {
			draw = append(draw, id)
		}
	}
	old.RuleDraw = draw
	order := old.Shuffles[1].Order[:0]
	for _, id := range old.Shuffles[1].Order {
		if !newCard[RuleID(id)] {
			order = append(order, id)
		}
	}
	old.Shuffles[1].Order = order
	migrated, err := MigratePhase2Snapshot(old)
	if err != nil {
		t.Fatal(err)
	}
	if migrated.CatalogVersion != CatalogVersion || migrated.GameNumber != 1 || !reflect.DeepEqual(migrated.Seats, old.Seats) || !reflect.DeepEqual(migrated.LastScorecard, old.LastScorecard) || !reflect.DeepEqual(migrated.RunScores, old.RunScores) {
		t.Fatal("phase-two migration changed historical data")
	}
	if _, err := ReplayMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), migrated.Shuffles[:2]); err != nil {
		t.Fatalf("migrated shuffle does not replay: %v", err)
	}
}

func TestWhistAndPitchTargetMatchesCompleteDeterministically(t *testing.T) {
	whist := []RuleID{RuleWhistDeal, RuleWhistTrump, RuleWhistMajority, RulePartnership, RuleFirstTo5}
	pitch := []RuleID{RuleSixCardHand, RulePitchTrump, RuleNoPoints, RuleHighTrump, RuleLowTrump, RuleJackTrump, RuleGamePoint, RuleAuction, RuleSetBidder, RulePartnership, RuleFirstTo21, RuleWinOnBid}
	for _, tc := range []struct {
		name  string
		rules []RuleID
	}{
		{"whist_team", whist},
		{"whist_individual", append(append([]RuleID(nil), whist[:3]...), whist[4:]...)},
		{"pitch_team", pitch},
		{"pitch_cutthroat", append(append([]RuleID(nil), pitch[:9]...), pitch[10:]...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.MaxHands, cfg.RevealEvery, cfg.InitialRules = 1, 100, tc.rules
			state, err := NewMatch([]SeatID{"a", "b", "c", "d"}, cfg, DefaultCatalog(), identityShuffle{})
			if err != nil {
				t.Fatal(err)
			}
			for hand := 0; hand < 80 && state.Phase != PhaseGameComplete; hand++ {
				state = finishHand(t, state)
				if state.Phase != PhaseGameComplete {
					state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()})
				}
			}
			if state.GameResult == nil || state.Phase != PhaseGameComplete {
				t.Fatalf("target did not finish: scores %+v, teams %+v, contract %+v", state.GameScores, state.GameTeamScores, state.Auction)
			}
			if state.LastScorecard == nil || state.LastScorecard.GameResult == nil {
				t.Fatal("completed game lost result")
			}
		})
	}
}

func TestEndlessContinuesAfterTargetVictoryWithRunScores(t *testing.T) {
	s := targetMatch(t, ModeEndless, RuleFirstTo5)
	for hand := 0; hand < 40 && s.GameResult == nil; hand++ {
		s = finishHand(t, s)
		if s.GameResult != nil {
			break
		}
		if s.Phase == PhaseAwaitingChoice {
			s = handAction(t, s, Action{Kind: ActionChooseRule, Seat: "a", BoundaryID: s.PendingBoundaryID})
		}
		s = handAction(t, s, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()})
	}
	if s.GameResult == nil || s.Phase == PhaseGameComplete || s.ShortGameResult != nil || s.ShortGameRound != 0 {
		t.Fatalf("Endless target result competed with run or milestone: %+v", s.GameResult)
	}
	result := cloneTargetResult(s.GameResult)
	run := cloneSeatScores(s.RunScores)
	personal := append([]Seat(nil), s.Seats...)
	if s.Phase == PhaseAwaitingChoice {
		s = handAction(t, s, Action{Kind: ActionChooseRule, Seat: "a", BoundaryID: s.PendingBoundaryID})
	}
	s = handAction(t, s, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()})
	if s.Phase == PhaseGameComplete || s.GameNumber != result.Number+1 || !reflect.DeepEqual(s.GameResult, result) || !reflect.DeepEqual(s.RunScores, run) {
		t.Fatal("new target game ended or erased Endless history")
	}
	for _, seat := range personal {
		if indexOfSeat(s.Seats, seat.ID) < 0 {
			t.Fatal("target victory removed a player")
		}
	}
	for _, points := range s.GameScores {
		if points != 0 {
			t.Fatal("next target game did not reset interval score")
		}
	}
}
