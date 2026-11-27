package domain

import (
	"strings"
	"testing"
)

func TestEndlessOpeningAddsExpandedHandOnce(t *testing.T) {
	catalog := DefaultCatalog()
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize = ModeEndless, 3
	state, err := NewMatch([]SeatID{"human", "bot:1"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	if state.DealSize != 3 || len(state.Hands["human"]) != 3 {
		t.Fatal("round one did not deal three cards")
	}
	for i := 0; i < 3; i++ {
		state = finishHand(t, state)
		if i < 2 {
			state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "human", DeckOrder: canonicalOrder()})
			if state.DealSize != 3 {
				t.Fatal("round one deal changed early")
			}
		}
	}
	if state.Round != 2 || state.Phase != PhaseHandComplete || !strings.Contains(state.TableCue, "Expanded Hand") {
		t.Fatalf("opening boundary: %+v", state)
	}
	count := 0
	for _, rule := range state.Field {
		if rule.DefinitionID == RuleExpandedHand && !rule.Removed {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("Expanded Hand applied %d times", count)
	}
	state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "human", DeckOrder: canonicalOrder()})
	if state.DealSize != 5 || len(state.Seats) != 3 {
		t.Fatalf("round two deal: %d cards, %d players", state.DealSize, len(state.Seats))
	}
	state = finishHand(t, state)
	state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "human", DeckOrder: canonicalOrder()})
	if state.DealSize != 5 {
		t.Fatalf("expanded hand accumulated: %d", state.DealSize)
	}
}

func TestEndlessCadenceScoreAndOnceOnlyDraft(t *testing.T) {
	catalog := DefaultCatalog()
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize = ModeEndless, 3
	state, err := NewMatch([]SeatID{"human", "bot"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	if state.Round != 1 || state.RoundHand != 1 || state.DealSize != 3 {
		t.Fatalf("opening: %+v", state)
	}
	drafts, tables, holds := 0, 0, 0
	for state.CompletedHands < 12 {
		switch state.Phase {
		case PhaseAwaitingChoice:
			drafts++
			if state.PendingBoundaryID == "" || len(state.DraftOptions) == 0 || len(state.DraftOptions) > 3 {
				t.Fatalf("draft state: %+v", state)
			}
			choice := RuleID("")
			if drafts == 1 {
				choice = state.DraftOptions[0]
			}
			boundary := state.PendingBoundaryID
			state, _, err = Step(state, Action{Kind: ActionChooseRule, Seat: "human", Rule: choice, BoundaryID: boundary}, catalog)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := Step(state, Action{Kind: ActionChooseRule, Seat: "human", Rule: choice, BoundaryID: boundary}, catalog); err == nil {
				t.Fatal("draft accepted twice")
			}
		case PhaseHandComplete:
			state, _, err = Step(state, Action{Kind: ActionStartHand, Seat: "human", DeckOrder: canonicalOrder()}, catalog)
			if err != nil {
				t.Fatal(err)
			}
		case PhasePlayingTrick:
			view, err := ProjectForSeat(state, catalog, state.Turn)
			if err != nil {
				t.Fatal(err)
			}
			legal, err := LegalCards(view, catalog)
			if err != nil || len(legal) == 0 {
				t.Fatalf("legal: %v %v", legal, err)
			}
			before := state.CompletedHands
			state, _, err = Step(state, Action{Kind: ActionPlayCard, Seat: state.Turn, Card: legal[0]}, catalog)
			if err != nil {
				t.Fatal(err)
			}
			if state.CompletedHands > before {
				if strings.Contains(state.TableCue, "table added") || strings.Contains(state.TableCue, "table retired") {
					tables++
				}
				if strings.Contains(state.TableCue, "rules stay the same") {
					holds++
				}
			}
		case PhaseAuction:
			state, _, err = Step(state, Action{Kind: ActionPass, Seat: state.Turn}, catalog)
			if err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("unexpected phase %s", state.Phase)
		}
	}
	if state.Phase == PhaseGameComplete || state.HandNumber != 12 || drafts < 2 || tables < 1 || holds < 1 {
		t.Fatalf("progression: hands=%d drafts=%d tables=%d holds=%d phase=%s", state.CompletedHands, drafts, tables, holds, state.Phase)
	}
	for _, seat := range state.Seats {
		if seat.Score != state.RunScores[seat.ID] {
			t.Fatalf("run score for %s lost at roster change", seat.ID)
		}
	}
	if state.ShortGameResult == nil || state.ShortGameResult.Number != 2 || state.ShortGameRound == 0 {
		t.Fatalf("missing recurring score milestone: %+v", state.ShortGameResult)
	}
}

func TestEndlessCandidatesKeepLegalFieldAndDependencies(t *testing.T) {
	catalog := DefaultCatalog()
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize = ModeEndless, 3
	state, err := NewMatch([]SeatID{"human", "bot"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	state.Round = 4
	state.RuleDraw = []RuleID{RuleLastTrickBonus}
	if options := endlessOptions(state, catalog); len(options) != 1 || options[0] != RuleLastTrickBonus {
		t.Fatalf("single option: %v", options)
	}
	state.RuleDraw = nil
	if options := endlessOptions(state, catalog); len(options) != 0 {
		t.Fatalf("no option: %v", options)
	}
	if !applyEndlessRule(&state, catalog, RuleSpadesTrump) || !applyEndlessRule(&state, catalog, RuleBreakSpades) {
		t.Fatal("could not make dependent field")
	}
	unsafe := append([]RuleInstance(nil), state.Field...)
	unsafe[len(unsafe)-2].Removed = true
	if preservesDependencies(state.Field, unsafe, catalog, cfg) {
		t.Fatal("retirement orphaned Break Spades")
	}
	state.RuleDraw = []RuleID{RuleExpandedHand}
	if !applyEndlessRule(&state, catalog, RuleExpandedHand) {
		t.Fatal("valid expanded deal was rejected")
	}
	resolved, err := ResolveRules(state.Field, catalog, cfg)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := HandPlanForRules(cfg, 2, resolved)
	if err != nil || plan.DealSize != 5 {
		t.Fatalf("next deal: %+v, %v", plan, err)
	}
}

func TestEndlessCompatibilityChecksEffectsDependenciesAndDeal(t *testing.T) {
	catalog := DefaultCatalog()
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize = ModeEndless, 3
	a, err := NewMatch([]SeatID{"a", "bot:1"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewMatch([]SeatID{"b", "bot:1"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	a.Round, b.Round = 3, 3
	a.RoundHand, b.RoundHand = 0, 0
	a.Phase, b.Phase = PhaseHandComplete, PhaseHandComplete
	if !EndlessCompatible(a, b, catalog) {
		t.Fatal("equal runs should fit")
	}
	b.Round = 2
	if !EndlessCompatible(a, b, catalog) {
		t.Fatal("different round positions should fit")
	}
	a.Round, b.Round = 5, 5
	a.ShortGameRound, b.ShortGameRound = 2, 2
	if !EndlessCompatible(a, b, catalog) {
		t.Fatal("matching should remain available between milestone hands")
	}
	b.Round = 3
	if !applyEndlessRule(&b, catalog, RuleLastTrickBonus) || EndlessCompatible(a, b, catalog) {
		t.Fatal("different resolved effects accepted")
	}
	b = cloneState(a)
	b.Seats[0].ID = "b"
	b.Config.HandSize = 4
	if EndlessCompatible(a, b, catalog) {
		t.Fatal("different deal accepted")
	}
	b = cloneState(a)
	b.Seats[0].ID = "b"
	b.Field = append(b.Field, RuleInstance{InstanceID: "bad-dependency", DefinitionID: RuleBreakSpades, Sequence: len(b.Field)})
	if EndlessCompatible(a, b, catalog) {
		t.Fatal("inactive dependency accepted")
	}
	b = cloneState(a)
	b.Seats[0].ID = "b"
	b.Field = append(b.Field, RuleInstance{InstanceID: "trump", DefinitionID: RuleSpadesTrump, Sequence: len(b.Field)})
	b.Field = append(b.Field, RuleInstance{InstanceID: "bad-overwrite", DefinitionID: RuleFollowOrTrump, Sequence: len(b.Field)})
	b.Field = append(b.Field, RuleInstance{InstanceID: "bad-overwrite-2", DefinitionID: RuleNoFollowRequired, Sequence: len(b.Field)})
	if EndlessCompatible(a, b, catalog) {
		t.Fatal("illegal overwrite accepted")
	}
	if _, err := JoinEndless(a, b, catalog, canonicalOrder()); err == nil {
		t.Fatal("invalid transfer accepted")
	}
}

func TestEndlessForecastSearchesTwoScheduledTableChanges(t *testing.T) {
	catalog := DefaultCatalog()
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize = ModeEndless, 3
	a, err := NewMatch([]SeatID{"a", "bot:1"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	b := cloneState(a)
	b.Seats[0].ID = "b"
	a.Round, b.Round = 2, 7
	if !applyEndlessRule(&a, catalog, RuleExpandedHand) {
		t.Fatal("opening rule")
	}
	b = cloneState(a)
	b.Seats[0].ID = "b"
	b.Round = 7
	if !applyEndlessRule(&b, catalog, RuleKeepOne) || !applyEndlessRule(&b, catalog, RuleLastTrickBonus) {
		t.Fatal("fixture rules")
	}
	before := append([]RuleID(nil), a.RuleDraw...)
	gap, ok := EndlessForecastGap(a, b, catalog, "")
	if !ok || gap != 0 {
		t.Fatalf("two scheduled changes should align: gap=%d ok=%t", gap, ok)
	}
	if len(a.RuleDraw) != len(before) || a.RuleDraw[0] != before[0] || a.RuleDraw[1] != before[1] {
		t.Fatal("forecast mutated live draw")
	}
}

func TestEndlessKeepOneResetsAtRosterChange(t *testing.T) {
	catalog := DefaultCatalog()
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize, cfg.InitialRules = ModeEndless, 3, []RuleID{RuleKeepOne}
	state, err := NewMatch([]SeatID{"human", "bot"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	for hand := 1; hand <= 3; hand++ {
		state = finishHand(t, state)
		if len(state.Retained) != 2 {
			t.Fatalf("hand %d did not retain cards", hand)
		}
		public, err := ProjectPublic(state, catalog)
		if err != nil {
			t.Fatal(err)
		}
		for _, award := range public.HandAwards {
			if award.Card == state.Retained["human"] || award.Card == state.Retained["bot"] {
				t.Fatal("retained card leaked in awards")
			}
		}
		if hand < 3 {
			prior := state.Retained["human"]
			state, _, err = Step(state, Action{Kind: ActionStartHand, Seat: "human", DeckOrder: canonicalOrder()}, catalog)
			if err != nil || state.Carried["human"] != prior {
				t.Fatalf("hand %d carry: %v", hand+1, err)
			}
		}
	}
	if state.Phase != PhaseHandComplete || state.Round != 2 {
		t.Fatal("third hand did not add the opening rule")
	}
	state, _, err = Step(state, Action{Kind: ActionStartHand, Seat: "human", DeckOrder: canonicalOrder()}, catalog)
	if err != nil || len(state.Seats) != 3 || len(state.Carried) != 0 || len(state.Retained) != 0 || state.DealSize != 5 || state.TricksPerHand != 4 {
		t.Fatalf("fresh roster deal: seats=%d carry=%+v, %v", len(state.Seats), state.Carried, err)
	}
	for _, seat := range state.Seats {
		if len(state.Hands[seat.ID]) != 5 {
			t.Fatalf("%s did not get a full fresh deal", seat.ID)
		}
	}
}

func TestEndlessMilestonesContinueWithoutPhaseSwitch(t *testing.T) {
	catalog := DefaultCatalog()
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize = ModeEndless, 3
	state, err := NewMatch([]SeatID{"human", "bot:1"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	start := map[SeatID]int{"human": 0, "bot:1": 0}
	for state.Round < 7 {
		if strings.Contains(state.TableCue, "three-round game starts") {
			t.Fatal("phase-switch cue returned")
		}
		switch state.Phase {
		case PhasePlayingTrick:
			state = handAction(t, state, Action{Kind: ActionPlayCard, Seat: state.Turn, Card: legalCard(state, catalog)})
		case PhaseAwaitingChoice:
			state = handAction(t, state, Action{Kind: ActionChooseRule, Seat: "human", BoundaryID: state.PendingBoundaryID})
		case PhaseHandComplete:
			state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "human", DeckOrder: canonicalOrder()})
		default:
			t.Fatalf("unexpected phase %s", state.Phase)
		}
		if state.Round == 4 && state.RoundHand == 0 {
			if state.ShortGameResult == nil || state.ShortGameResult.Number != 1 || state.ShortGameRound != 1 {
				t.Fatalf("first milestone: %+v", state.ShortGameResult)
			}
			for id, score := range state.ShortGameResult.Scores {
				if score != state.RunScores[id]-start[id] {
					t.Fatalf("milestone score for %s: %d", id, score)
				}
			}
		}
	}
	if state.ShortGameResult == nil || state.ShortGameResult.Number != 2 || state.ShortGameRound != 1 || len(state.Seats) != 4 {
		t.Fatalf("second milestone or roster lost: %+v", state)
	}
	for _, seat := range state.Seats {
		if seat.Score != state.RunScores[seat.ID] {
			t.Fatalf("run score for %s lost", seat.ID)
		}
	}
}

func TestEndlessIllegalRosterChangeKeepsCurrentSeats(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize = ModeEndless, 18
	state, err := NewMatch([]SeatID{"human", "bot:1"}, cfg, DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	for state.Round == 1 {
		if state.Phase == PhasePlayingTrick {
			state = finishHand(t, state)
		} else if state.Phase == PhaseHandComplete {
			state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "human", DeckOrder: canonicalOrder()})
		} else {
			t.Fatalf("phase %s", state.Phase)
		}
	}
	state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "human", DeckOrder: canonicalOrder()})
	if len(state.Seats) != 2 || state.DealSize != 20 {
		t.Fatal("illegal three-seat deal changed roster")
	}
}
