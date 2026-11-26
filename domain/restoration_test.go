package domain

import "testing"

func TestPointsPerTrickFiniteRevealWaitsForNoPoints(t *testing.T) {
	catalog := DefaultCatalog()
	cfg := DefaultConfig()
	cfg.RevealEvery = 1
	state, err := NewMatch([]SeatID{"a", "b"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state.RuleDraw = []RuleID{RulePointsPerTrick, RuleNoPoints}
	state, revealed, err := RecordCompletedHand(state, catalog)
	if err != nil || revealed == nil || revealed.DefinitionID != RuleNoPoints {
		t.Fatalf("first reveal: %+v %v", revealed, err)
	}
	state, revealed, err = RecordCompletedHand(state, catalog)
	if err != nil || revealed == nil || revealed.DefinitionID != RulePointsPerTrick {
		t.Fatalf("restoration reveal: %+v %v", revealed, err)
	}
	rules, err := ResolveRules(state.Field, catalog, cfg)
	if err != nil {
		t.Fatal(err)
	}
	effect, err := rules.Effect(MechanicTrickPoints)
	if err != nil || effect.Value != ValueOnePoint || !rules.History[len(rules.History)-1].Active {
		t.Fatalf("restoration did not remain active: %+v %v", effect, err)
	}
	cfg.InitialRules = []RuleID{RulePointsPerTrick}
	if err := ValidateInitialRules(cfg, catalog); err == nil {
		t.Fatal("redundant starting restoration accepted")
	}
	cfg.InitialRules = []RuleID{RuleNoPoints, RulePointsPerTrick}
	if err := ValidateInitialRules(cfg, catalog); err == nil {
		t.Fatal("restoration offered as a starting card")
	}
	old := catalog
	old.Version--
	if _, err := ReplayMatch([]SeatID{"a", "b"}, DefaultConfig(), old, state.Shuffles); err == nil {
		t.Fatal("old-version replay accepted")
	}
}

func TestPointsPerTrickEndlessDraftAndChoiceValidation(t *testing.T) {
	catalog := DefaultCatalog()
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize = ModeEndless, 3
	state, err := NewMatch([]SeatID{"a", "bot:1"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state.RuleDraw = []RuleID{RulePointsPerTrick, RuleNoPoints}
	state.Round, state.Phase, state.PendingBoundaryID = 7, PhaseAwaitingChoice, "test-boundary"
	state.DraftOptions = []RuleID{RulePointsPerTrick}
	if _, ok := eligibleEndlessRule(state, catalog, RulePointsPerTrick); ok {
		t.Fatal("redundant restoration offered in draft")
	}
	if options := endlessOptions(state, catalog); len(options) != 0 {
		t.Fatalf("No Points appeared without positive scoring: %v", options)
	}
	if !applyEndlessRule(&state, catalog, RuleGamePoint) {
		t.Fatal("Game Point did not apply")
	}
	if options := endlessOptions(state, catalog); len(options) == 0 || options[0] != RuleNoPoints {
		t.Fatalf("draft options with Game Point: %v", options)
	}
	if _, _, err := Step(state, Action{Kind: ActionChooseRule, Seat: "a", Rule: RulePointsPerTrick, BoundaryID: state.PendingBoundaryID}, catalog); err == nil {
		t.Fatal("server-side choice accepted redundant restoration")
	}
	if !applyEndlessRule(&state, catalog, RuleNoPoints) {
		t.Fatal("No Points did not apply")
	}
	if _, ok := eligibleEndlessRule(state, catalog, RulePointsPerTrick); !ok {
		t.Fatal("restoration missing after No Points")
	}
	if options := endlessOptions(state, catalog); len(options) == 0 || options[0] != RulePointsPerTrick {
		t.Fatalf("draft options after No Points: %v", options)
	}
	state.DraftOptions = []RuleID{RulePointsPerTrick}
	next, _, err := Step(state, Action{Kind: ActionChooseRule, Seat: "a", Rule: RulePointsPerTrick, BoundaryID: state.PendingBoundaryID}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := ResolveRules(next.Field, catalog, cfg)
	if err != nil {
		t.Fatal(err)
	}
	effect, _ := rules.Effect(MechanicTrickPoints)
	if effect.Value != ValueOnePoint || !rules.History[len(rules.History)-1].Active {
		t.Fatal("restored rule became inactive")
	}
}
