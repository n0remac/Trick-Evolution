package domain

import "testing"

func scoringState(t *testing.T, initial ...RuleID) (MatchState, RuleResolution) {
	t.Helper()
	cfg := DefaultConfig()
	cfg.InitialRules = append([]RuleID(nil), initial...)
	state, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state.DealOwner = map[CardID]SeatID{"std:S:14": "a", "std:S:02": "b", "std:S:11": "a", "std:H:10": "b", "std:D:10": "a"}
	state.StandardDraw = []CardID{"std:S:03"} // Undealt trump cannot affect high/low.
	state.TrumpSuit = Spades
	state.Captured = map[SeatID][]CardID{"a": {"std:S:02", "std:H:10", "std:D:10"}, "b": {"std:S:14", "std:S:11"}}
	rules, err := ResolveRules(state.Field, DefaultCatalog(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return state, rules
}

func TestPitchHandAwardsAndRankIndependence(t *testing.T) {
	state, rules := scoringState(t, RuleSpadesTrump, RuleLowCardWins, RuleHighTrump, RuleLowTrump, RuleJackTrump, RuleGamePoint, RuleNoPoints)
	if err := settleHandScores(&state, rules); err != nil {
		t.Fatal(err)
	}
	if len(state.HandAwards) != 4 {
		t.Fatalf("awards: %+v", state.HandAwards)
	}
	want := []SeatID{"a", "a", "b", "a"}
	for i, award := range state.HandAwards {
		if award.Seat != want[i] {
			t.Fatalf("award %d: %+v", i, award)
		}
	}
	if state.HandAwards[0].Card != "std:S:14" || state.HandAwards[1].Card != "std:S:02" || state.HandAwards[2].Card != "std:S:11" || state.HandAwards[3].Tally != 20 {
		t.Fatalf("award detail %+v", state.HandAwards)
	}
}

func TestKeepLowAndGameTie(t *testing.T) {
	state, rules := scoringState(t, RuleSpadesTrump, RuleLowTrump, RuleKeepLow, RuleGamePoint)
	state.Captured["a"] = []CardID{"std:S:02", "std:H:10", "std:S:14", "std:S:11"}
	state.Captured["b"] = []CardID{"std:D:10", "std:C:13", "std:C:12"}
	if err := settleHandScores(&state, rules); err != nil {
		t.Fatal(err)
	}
	if len(state.HandAwards) != 1 || state.HandAwards[0].Seat != "b" || state.HandAwards[0].Rule != RuleLowTrump {
		t.Fatalf("keep low/tie %+v", state.HandAwards)
	}
}

func TestUndealtTrumpNoAwardsAndInitialValidation(t *testing.T) {
	state, rules := scoringState(t, RuleSpadesTrump, RuleHighTrump, RuleLowTrump, RuleJackTrump)
	state.DealOwner = map[CardID]SeatID{"std:H:02": "a"}
	if err := settleHandScores(&state, rules); err != nil || len(state.HandAwards) != 0 {
		t.Fatalf("undealt awards %+v %v", state.HandAwards, err)
	}
	cfg := DefaultConfig()
	cfg.InitialRules = []RuleID{RuleLowTrump}
	if _, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{}); err == nil {
		t.Fatal("missing trump prerequisite accepted")
	}
	cfg.InitialRules = []RuleID{RuleSpadesTrump, RulePitchTrump}
	if _, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{}); err == nil {
		t.Fatal("conflicting trump alternatives accepted")
	}
	cfg.InitialRules = []RuleID{RuleSpadesTrump, RuleLowTrump, RuleNoPoints}
	state, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range state.RuleDraw {
		if id == RuleSpadesTrump || id == RuleLowTrump || id == RuleNoPoints {
			t.Fatal("initial card remains in draw", id)
		}
	}
	replay, err := ReplayMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), state.Shuffles)
	if err != nil || len(replay.Field) != len(state.Field) {
		t.Fatalf("initial replay %v", err)
	}
}

func TestNoTrickPointsSettleOnceAndPrivateUntilEnd(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize = 1
	cfg.InitialRules = []RuleID{RuleNoPoints, RuleGamePoint}
	state, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state.Hands["a"] = []CardID{"std:H:10"}
	state.Hands["b"] = []CardID{"std:H:11"}
	state.DealOwner = map[CardID]SeatID{"std:H:10": "a", "std:H:11": "b"}
	before, err := ProjectPublic(state, DefaultCatalog())
	if err != nil || len(before.HandAwards) != 0 {
		t.Fatal("premature score metadata", err)
	}
	state, _, err = Step(state, Action{Kind: ActionPlayCard, Seat: "b", Card: "std:H:11"}, DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	middle, err := ProjectPublic(state, DefaultCatalog())
	if err != nil || len(middle.HandAwards) != 0 {
		t.Fatal("midhand metadata leak", err)
	}
	state, _, err = Step(state, Action{Kind: ActionPlayCard, Seat: "a", Card: "std:H:10"}, DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if state.Seats[1].Score != 1 || len(state.HandAwards) != 1 || state.HandAwards[0].Rule != RuleGamePoint {
		t.Fatalf("settlement %+v %+v", state.Seats, state.HandAwards)
	}
	if _, _, err = Step(state, Action{Kind: ActionPlayCard, Seat: "b", Card: "std:H:11"}, DefaultCatalog()); err == nil {
		t.Fatal("second settlement accepted")
	}
}

func TestTrickPointReplacementAndRestoration(t *testing.T) {
	catalog := DefaultCatalog()
	cfg := DefaultConfig()
	field := StartingField(catalog)
	for _, tc := range []struct {
		id   RuleID
		want EffectValue
	}{{RuleNoPoints, ValueNoPoints}, {RulePointsPerTrick, ValueOnePoint}} {
		field = append(field, RuleInstance{InstanceID: string(tc.id), DefinitionID: tc.id, Sequence: len(field), IntroducedAfterHand: 3})
		resolved, err := ResolveRules(field, catalog, cfg)
		if err != nil {
			t.Fatal(err)
		}
		effect, err := resolved.Effect(MechanicTrickPoints)
		if err != nil || effect.Value != tc.want {
			t.Fatalf("replacement %+v %v", effect, err)
		}
	}
}

func TestEndGameMidhandDoesNotSettleTrumpAwards(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HandSize = 2
	cfg.InitialRules = []RuleID{RuleSpadesTrump, RuleHighTrump, RuleLowTrump, RuleGamePoint}
	state, err := NewMatch([]SeatID{"a", "b"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state.Hands["a"] = []CardID{"std:S:02", "std:H:03"}
	state.Hands["b"] = []CardID{"std:S:14", "std:H:02"}
	state.DealOwner = map[CardID]SeatID{"std:S:02": "a", "std:H:03": "a", "std:S:14": "b", "std:H:02": "b"}
	state, _, err = Step(state, Action{Kind: ActionPlayCard, Seat: "b", Card: "std:S:14"}, DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = Step(state, Action{Kind: ActionPlayCard, Seat: "a", Card: "std:S:02"}, DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = Step(state, Action{Kind: ActionEndGame, Seat: "a"}, DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if state.Seats[1].Score != 1 || state.CompletedHands != 0 {
		t.Fatalf("abort score %+v", state.Seats)
	}
	public, err := ProjectPublic(state, DefaultCatalog())
	if err != nil || len(public.HandAwards) != 1 || public.HandAwards[0].Rule != RuleTrickPoint {
		t.Fatal("abort omitted earned trick score or exposed future awards", err)
	}
}
