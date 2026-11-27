package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

func auctionMatch(t *testing.T, seats []SeatID, rules ...RuleID) MatchState {
	t.Helper()
	cfg := DefaultConfig()
	cfg.HandSize, cfg.MaxHands, cfg.InitialRules = 1, 1, rules
	state, err := NewMatch(seats, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestAuctionFallbackBoundsAndPitchLead(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize, cfg.MaxHands = 6, 1
	cfg.InitialRules = []RuleID{RulePitchTrump, RuleNoPoints, RuleHighTrump, RuleLowTrump, RuleJackTrump, RuleGamePoint, RuleAuction}
	state, err := NewMatch([]SeatID{"a", "b", "c", "d"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	if state.Phase != PhaseAuction || state.Auction.Min != 2 || state.Auction.Max != 4 || state.Turn != "b" {
		t.Fatalf("pitch auction: %+v", state.Auction)
	}
	replay, err := ReplayMatch([]SeatID{"a", "b", "c", "d"}, cfg, DefaultCatalog(), state.Shuffles)
	if err != nil || !reflect.DeepEqual(replay, state) {
		t.Fatalf("auction replay: %v", err)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var restored MatchState
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored, state) {
		t.Fatal("auction snapshot round trip diverged")
	}
	for state.Phase == PhaseAuction {
		state = handAction(t, state, Action{Kind: ActionPass, Seat: state.Turn})
	}
	if state.Auction.Contract.Bidder != "a" || state.Auction.Contract.Bid != 2 || !state.Auction.Contract.Fallback || state.Turn != "a" {
		t.Fatalf("dealer fallback: %+v", state.Auction.Contract)
	}
	lead := legalCard(state, DefaultCatalog())
	card, _ := cardByID(lead)
	state = handAction(t, state, Action{Kind: ActionPlayCard, Seat: state.Turn, Card: lead})
	if state.TrumpSuit != card.Suit {
		t.Fatal("winning bidder's first lead did not set Pitch trump")
	}
}

func TestAuctionIllegalAndSetIndividualOrTeam(t *testing.T) {
	for _, test := range []struct {
		name  string
		seats []SeatID
		rules []RuleID
		team  int
	}{
		{"individual_points", []SeatID{"a", "b"}, []RuleID{RuleAuction, RuleSetBidder}, -1},
		{"individual_tricks", []SeatID{"a", "b"}, []RuleID{RuleAuction, RuleBidTricks, RuleSetBidder}, -1},
		{"team_points", []SeatID{"a", "b", "c", "d"}, []RuleID{RulePartnership, RuleAuction, RuleSetBidder}, 1},
		{"team_tricks", []SeatID{"a", "b", "c", "d"}, []RuleID{RulePartnership, RuleAuction, RuleBidTricks, RuleSetBidder}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := auctionMatch(t, test.seats, test.rules...)
			for _, action := range []Action{{Kind: ActionBid, Seat: "a", Bid: 1}, {Kind: ActionBid, Seat: "b", Bid: 0}, {Kind: ActionBid, Seat: "b", Bid: 2}, {Kind: ActionPlayCard, Seat: "b", Card: state.Hands["b"][0]}} {
				before := cloneState(state)
				if _, _, err := Step(state, action, DefaultCatalog()); err == nil || !reflect.DeepEqual(before, state) {
					t.Fatalf("illegal auction action accepted: %+v", action)
				}
			}
			state = handAction(t, state, Action{Kind: ActionBid, Seat: "b", Bid: 1})
			if state.Phase == PhaseAuction {
				for state.Phase == PhaseAuction {
					state = handAction(t, state, Action{Kind: ActionPass, Seat: state.Turn})
				}
			}
			state = finishHand(t, state)
			c := state.Auction.Contract
			if c.Team != test.team || c.Made || c.Actual != 0 || c.Penalty != 1 || state.LastScorecard.Contract.Penalty != 1 {
				t.Fatalf("contract: %+v", c)
			}
			if test.team < 0 {
				if state.Seats[1].Score != -1 || state.Seats[0].Score != 1 {
					t.Fatalf("individual score: %+v", state.Seats)
				}
			} else if state.TeamScores[1] != -1 || state.TeamScores[0] != 1 || state.LastScorecard.Teams[1].Total != -1 {
				t.Fatalf("team score: %+v", state.TeamScores)
			}
		})
	}
}

func TestAuctionSuccessAndTeamAwardBalance(t *testing.T) {
	state := auctionMatch(t, []SeatID{"a", "b", "c", "d"}, RulePartnership, RuleAuction, RuleBidTricks, RuleSetBidder)
	for state.Phase == PhaseAuction {
		state = handAction(t, state, Action{Kind: ActionPass, Seat: state.Turn})
	}
	state = finishHand(t, state)
	if c := state.Auction.Contract; c.Bidder != "a" || c.Actual != 1 || !c.Made || c.Penalty != 0 {
		t.Fatalf("successful contract %+v", c)
	}
	if state.TeamScores[0] != 1 || state.TeamScores[1] != 0 {
		t.Fatalf("team awards duplicated: %+v", state.TeamScores)
	}
	points := auctionMatch(t, []SeatID{"a", "b", "c", "d"}, RulePartnership, RuleLastTrickBonus, RuleAuction)
	for points.Phase == PhaseAuction {
		points = handAction(t, points, Action{Kind: ActionPass, Seat: points.Turn})
	}
	points = finishHand(t, points)
	if points.Auction.Contract.Actual != 3 || !points.Auction.Contract.Made || points.TeamScores[0] != 3 || points.LastScorecard.Teams[0].Total != 3 {
		t.Fatalf("team point awards counted more than once: %+v", points.Auction.Contract)
	}
	state = MatchState{Seats: []Seat{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}, HandTeams: map[SeatID]int{"a": 0, "c": 0, "b": 1, "d": 1}, TeamScores: map[int]int{0: 0, 1: 0}, TeamAwardBalances: map[SeatID]int{}, HandNumber: 1}
	for i := 0; i < 4; i++ {
		awardPoints(&state, HandAward{Seat: teamAwardSeat(&state, 0), Points: 1, TeamWide: true})
	}
	if state.TeamScores[0] != 4 || state.Seats[0].Score != 2 || state.Seats[2].Score != 2 {
		t.Fatalf("team attribution skewed or duplicated: %+v", state)
	}
}

func TestAuctionRejectsNoPossiblePointContract(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize = 1
	cfg.InitialRules = []RuleID{RuleNoPoints, RuleAuction}
	if _, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{}); err == nil {
		t.Fatal("meaningless point auction accepted")
	}
	cfg.InitialRules = []RuleID{RuleNoPoints, RuleAuction, RuleBidTricks}
	if _, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{}); err == nil {
		t.Fatal("out-of-order setup accepted")
	}
	cfg.InitialRules = []RuleID{RuleAuction, RuleBidTricks, RuleNoPoints}
	if _, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{}); err != nil {
		t.Fatalf("trick auction should work without point awards: %v", err)
	}
}

func TestFiniteAuctionDrawKeepsMetricsAvailable(t *testing.T) {
	catalog := DefaultCatalog()
	if got := catalog.EvolutionDeck(ModeFinite); len(got) != 18 {
		t.Fatalf("default finite draw changed: %d", len(got))
	}
	cfg := DefaultConfig()
	cfg.InitialRules = []RuleID{RuleAuction, RuleBidTricks}
	state, err := NewMatch([]SeatID{"a", "b"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[RuleID]bool{}
	for _, id := range state.RuleDraw {
		seen[id] = true
	}
	if !seen[RuleBidPoints] || !seen[RuleSetBidder] || seen[RuleBidTricks] {
		t.Fatalf("finite bid alternatives: %+v", state.RuleDraw)
	}
}

func TestEndlessStrategiesIndependentAtEstablishedStage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize = ModeEndless, 3
	catalog := DefaultCatalog()
	state, err := NewMatch([]SeatID{"a", "b", "c", "d"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state.Round = 7
	if !applyEndlessRule(&state, catalog, RuleGamePoint) {
		t.Fatal("positive strategy unavailable")
	}
	options := endlessOptions(state, catalog)
	seen := map[RuleID]bool{}
	for _, id := range options {
		seen[id] = true
	}
	if !seen[RulePartnership] || !seen[RuleAuction] {
		t.Fatalf("strategies depend on a particular predecessor: %v", options)
	}
	for _, first := range []RuleID{RulePartnership, RuleAuction} {
		candidate := cloneState(state)
		if !applyEndlessRule(&candidate, catalog, first) {
			t.Fatalf("cannot select %s", first)
		}
		other := RuleAuction
		if first == RuleAuction {
			other = RulePartnership
		}
		if !endlessCandidate(candidate, catalog, endlessDrawPosition(candidate), other) {
			t.Fatalf("%s starved after %s", other, first)
		}
	}
}

func TestSetBidderReplacesEarnedPointsAndSafeRemoval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize, cfg.MaxHands = 2, 2
	cfg.InitialRules = []RuleID{RuleAuction, RuleBidTricks, RuleSetBidder}
	base, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	order := handOrder("std:H:05", "std:H:03", "std:H:02", "std:H:04")
	records := []ShuffleRecord{{Kind: StandardDeckKind, Order: make([]string, len(order))}, base.Shuffles[1]}
	for i, id := range order {
		records[0].Order[i] = string(id)
	}
	state, err := ReplayMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), records)
	if err != nil {
		t.Fatal(err)
	}
	state = handAction(t, state, Action{Kind: ActionBid, Seat: "b", Bid: 2})
	for _, play := range []struct {
		seat SeatID
		card CardID
	}{{"b", "std:H:05"}, {"a", "std:H:03"}, {"b", "std:H:02"}, {"a", "std:H:04"}} {
		state = handAction(t, state, Action{Kind: ActionPlayCard, Seat: play.seat, Card: play.card})
	}
	if c := state.Auction.Contract; c.Actual != 1 || c.Made || c.Penalty != 2 {
		t.Fatalf("failed earned contract: %+v", c)
	}
	if state.Seats[1].Score != -2 || state.Seats[0].Score != 1 || state.LastScorecard.Players[1].Total != -2 {
		t.Fatalf("penalty did not replace earned points: %+v", state.Seats)
	}
	if _, _, err := Step(state, Action{Kind: ActionRemoveRule, Seat: "a", Rule: RuleAuction}, DefaultCatalog()); err == nil {
		t.Fatal("removed auction under dependent rules")
	}
	for _, id := range []RuleID{RuleBidTricks, RuleSetBidder, RuleAuction} {
		state = handAction(t, state, Action{Kind: ActionRemoveRule, Seat: "a", Rule: id})
	}
	state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()})
	if state.Phase != PhasePlayingTrick || state.Auction != nil || state.LastScorecard.Contract.Bid != 2 {
		t.Fatal("auction removal changed history or next deal")
	}
}

func TestEndlessAuctionAndHandReplacementReachability(t *testing.T) {
	state, catalog := graphState(t)
	for state.Round < 7 {
		switch state.Phase {
		case PhasePlayingTrick, PhaseAuction:
			state = finishHand(t, state)
		case PhaseHandComplete:
			state = handAction(t, state, Action{Kind: ActionStartHand, Seat: state.Seats[0].ID, DeckOrder: canonicalOrder()})
		case PhaseAwaitingChoice:
			choice := RuleID("")
			if state.Round == 4 {
				for _, id := range state.DraftOptions {
					if id == RuleGamePoint {
						choice = id
					}
				}
			}
			state = handAction(t, state, Action{Kind: ActionChooseRule, Seat: state.Seats[0].ID, Rule: choice, BoundaryID: state.PendingBoundaryID})
		default:
			t.Fatalf("unexpected %s", state.Phase)
		}
	}
	if state.Phase != PhaseAwaitingChoice {
		t.Fatalf("round 7 not a draft: %s", state.Phase)
	}
	seen := map[RuleID]bool{}
	for _, id := range state.DraftOptions {
		seen[id] = true
	}
	if !seen[RuleAuction] || !seen[RuleSixCardHand] {
		t.Fatalf("missing auction or hand replacement: %v", state.DraftOptions)
	}
	state = handAction(t, state, Action{Kind: ActionChooseRule, Seat: state.Seats[0].ID, Rule: RuleAuction, BoundaryID: state.PendingBoundaryID})
	state = handAction(t, state, Action{Kind: ActionStartHand, Seat: state.Seats[0].ID, DeckOrder: canonicalOrder()})
	if state.Phase != PhaseAuction {
		t.Fatal("auction did not start at next deal")
	}
	for state.Phase == PhaseAuction {
		state = handAction(t, state, Action{Kind: ActionPass, Seat: state.Turn})
	}
	state = finishHand(t, state)
	state.Round = 7
	if _, ok := eligibleEndlessRule(state, catalog, RuleSixCardHand); !ok {
		t.Fatal("Six-Card Hand cannot replace Expanded Hand")
	}
	state.Round = 9
	if _, ok := eligibleEndlessRule(state, catalog, RuleWhistDeal); !ok {
		t.Fatal("Full Deal cannot replace Expanded Hand")
	}
	for _, test := range []struct {
		rule RuleID
		size int
	}{{RuleSixCardHand, 6}, {RuleWhistDeal, 13}} {
		candidate := cloneState(state)
		if !applyEndlessRule(&candidate, catalog, test.rule) {
			t.Fatalf("cannot select %s", test.rule)
		}
		candidate = handAction(t, candidate, Action{Kind: ActionStartHand, Seat: candidate.Seats[0].ID, DeckOrder: canonicalOrder()})
		if candidate.DealSize != test.size || candidate.Phase != PhaseAuction {
			t.Fatalf("%s next deal: %d %s", test.rule, candidate.DealSize, candidate.Phase)
		}
	}
}

func TestPhaseOneSnapshotMigrationPreservesHistory(t *testing.T) {
	for _, mode := range []MatchMode{ModeFinite, ModeEndless} {
		cfg := DefaultConfig()
		cfg.Mode = mode
		state, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{})
		if err != nil {
			t.Fatal(err)
		}
		state = finishHand(t, state)
		old := cloneState(state)
		old.CatalogVersion = 8
		field := old.Field[:0]
		for _, rule := range old.Field {
			if rule.DefinitionID != RuleDefaultBidPoints {
				rule.Sequence = len(field)
				field = append(field, rule)
			}
		}
		old.Field = field
		if mode == ModeEndless {
			newCards := map[RuleID]bool{RuleAuction: true, RuleBidPoints: true, RuleBidTricks: true, RuleSetBidder: true, RuleFirstTo5: true, RuleFirstTo21: true, RuleWinOnBid: true}
			draw := old.RuleDraw[:0]
			for _, id := range old.RuleDraw {
				if !newCards[id] {
					draw = append(draw, id)
				}
			}
			old.RuleDraw = draw
			order := old.Shuffles[1].Order[:0]
			for _, id := range old.Shuffles[1].Order {
				if !newCards[RuleID(id)] {
					order = append(order, id)
				}
			}
			old.Shuffles[1].Order = order
		}
		if _, err := ProjectPublic(old, DefaultCatalog()); err == nil {
			t.Fatal("old snapshot silently accepted")
		}
		migrated, err := MigratePhase1Snapshot(old)
		if err != nil {
			t.Fatal(err)
		}
		if migrated.CatalogVersion != CatalogVersion || migrated.CompletedHands != old.CompletedHands || !reflect.DeepEqual(migrated.LastScorecard, old.LastScorecard) || !reflect.DeepEqual(migrated.Seats, old.Seats) {
			t.Fatal("migration rewrote historical scores")
		}
		if _, err := ReplayMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), migrated.Shuffles[:2]); err != nil {
			t.Fatalf("migrated replay %s: %v", mode, err)
		}
	}
}
