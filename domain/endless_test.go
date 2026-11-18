package domain

import (
	"strings"
	"testing"
)

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
				choice = RuleLastTrickBonus
				if len(state.DraftOptions) != 3 || state.DraftOptions[0] != choice || state.DraftOptions[1] != RuleAvoidHearts || state.DraftOptions[2] != RuleExpandedHand {
					t.Fatalf("first options: %v", state.DraftOptions)
				}
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
	if state.ShortGameResult == nil || state.ShortGameResult.Number != 1 || state.ShortGameRound != 0 {
		t.Fatalf("missing short-game result: %+v", state.ShortGameResult)
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
	if state.Phase != PhaseAwaitingChoice {
		t.Fatal("third hand did not wait for draft")
	}
	state, _, err = Step(state, Action{Kind: ActionChooseRule, Seat: "human", Rule: RuleLastTrickBonus, BoundaryID: state.PendingBoundaryID}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	state, _, err = Step(state, Action{Kind: ActionStartHand, Seat: "human", DeckOrder: canonicalOrder()}, catalog)
	if err != nil || len(state.Seats) != 3 || len(state.Carried) != 0 || len(state.Retained) != 0 || state.DealSize != 3 || state.TricksPerHand != 2 {
		t.Fatalf("fresh roster deal: seats=%d carry=%+v, %v", len(state.Seats), state.Carried, err)
	}
	for _, seat := range state.Seats {
		if len(state.Hands[seat.ID]) != 3 {
			t.Fatalf("%s did not get a full fresh deal", seat.ID)
		}
	}
}

func TestEndlessShortGameTimelineAndScores(t *testing.T) {
	catalog := DefaultCatalog()
	cfg := DefaultConfig()
	cfg.Mode, cfg.HandSize = ModeEndless, 3
	state, err := NewMatch([]SeatID{"human", "bot:1"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	var gameStart map[SeatID]int
	seenNotice, seenResult := false, false
	for state.Round < 7 || state.Round == 7 && state.RoundHand == 0 {
		if state.Round == 3 {
			if !strings.Contains(state.TableCue, "A three-round game starts after this round") {
				t.Fatal("last open round lost announcement")
			}
			seenNotice = true
		}
		if state.Round >= 4 && state.Round <= 6 {
			if len(state.Seats) != 4 && state.RoundHand > 0 {
				t.Fatalf("short game roster changed: round %d seats %d", state.Round, len(state.Seats))
			}
			if state.ShortGameRound != state.Round-3 {
				t.Fatalf("short game progress %d in round %d", state.ShortGameRound, state.Round)
			}
			if gameStart == nil {
				gameStart = make(map[SeatID]int)
				for id, score := range state.RunScores {
					gameStart[id] = score
				}
			}
		}
		switch state.Phase {
		case PhasePlayingTrick:
			state = handAction(t, state, Action{Kind: ActionPlayCard, Seat: state.Turn, Card: legalCard(state, catalog)})
		case PhaseAwaitingChoice:
			state = handAction(t, state, Action{Kind: ActionChooseRule, Seat: "human", BoundaryID: state.PendingBoundaryID})
		case PhaseHandComplete:
			state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "human", DeckOrder: canonicalOrder()})
			switch state.Round {
			case 2:
				if len(state.Seats) != 3 {
					t.Fatal("round 2 must have three seats")
				}
			case 3:
				if len(state.Seats) != 4 {
					t.Fatal("round 3 must have four seats")
				}
			}
		default:
			t.Fatalf("unexpected phase %s", state.Phase)
		}
		if state.Round == 7 && state.ShortGameResult != nil {
			seenResult = true
		}
	}
	if !seenNotice || !seenResult || state.ShortGameResult == nil || state.ShortGameResult.Number != 1 {
		t.Fatal("timeline did not reach short-game result")
	}
	result := state.ShortGameResult
	for id, points := range result.Scores {
		if points != state.RunScores[id]-gameStart[id] {
			t.Fatalf("game points for %s = %d, run delta %d", id, points, state.RunScores[id]-gameStart[id])
		}
	}
	best := -1
	for _, points := range result.Scores {
		if points > best {
			best = points
		}
	}
	for _, id := range result.Winners {
		if result.Scores[id] != best {
			t.Fatalf("nonwinner %s reported as winner", id)
		}
	}
	if len(state.Seats) != 2 || state.ShortGameResult == nil || state.ShortGameRound != 0 || !strings.Contains(state.TableCue, "fresh deal") {
		t.Fatalf("result or open play lost: %+v", state)
	}
	for _, seat := range state.Seats {
		if seat.Score != state.RunScores[seat.ID] {
			t.Fatal("run score lost after roster reduction")
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
	if state.Phase == PhaseAwaitingChoice {
		state = handAction(t, state, Action{Kind: ActionChooseRule, Seat: "human", BoundaryID: state.PendingBoundaryID})
	}
	state = handAction(t, state, Action{Kind: ActionStartHand, Seat: "human", DeckOrder: canonicalOrder()})
	if len(state.Seats) != 2 || state.DealSize != 18 {
		t.Fatal("illegal three-seat deal changed roster")
	}
}
