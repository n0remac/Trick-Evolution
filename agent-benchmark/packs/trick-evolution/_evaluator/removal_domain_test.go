package domain

import (
	"reflect"
	"testing"
)

func benchmarkDraftState(t *testing.T) (MatchState, Catalog) {
	t.Helper()
	catalog := DefaultCatalog()
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize = ModeEndless, 3
	state, err := NewMatch([]SeatID{"host", "bot:1"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state.Field = append(state.Field, RuleInstance{InstanceID: "benchmark:expanded", DefinitionID: RuleExpandedHand, Sequence: len(state.Field), IntroducedAfterHand: 3})
	state.Round, state.Phase, state.PendingBoundaryID = 4, PhaseAwaitingChoice, "round:4:after:4"
	state.DraftOptions = []RuleID{RuleLastTrickBonus}
	return state, catalog
}

func TestBenchmarkRemoveEligibleRuleAtDraft(t *testing.T) {
	state, catalog := benchmarkDraftState(t)
	before := cloneState(state)
	for _, action := range []Action{
		{Kind: ActionKind("remove_rule"), Seat: "bot:1", Rule: RuleExpandedHand, BoundaryID: state.PendingBoundaryID},
		{Kind: ActionKind("remove_rule"), Seat: "host", Rule: RuleExpandedHand, BoundaryID: "stale"},
		{Kind: ActionKind("remove_rule"), Seat: "host", Rule: RuleFollowSuit, BoundaryID: state.PendingBoundaryID},
		{Kind: ActionKind("remove_rule"), Seat: "host", Rule: RuleBreakSpades, BoundaryID: state.PendingBoundaryID},
	} {
		got, events, err := Step(state, action, catalog)
		if err == nil || len(events) != 0 || !reflect.DeepEqual(got, before) || !reflect.DeepEqual(state, before) {
			t.Fatalf("ineligible removal changed state: action=%+v state=%+v events=%+v err=%v", action, got, events, err)
		}
	}
	beforeRevision := state.Revision
	next, events, err := Step(state, Action{Kind: ActionKind("remove_rule"), Seat: "host", Rule: RuleExpandedHand, BoundaryID: state.PendingBoundaryID}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if next.Revision != beforeRevision+1 || next.Phase != PhaseHandComplete || next.PendingBoundaryID != "" || len(next.DraftOptions) != 0 || len(next.Field) != len(state.Field) || !next.Field[len(next.Field)-1].Removed || len(events) != 1 || events[0].Kind != EventRoundDecision {
		t.Fatalf("valid removal did not settle exactly once: %+v %+v", next, events)
	}
	resolved, err := ResolveRules(next.Field, catalog, next.Config)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range resolved.ActiveField {
		if rule.Instance.DefinitionID == RuleExpandedHand {
			t.Fatal("removed rule remains active")
		}
	}
	if _, _, err := Step(next, Action{Kind: ActionKind("remove_rule"), Seat: "host", Rule: RuleExpandedHand, BoundaryID: state.PendingBoundaryID}, catalog); err == nil {
		t.Fatal("same draft removal accepted twice")
	}
}

func TestBenchmarkRemoveRejectsBrokenDependency(t *testing.T) {
	state, catalog := benchmarkDraftState(t)
	if !applyEndlessRule(&state, catalog, RuleSpadesTrump) || !applyEndlessRule(&state, catalog, RuleBreakSpades) {
		t.Fatal("could not prepare dependent rules")
	}
	before := cloneState(state)
	next, events, err := Step(state, Action{Kind: ActionKind("remove_rule"), Seat: "host", Rule: RuleSpadesTrump, BoundaryID: state.PendingBoundaryID}, catalog)
	if err == nil || len(events) != 0 || !reflect.DeepEqual(next, before) || !reflect.DeepEqual(state, before) {
		t.Fatalf("dependent rule was removed: next=%+v events=%+v err=%v", next, events, err)
	}
}

func TestBenchmarkRemoveRejectsLobbySelectedInitialRule(t *testing.T) {
	catalog := DefaultCatalog()
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize, cfg.InitialRules = ModeEndless, 3, []RuleID{RuleExpandedHand}
	state, err := NewMatch([]SeatID{"host", "bot:1"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state.Round, state.Phase, state.PendingBoundaryID = 4, PhaseAwaitingChoice, "round:4:after:4"
	state.DraftOptions = []RuleID{RuleLastTrickBonus}
	before := cloneState(state)
	next, events, err := Step(state, Action{Kind: ActionKind("remove_rule"), Seat: "host", Rule: RuleExpandedHand, BoundaryID: state.PendingBoundaryID}, catalog)
	if err == nil || len(events) != 0 || !reflect.DeepEqual(next, before) || !reflect.DeepEqual(state, before) {
		t.Fatalf("initial rule was removed: next=%+v events=%+v err=%v", next, events, err)
	}
}
