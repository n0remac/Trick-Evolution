package domain

import (
	"reflect"
	"testing"
)

func graphState(t *testing.T) (MatchState, Catalog) {
	t.Helper()
	catalog := DefaultCatalog()
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize = ModeEndless, 3
	state, err := NewMatch([]SeatID{"human", "bot:1"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	return state, catalog
}

func TestEndlessGraphReferencesAndReachability(t *testing.T) {
	catalog := DefaultCatalog()
	deck := map[RuleID]bool{}
	for _, id := range catalog.RuleDeck() {
		deck[id] = true
	}
	if len(deck) != 31 {
		t.Fatalf("deck has %d candidates", len(deck))
	}
	seen := map[RuleID]bool{}
	queue := append([]RuleID{RuleExpandedHand}, endlessStarters...)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		if !deck[id] {
			t.Fatalf("graph references nondeck rule %s", id)
		}
		seen[id] = true
		queue = append(queue, endlessFollowUps[id]...)
	}
	for id := range deck {
		if !seen[id] {
			t.Errorf("unreachable deck rule %s", id)
		}
		if _, ok := endlessFollowUps[id]; !ok {
			t.Errorf("missing graph source %s", id)
		}
	}
	for source, targets := range endlessFollowUps {
		if !deck[source] {
			t.Errorf("invalid graph source %s", source)
		}
		seenTargets := map[RuleID]bool{}
		for _, target := range targets {
			if !deck[target] || seenTargets[target] {
				t.Errorf("invalid or repeated edge %s -> %s", source, target)
			}
			seenTargets[target] = true
		}
	}
}

func TestRealDraftsReachTargetAndResetAtReplacementBoundary(t *testing.T) {
	state, catalog := graphState(t)
	chosenStrategy := false
	for steps := 0; state.Round < 9 && steps < 40; steps++ {
		switch state.Phase {
		case PhasePlayingTrick, PhaseAuction:
			state = finishHand(t, state)
		case PhaseAwaitingChoice:
			choice := RuleID("")
			for _, id := range state.DraftOptions {
				if id == RuleGamePoint {
					choice, chosenStrategy = id, true
					break
				}
			}
			state = handAction(t, state, Action{Kind: ActionChooseRule, Seat: "human", Rule: choice, BoundaryID: state.PendingBoundaryID})
		case PhaseHandComplete:
			state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "human", DeckOrder: canonicalOrder()})
		}
	}
	if !chosenStrategy || state.Round != 9 || state.Phase != PhaseAwaitingChoice {
		t.Fatalf("round-nine draft missing: round %d phase %s strategy %t", state.Round, state.Phase, chosenStrategy)
	}
	if !containsRule(state.DraftOptions, RuleFirstTo5) || !containsRule(state.DraftOptions, RuleAuction) {
		t.Fatalf("target displaced independent strategy: %v", state.DraftOptions)
	}
	state = handAction(t, state, Action{Kind: ActionChooseRule, Seat: "human", Rule: RuleFirstTo5, BoundaryID: state.PendingBoundaryID})
	state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "human", DeckOrder: canonicalOrder()})
	if targetOfState(state, catalog) != 5 || state.GameNumber < 2 || state.ShortGameRound != 0 || state.KnownGame != "" {
		t.Fatal("target deal did not start a separate unnamed game interval")
	}
	state = finishHand(t, state)
	if state.Phase == PhaseAwaitingChoice {
		state = handAction(t, state, Action{Kind: ActionChooseRule, Seat: "human", BoundaryID: state.PendingBoundaryID})
	}
	runBefore := cloneSeatScores(state.RunScores)
	state = handAction(t, state, Action{Kind: ActionRemoveRule, Seat: "human", Rule: RuleFirstTo5})
	if !applyEndlessRule(&state, catalog, RuleFirstTo21) {
		t.Fatal("target replacement was unreachable")
	}
	state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "human", DeckOrder: canonicalOrder()})
	if targetOfState(state, catalog) != 21 || state.GameScores["human"] != 0 || !reflect.DeepEqual(state.RunScores, runBefore) {
		t.Fatal("replacement did not reset only target interval")
	}
}

func targetOfState(state MatchState, catalog Catalog) int {
	r, _ := ResolveRules(state.Field, catalog, state.Config)
	return targetForRules(r)
}

func containsRule(ids []RuleID, want RuleID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func TestCuratedNamedGamePathAndIncompatibleReplacements(t *testing.T) {
	catalog := DefaultCatalog()
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize = ModeEndless, 3
	s, err := NewMatch([]SeatID{"a", "b", "c", "d"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	s = finishHand(t, s)
	s.Round = 9 // The actual early draft cadence is covered above; this probes late legal edges.
	for _, id := range []RuleID{RuleExpandedHand, RuleSixCardHand, RulePitchTrump, RuleHighTrump, RuleLowTrump, RuleJackTrump, RuleGamePoint, RuleNoPoints, RuleAuction, RuleSetBidder, RulePartnership, RuleFirstTo21, RuleWinOnBid} {
		if !applyEndlessRule(&s, catalog, id) {
			t.Fatalf("Pitch path cannot draft %s", id)
		}
	}
	s = handAction(t, s, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()})
	if s.KnownGame != "Four-point partnership Pitch variant" {
		t.Fatalf("curated Pitch path: %q", s.KnownGame)
	}
	cutthroat := finishHand(t, cloneState(s))
	cutthroat = handAction(t, cutthroat, Action{Kind: ActionRemoveRule, Seat: "a", Rule: RulePartnership})
	cutthroat = handAction(t, cutthroat, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()})
	if cutthroat.KnownGame != "Four-point cutthroat Pitch variant" {
		t.Fatalf("curated cutthroat path: %q", cutthroat.KnownGame)
	}
	s = finishHand(t, s)
	for _, id := range []RuleID{RuleWinOnBid, RuleSetBidder, RuleAuction, RuleHighTrump, RuleLowTrump, RuleJackTrump, RuleGamePoint} {
		s = handAction(t, s, Action{Kind: ActionRemoveRule, Seat: "a", Rule: id})
	}
	for _, id := range []RuleID{RuleWhistDeal, RuleWhistTrump, RuleWhistMajority, RuleFirstTo5} {
		if !applyEndlessRule(&s, catalog, id) {
			t.Fatalf("Whist replacement cannot draft %s", id)
		}
	}
	s = handAction(t, s, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()})
	if s.KnownGame != "Classic short Whist (without honours)" {
		t.Fatalf("curated Whist replacement path: %q", s.KnownGame)
	}
}

func TestEndlessOpeningAndStarterContrast(t *testing.T) {
	state, catalog := graphState(t)
	state.Round = 4
	state.RuleDraw = []RuleID{RulePitchTrump, RuleSpadesTrump, RuleAvoidHearts, RuleGamePoint, RuleLastTrickBonus}
	if got := endlessOptions(state, catalog); !reflect.DeepEqual(got, []RuleID{RulePitchTrump, RuleAvoidHearts, RuleGamePoint}) {
		t.Fatalf("opening draft: %v", got)
	}
	state.Round = 7
	state.RuleDraw = []RuleID{RuleHighTrump, RuleJackTrump, RuleBreakSpades, RuleFollowOrTrump, RuleAvoidHearts}
	if !applyEndlessRule(&state, catalog, RulePitchTrump) {
		t.Fatal("pitch")
	}
	if got := endlessOptions(state, catalog); len(got) < 2 || got[0] != RuleHighTrump || got[1] != RuleJackTrump {
		t.Fatalf("pitch edges: %v", got)
	}
	spades, catalog := graphState(t)
	spades.Round = 7
	spades.RuleDraw = []RuleID{RuleHighTrump, RuleJackTrump, RuleBreakSpades, RuleFollowOrTrump, RuleAvoidHearts}
	if !applyEndlessRule(&spades, catalog, RuleSpadesTrump) {
		t.Fatal("spades")
	}
	if got := endlessOptions(spades, catalog); len(got) < 2 || got[0] != RuleBreakSpades || got[1] != RuleHighTrump {
		t.Fatalf("spades edges: %v", got)
	}
	hearts, catalog := graphState(t)
	hearts.Round = 7
	hearts.RuleDraw = []RuleID{RuleBreakHearts, RuleAvoidQueenHearts, RuleLastTrickBonus}
	if !applyEndlessRule(&hearts, catalog, RuleAvoidHearts) {
		t.Fatal("hearts")
	}
	if got := endlessOptions(hearts, catalog); len(got) < 2 || got[0] != RuleBreakHearts || got[1] != RuleAvoidQueenHearts {
		t.Fatalf("hearts edges: %v", got)
	}
}

func TestEndlessGraphUsesResolvedSourcesAndScoringGuard(t *testing.T) {
	state, catalog := graphState(t)
	state.Round = 7
	state.RuleDraw = []RuleID{RuleBreakSpades, RuleHighTrump, RuleJackTrump, RuleNoPoints, RulePointsPerTrick}
	if !applyEndlessRule(&state, catalog, RuleSpadesTrump) || !applyEndlessRule(&state, catalog, RulePitchTrump) {
		t.Fatal("trump fixture")
	}
	if got := endlessFollowUpChoices(state, catalog); len(got) == 0 || got[0] != RuleHighTrump {
		t.Fatalf("pitch source missing: %v", got)
	} else {
		for _, id := range got {
			if id == RuleBreakSpades {
				t.Fatal("overridden spades acted as source")
			}
		}
	}
	if _, ok := eligibleEndlessRule(state, catalog, RuleNoPoints); ok {
		t.Fatal("No Points allowed without positive scoring")
	}
	if !applyEndlessRule(&state, catalog, RuleHighTrump) {
		t.Fatal("high trump")
	}
	if _, ok := eligibleEndlessRule(state, catalog, RuleNoPoints); !ok {
		t.Fatal("No Points blocked with positive scoring")
	}
	if !applyEndlessRule(&state, catalog, RuleNoPoints) {
		t.Fatal("No Points")
	}
	if got := endlessFollowUpChoices(state, catalog); len(got) == 0 || got[0] != RulePointsPerTrick {
		t.Fatalf("restoration not prioritized: %v", got)
	}
	state.RuleDraw = nil
	if got := endlessOptions(state, catalog); len(got) != 0 {
		t.Fatalf("empty pool: %v", got)
	}
	if _, ok := endlessAutomaticChoice(state, catalog); ok {
		t.Fatal("empty pool changed rule")
	}
	state.Round, state.RoundHand = 5, 3
	beforeField := append([]RuleInstance(nil), state.Field...)
	if err := settleEndlessBoundary(&state, catalog); err != nil || !reflect.DeepEqual(beforeField, state.Field) {
		t.Fatalf("empty automatic pool did not hold: %v", err)
	}
	retired, catalog := graphState(t)
	retired.Round = 7
	retired.RuleDraw = []RuleID{RuleNoPoints}
	if !applyEndlessRule(&retired, catalog, RuleGamePoint) {
		t.Fatal("retired fixture")
	}
	retired.Field[len(retired.Field)-1].Removed = true
	if got := endlessFollowUpChoices(retired, catalog); len(got) != 0 {
		t.Fatalf("retired source offered %v", got)
	}
}

func TestEndlessAutomaticForecastMatchesLiveBoundary(t *testing.T) {
	state, catalog := graphState(t)
	state.Round = 5
	state.RoundHand = 3
	if !applyEndlessRule(&state, catalog, RuleExpandedHand) || !applyEndlessRule(&state, catalog, RuleKeepOne) {
		t.Fatal("source fixture")
	}
	choice, ok := endlessAutomaticChoice(state, catalog)
	if !ok || choice != RuleLastTrickBonus {
		t.Fatalf("automatic choice: %s %t", choice, ok)
	}
	forecast, ok := forecastEndlessTo(state, 6, catalog)
	if !ok {
		t.Fatal("forecast failed")
	}
	live := cloneState(state)
	if err := settleEndlessBoundary(&live, catalog); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(forecast.Field, live.Field) || !reflect.DeepEqual(forecast.RuleDraw, live.RuleDraw) {
		t.Fatalf("forecast and live diverged: forecast=%v live=%v", forecast.RuleDraw, live.RuleDraw)
	}
}

func TestEndlessSharedDraftRejectsGuestAndStalePick(t *testing.T) {
	catalog := DefaultCatalog()
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize = ModeEndless, 3
	state, err := NewMatch([]SeatID{"human", "guest"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state.Round, state.Phase = 7, PhaseAwaitingChoice
	state.PendingBoundaryID = "shared:7"
	state.RuleDraw = []RuleID{RuleKeepOne, RuleGamePoint, RuleAvoidHearts}
	if !applyEndlessRule(&state, catalog, RuleExpandedHand) {
		t.Fatal("shared fixture")
	}
	state.DraftOptions = endlessOptions(state, catalog)
	if len(state.DraftOptions) == 0 {
		t.Fatal("no shared draft")
	}
	choice := state.DraftOptions[0]
	if _, _, err := Step(state, Action{Kind: ActionChooseRule, Seat: "guest", Rule: choice, BoundaryID: state.PendingBoundaryID}, catalog); err == nil {
		t.Fatal("guest chose for shared table")
	}
	next, _, err := Step(state, Action{Kind: ActionChooseRule, Seat: "human", Rule: choice, BoundaryID: state.PendingBoundaryID}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if next.Phase != PhaseHandComplete || next.PendingBoundaryID != "" || len(next.DraftOptions) != 0 {
		t.Fatal("shared draft did not close")
	}
	if _, _, err := Step(next, Action{Kind: ActionChooseRule, Seat: "human", Rule: choice, BoundaryID: state.PendingBoundaryID}, catalog); err == nil {
		t.Fatal("stale shared pick accepted")
	}
}

func TestEndlessMilestoneRanksNegativeScores(t *testing.T) {
	state, _ := graphState(t)
	state.ShortGameScores = map[SeatID]int{"human": -2, "bot:1": -5}
	finishShortGame(&state)
	if state.ShortGameResult == nil || !reflect.DeepEqual(state.ShortGameResult.Winners, []SeatID{"human"}) {
		t.Fatalf("negative score winner: %+v", state.ShortGameResult)
	}
}
