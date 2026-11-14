package domain

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func ruleFixture(t *testing.T, ids ...RuleID) RuleResolution {
	t.Helper()
	field := StartingField(DefaultCatalog())
	for _, id := range ids {
		field = appendRule(field, id)
	}
	rules, err := ResolveRules(field, DefaultCatalog(), DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func TestTrumpWinnerEligibilityAndRankRemainSeparate(t *testing.T) {
	plays := []Play{{"a", "std:H:14"}, {"b", "std:S:02"}, {"c", "std:H:02"}}
	for _, test := range []struct {
		name  string
		rules RuleResolution
		trump Suit
		want  SeatID
	}{
		{"no trump high", ruleFixture(t), "", "a"},
		{"fixed trump high", ruleFixture(t, RuleSpadesTrump), Spades, "b"},
		{"fixed trump low", ruleFixture(t, RuleSpadesTrump, RuleLowCardWins), Spades, "b"},
		{"no trump low", ruleFixture(t, RuleLowCardWins), "", "c"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := WinningPlay(plays, test.rules, test.trump)
			if err != nil || got.Seat != test.want {
				t.Fatalf("winner %+v, want %s: %v", got, test.want, err)
			}
		})
	}
	trumps := []Play{{"a", "std:H:14"}, {"b", "std:S:10"}, {"c", "std:S:02"}}
	for _, test := range []struct {
		rules RuleResolution
		want  SeatID
	}{
		{ruleFixture(t, RuleSpadesTrump), "b"},
		{ruleFixture(t, RuleSpadesTrump, RuleLowCardWins), "c"},
	} {
		winner, err := WinningPlay(trumps, test.rules, Spades)
		if err != nil || winner.Seat != test.want {
			t.Fatalf("trump rank comparison: %+v, want %s: %v", winner, test.want, err)
		}
	}
	// Equal eligible ranks are impossible in a single deck, but a repeated
	// candidate still demonstrates that the comparator retains the first play.
	tie := []Play{{"a", "std:S:09"}, {"b", "std:S:09"}}
	got, err := WinningPlay(tie, ruleFixture(t, RuleSpadesTrump), Spades)
	if err != nil || got.Seat != "a" {
		t.Fatalf("tie: %+v %v", got, err)
	}
}

func TestTrumpFollowRestrictionsAndBreakFallback(t *testing.T) {
	leadHearts := []Play{{"a", "std:H:09"}}
	leadSpades := []Play{{"a", "std:S:09"}}
	hand := []CardID{"std:H:03", "std:S:02", "std:C:04"}
	strict := ruleFixture(t, RuleSpadesTrump)
	optional := ruleFixture(t, RuleSpadesTrump, RuleFollowOrTrump)
	cases := []struct {
		name  string
		rules RuleResolution
		trick []Play
		hand  []CardID
		facts HandFacts
		want  []CardID
	}{
		{"strict follows despite trump", strict, leadHearts, hand, HandFacts{TrumpSuit: Spades}, []CardID{"std:H:03"}},
		{"optional trump with lead suit", optional, leadHearts, hand, HandFacts{TrumpSuit: Spades}, []CardID{"std:H:03", "std:S:02"}},
		{"optional void allows all", optional, leadHearts, []CardID{"std:S:02", "std:C:04"}, HandFacts{TrumpSuit: Spades}, []CardID{"std:S:02", "std:C:04"}},
		{"trump lead must follow trump", optional, leadSpades, hand, HandFacts{TrumpSuit: Spades}, []CardID{"std:S:02"}},
		{"break spades", ruleFixture(t, RuleSpadesTrump, RuleBreakSpades), nil, hand, HandFacts{TrumpSuit: Spades}, []CardID{"std:H:03", "std:C:04"}},
		{"spades only exception", ruleFixture(t, RuleSpadesTrump, RuleBreakSpades), nil, []CardID{"std:S:02"}, HandFacts{TrumpSuit: Spades}, []CardID{"std:S:02"}},
		{"break hearts", ruleFixture(t, RuleBreakHearts), nil, hand, HandFacts{}, []CardID{"std:S:02", "std:C:04"}},
		{"both bans fallback", ruleFixture(t, RuleSpadesTrump, RuleBreakSpades, RuleBreakHearts), nil, []CardID{"std:S:02", "std:H:03"}, HandFacts{TrumpSuit: Spades}, []CardID{"std:S:02", "std:H:03"}},
		{"broken cards may lead", ruleFixture(t, RuleSpadesTrump, RuleBreakSpades, RuleBreakHearts), nil, hand, HandFacts{TrumpSuit: Spades, BrokenSpades: true, BrokenHearts: true}, hand},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := legalCardsWithRules(tc.hand, tc.trick, tc.rules, tc.facts)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("legal %v, want %v: %v", got, tc.want, err)
			}
		})
	}
	if reason := IllegalPlayReason(hand, leadHearts, "std:C:04", optional, HandFacts{TrumpSuit: Spades}); !strings.Contains(reason, "Follow Suit or Trump") {
		t.Fatal(reason)
	}
}

func TestPitchFirstLeadAppliesImmediatelyAndHandFactsReset(t *testing.T) {
	catalog := DefaultCatalog()
	state := mixedState(t, RulePitchTrump)
	state, events, err := Step(state, Action{Kind: ActionPlayCard, Seat: "b", Card: "std:H:02"}, catalog)
	if err != nil || state.TrumpSuit != Hearts || len(events) != 1 || events[0].TrumpSuit != Hearts {
		t.Fatalf("pitch lead: %+v %+v %v", state, events, err)
	}
	state, _, err = Step(state, Action{Kind: ActionPlayCard, Seat: "a", Card: "std:H:03"}, catalog)
	if err != nil || state.Seats[0].Score != 1 {
		t.Fatalf("pitch first trick: %+v %v", state, err)
	}
	for state.Phase == PhasePlayingTrick {
		view, err := ProjectForSeat(state, catalog, state.Turn)
		if err != nil {
			t.Fatal(err)
		}
		legal, err := LegalCards(view, catalog)
		if err != nil || len(legal) == 0 {
			t.Fatalf("legal %v %v", legal, err)
		}
		state, _, err = Step(state, Action{Kind: ActionPlayCard, Seat: state.Turn, Card: legal[0]}, catalog)
		if err != nil {
			t.Fatal(err)
		}
	}
	if state.Phase != PhaseHandComplete {
		t.Fatal(state.Phase)
	}
	state, _, err = Step(state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()}, catalog)
	if err != nil || state.TrumpSuit != "" || state.BrokenSpades || state.BrokenHearts {
		t.Fatalf("facts did not reset: %+v %v", state, err)
	}
}

func TestTrumpUnlockReplacementAndInactiveBreak(t *testing.T) {
	catalog := DefaultCatalog()
	state, err := NewMatch([]SeatID{"a", "b"}, DefaultConfig(), catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state.RuleDraw = []RuleID{RuleFollowOrTrump, RuleBreakSpades, RuleSpadesTrump, RulePitchTrump, RuleBreakHearts}
	for i := 0; i < 3; i++ {
		state, _, err = RecordCompletedHand(state, catalog)
		if err != nil {
			t.Fatal(err)
		}
	}
	if state.Field[len(state.Field)-1].DefinitionID != RuleSpadesTrump {
		t.Fatalf("ineligible cards were not deferred: %v", state.RuleDraw)
	}
	if len(state.RuleDraw) != 4 {
		t.Fatal(state.RuleDraw)
	}
	state.Field = appendRule(state.Field, RuleBreakSpades)
	state.Field = appendRule(state.Field, RuleFollowOrTrump)
	r, err := ResolveRules(state.Field, catalog, state.Config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Effect(MechanicBreakSpades); err != nil {
		t.Fatal(err)
	}
	state.Field = appendRule(state.Field, RulePitchTrump)
	r, err = ResolveRules(state.Field, catalog, state.Config)
	if err != nil {
		t.Fatal(err)
	}
	trump, _ := r.Effect(MechanicTrump)
	follow, _ := r.Effect(MechanicPlayRestriction)
	if trump.Value != ValueFirstLeadTrump || follow.Value != ValueFollowOrTrump || r.History[len(state.Field)-3].InactiveReason != "prerequisite_inactive" {
		t.Fatalf("pitch did not deactivate spade break: %+v", r)
	}
	state.Field = appendRule(state.Field, RuleSpadesTrump)
	r, err = ResolveRules(state.Field, catalog, state.Config)
	if err != nil || !r.History[len(state.Field)-4].Active {
		t.Fatalf("spade break did not reactivate: %+v %v", r, err)
	}
	state.Field[len(state.Field)-1].Removed = true
	r, err = ResolveRules(state.Field, catalog, state.Config)
	if err != nil {
		t.Fatal(err)
	}
	trump, _ = r.Effect(MechanicTrump)
	if trump.Value != ValueNoTrump || r.History[len(state.Field)-4].Active {
		t.Fatalf("removal revived prior evolution: %+v", r)
	}
}

func TestFixedTrumpEngineAndPublicProjection(t *testing.T) {
	catalog := DefaultCatalog()
	state := mixedState(t, RuleNoFollowRequired, RuleSpadesTrump)
	state.TrumpSuit = Spades
	state, _, err := Step(state, Action{Kind: ActionPlayCard, Seat: "b", Card: "std:H:02"}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	view, err := ProjectForSeat(state, catalog, "a")
	if err != nil {
		t.Fatal(err)
	}
	legal, err := LegalCards(view, catalog)
	if err != nil || !containsCard(legal, "std:S:02") || view.Public.TrumpSuit != Spades {
		t.Fatalf("projection/legal: %+v %v %v", view.Public, legal, err)
	}
	state, _, err = Step(state, Action{Kind: ActionPlayCard, Seat: "a", Card: "std:S:02"}, catalog)
	if err != nil || state.Seats[0].Score != 1 {
		t.Fatalf("trump did not beat lead: %+v %v", state.Seats, err)
	}
	old := state
	old.CatalogVersion = 2
	if _, _, err := Step(old, Action{Kind: ActionEndGame, Seat: "a"}, catalog); err == nil {
		t.Fatal("old record reinterpreted")
	}
}

func TestFixedTrumpStartingFixtureInitializesAndReplays(t *testing.T) {
	catalog := DefaultCatalog()
	for i := range catalog.Definitions {
		switch catalog.Definitions[i].ID {
		case RuleNoTrump:
			catalog.Definitions[i].StartActive = false
			catalog.Definitions[i].StartingVisibility = NotStarting
		case RuleSpadesTrump:
			catalog.Definitions[i].StartActive = true
			catalog.Definitions[i].StartingVisibility = Visible
			catalog.Definitions[i].InDeck = false
		}
	}
	state, err := NewMatch([]SeatID{"a", "b"}, DefaultConfig(), catalog, SeededShuffle(64))
	if err != nil || state.TrumpSuit != Spades {
		t.Fatalf("starting fixed trump: %+v %v", state, err)
	}
	replayed, err := ReplayMatch([]SeatID{"a", "b"}, DefaultConfig(), catalog, state.Shuffles)
	if err != nil || !reflect.DeepEqual(state, replayed) {
		t.Fatalf("fixed starter replay: %+v %v", replayed, err)
	}
}

func TestMalformedPublicHandFactsAreRejected(t *testing.T) {
	state := mixedState(t)
	state.TrumpSuit = Hearts
	if _, err := ProjectPublic(state, DefaultCatalog()); err == nil {
		t.Fatal("no-trump state exposed a trump suit")
	}
	state.TrumpSuit, state.BrokenSpades = "", true
	if _, err := ProjectPublic(state, DefaultCatalog()); err == nil {
		t.Fatal("broken suit before first play accepted")
	}
}

func TestBrokenSuitsArePublicBeforeRestrictionIsActive(t *testing.T) {
	for _, tc := range []struct {
		lead, off                  CardID
		brokenSpades, brokenHearts bool
	}{
		{"std:H:02", "std:S:02", true, false},
		{"std:C:02", "std:H:03", false, true},
	} {
		state := mixedState(t, RuleNoFollowRequired)
		var err error
		state, _, err = Step(state, Action{Kind: ActionPlayCard, Seat: "b", Card: tc.lead}, DefaultCatalog())
		if err != nil {
			t.Fatal(err)
		}
		state, events, err := Step(state, Action{Kind: ActionPlayCard, Seat: "a", Card: tc.off}, DefaultCatalog())
		if err != nil {
			t.Fatal(err)
		}
		view, err := ProjectPublic(state, DefaultCatalog())
		if err != nil || view.BrokenSpades != tc.brokenSpades || view.BrokenHearts != tc.brokenHearts || events[0].BrokeSpades != tc.brokenSpades || events[0].BrokeHearts != tc.brokenHearts {
			t.Fatalf("broken suit fact missing: %+v %+v %v", state, events, err)
		}
	}
}

func TestRestrictionErrorsNameTheActiveRule(t *testing.T) {
	state := mixedState(t, RuleBreakHearts)
	_, _, err := Step(state, Action{Kind: ActionPlayCard, Seat: "b", Card: "std:H:02"}, DefaultCatalog())
	var transition *TransitionError
	if !errors.As(err, &transition) || transition.Code != ErrIllegalLead || !strings.Contains(transition.Detail, "Break Hearts") {
		t.Fatalf("lead restriction feedback: %v", err)
	}
	state = mixedState(t, RuleSpadesTrump, RuleFollowOrTrump)
	state.TrumpSuit = Spades
	state, _, err = Step(state, Action{Kind: ActionPlayCard, Seat: "b", Card: "std:H:02"}, DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = Step(state, Action{Kind: ActionPlayCard, Seat: "a", Card: "std:C:03"}, DefaultCatalog())
	if !errors.As(err, &transition) || transition.Code != ErrMustFollowOrTrump || !strings.Contains(transition.Detail, "Follow Suit or Trump") {
		t.Fatalf("optional trump feedback: %v", err)
	}
}

func containsCard(cards []CardID, want CardID) bool {
	for _, card := range cards {
		if card == want {
			return true
		}
	}
	return false
}
