package gameui

import (
	"context"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/n0remac/trick-evolution/domain"
)

func optionalTrumpObservation(t *testing.T) (BotObservation, domain.MatchState, domain.Catalog) {
	t.Helper()
	catalog := domain.DefaultCatalog()
	state, err := domain.NewMatch([]domain.SeatID{"human", "bot"}, domain.DefaultConfig(), catalog, domain.SeededShuffle(55))
	if err != nil {
		t.Fatal(err)
	}
	state.Field = append(state.Field, domain.RuleInstance{InstanceID: "test:spades", DefinitionID: domain.RuleSpadesTrump, Sequence: len(state.Field)})
	state.Field = append(state.Field, domain.RuleInstance{InstanceID: "test:optional", DefinitionID: domain.RuleFollowOrTrump, Sequence: len(state.Field)})
	state.TrumpSuit = domain.Spades
	state.Hands["bot"] = []domain.CardID{"std:H:02", "std:H:03", "std:H:04", "std:C:04", "std:S:14"}
	// Human demonstrably holds hearts but may trump a heart lead.
	state.Hands["human"] = []domain.CardID{"std:H:07", "std:S:02", "std:S:03", "std:C:05", "std:D:05"}
	used := map[domain.CardID]bool{}
	for _, hand := range state.Hands {
		for _, id := range hand {
			used[id] = true
		}
	}
	state.StandardDraw = nil
	for _, card := range domain.StandardDeck() {
		if !used[card.ID] {
			state.StandardDraw = append(state.StandardDraw, card.ID)
		}
	}
	state.RuleDraw = nil
	for _, id := range catalog.EvolutionDeck(domain.ModeFinite) {
		if id != domain.RuleSpadesTrump && id != domain.RuleFollowOrTrump {
			state.RuleDraw = append(state.RuleDraw, id)
		}
	}
	room := &room{server: &Server{catalog: catalog}}
	for _, action := range []domain.Action{{Kind: domain.ActionPlayCard, Seat: "bot", Card: "std:H:02"}, {Kind: domain.ActionPlayCard, Seat: "human", Card: "std:S:02"}, {Kind: domain.ActionPlayCard, Seat: "human", Card: "std:H:07"}, {Kind: domain.ActionPlayCard, Seat: "bot", Card: "std:S:14"}} {
		next, events, err := domain.Step(state, action, catalog)
		if err != nil {
			t.Fatal(err)
		}
		room.recordPublicEventsLocked(&state, events)
		state = next
	}
	room.state = &state
	obs, err := room.botObservationLocked("bot")
	if err != nil {
		t.Fatal(err)
	}
	return obs, state, catalog
}

func TestOptionalTrumpDoesNotInventVoidAndCarriesFacts(t *testing.T) {
	obs, real, catalog := optionalTrumpObservation(t)
	if obs.Played[1].PlayRestriction != domain.ValueFollowOrTrump || obs.Played[1].TrumpSuit != domain.Spades {
		t.Fatal(obs.Played)
	}
	prepared, err := prepareObservation(obs, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.voids["human"][domain.Hearts] {
		t.Fatal("legal trump falsely proved a heart void")
	}
	world, err := prepared.sampleWorld(context.Background(), rand.New(rand.NewSource(7)), 3000, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if world.TrumpSuit != real.TrumpSuit || world.BrokenSpades != real.BrokenSpades || world.BrokenHearts != real.BrokenHearts || !reflect.DeepEqual(world.CurrentTrick, real.CurrentTrick) {
		t.Fatalf("public hand facts diverged: %+v %+v", world, real)
	}
	// Compare continuations for the same fully specified hypothetical world.
	// The unknown hands are supplied from the real fixture only in this test;
	// production sampling never receives them.
	world.Hands["human"] = append([]domain.CardID(nil), real.Hands["human"]...)
	world.StandardDraw = append([]domain.CardID(nil), real.StandardDraw...)
	world.RuleDraw = append([]domain.RuleID(nil), real.RuleDraw...)
	for real.Phase == domain.PhasePlayingTrick {
		viewReal, err := domain.ProjectForSeat(real, catalog, real.Turn)
		if err != nil {
			t.Fatal(err)
		}
		viewWorld, err := domain.ProjectForSeat(world, catalog, world.Turn)
		if err != nil {
			t.Fatal(err)
		}
		legalReal, err := domain.LegalCards(viewReal, catalog)
		if err != nil {
			t.Fatal(err)
		}
		legalWorld, err := domain.LegalCards(viewWorld, catalog)
		if err != nil || !reflect.DeepEqual(legalReal, legalWorld) {
			t.Fatalf("legal divergence %v %v %v", legalReal, legalWorld, err)
		}
		action := domain.Action{Kind: domain.ActionPlayCard, Seat: real.Turn, Card: legalReal[0]}
		var eventsReal, eventsWorld []domain.Event
		real, eventsReal, err = domain.Step(real, action, catalog)
		if err != nil {
			t.Fatal(err)
		}
		world, eventsWorld, err = domain.Step(world, action, catalog)
		if err != nil || !reflect.DeepEqual(eventsReal, eventsWorld) || real.TrumpSuit != world.TrumpSuit || real.BrokenSpades != world.BrokenSpades || real.BrokenHearts != world.BrokenHearts || !reflect.DeepEqual(real.Seats, world.Seats) {
			t.Fatalf("engine/sample divergence: %+v %+v %v", real, world, err)
		}
	}
}

func TestHeuristicRecognizesTrumpAndPitchSuit(t *testing.T) {
	obs, _, catalog := optionalTrumpObservation(t)
	// Bot won the first trick and now leads; spade trump beats ordinary leads.
	chosen, err := chooseBotCard(obs.View, catalog)
	if err != nil {
		t.Fatal(err)
	}
	legal, err := domain.LegalCards(obs.View, catalog)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range legal {
		if id == chosen {
			found = true
		}
	}
	if !found {
		t.Fatalf("heuristic chose illegal card %s", chosen)
	}
	view := obs.View
	view.Public.CurrentTrick = []domain.Play{{Seat: "human", Card: "std:H:14"}}
	view.Public.Turn = "bot"
	view.Hand = []domain.CardID{"std:S:03", "std:C:05"}
	chosen, err = chooseBotCard(view, catalog)
	if err != nil || chosen != "std:S:03" {
		t.Fatalf("failed to recognize trump winner: %s %v", chosen, err)
	}
}

func TestProductionBudgetReportsCompletedSamplesOrFallback(t *testing.T) {
	fixed, _, catalog := optionalTrumpObservation(t)
	pitch := pitchUnsetObservation(t, catalog)
	budget := DefaultRolloutBudget()
	for _, tc := range []struct {
		name        string
		observation BotObservation
	}{{"fixed and broken", fixed}, {"pitch unset", pitch}} {
		t.Run(tc.name, func(t *testing.T) {
			counts := map[string]int{}
			completed := 0
			for seed := int64(1); seed <= 3; seed++ {
				decision, err := decideRollout(context.Background(), tc.observation, catalog, seed, budget)
				if err != nil {
					t.Fatal(err)
				}
				counts[decision.Reason]++
				completed += decision.CompletedSamples
				if decision.Steps > budget.MaxSteps || decision.CompletedSamples > budget.SamplesPerCard {
					t.Fatalf("budget exceeded %+v", decision)
				}
				if decision.CompletedSamples > 0 && (len(decision.Scores) == 0 || decision.Reason != "wall_budget" && decision.Reason != "sample_limit") {
					t.Fatalf("completed rounds were discarded: %+v", decision)
				}
			}
			t.Logf("default budget three seeded decisions: reasons=%v completed_rounds=%d of 24", counts, completed)
			if counts["invalid_observation"] != 0 || counts["simulation_error"] != 0 || counts["sample_failed"] != 0 {
				t.Fatal(counts)
			}
		})
	}
}

func TestContemporaneousObservationSurvivesRuleReplacement(t *testing.T) {
	obs, state, catalog := optionalTrumpObservation(t)
	const strictAgain domain.RuleID = "test.strict_again"
	catalog.Definitions = append(catalog.Definitions, domain.RuleDefinition{ID: strictAgain, Title: "Strict Again", Text: "Follow the lead suit.", Category: "rule", Status: domain.StatusSupported, FieldCategory: domain.FieldPlay, Effects: []domain.TypedEffect{{Mechanic: domain.MechanicPlayRestriction, Value: domain.ValueFollow}}, Overrides: []domain.RuleID{domain.RuleFollowOrTrump}, StartingVisibility: domain.NotStarting})
	state.Field = append(state.Field, domain.RuleInstance{InstanceID: "test:strict", DefinitionID: strictAgain, Sequence: len(state.Field)})
	view, err := domain.ProjectForSeat(state, catalog, "bot")
	if err != nil {
		t.Fatal(err)
	}
	obs.View = view
	// The current restriction is strict, but the earlier legal trump does not
	// prove that the opponent lacked hearts at the time of that play.
	prepared, err := prepareObservation(obs, catalog)
	if err != nil || prepared.voids["human"][domain.Hearts] {
		t.Fatalf("historical context lost: %v %+v", err, prepared.voids)
	}
}

func observedOptionalTrick(t *testing.T, lead, off domain.CardID, botHand, humanHand []domain.CardID) BotObservation {
	t.Helper()
	catalog := domain.DefaultCatalog()
	state, err := domain.NewMatch([]domain.SeatID{"human", "bot"}, domain.DefaultConfig(), catalog, domain.SeededShuffle(56))
	if err != nil {
		t.Fatal(err)
	}
	state.Field = append(state.Field, domain.RuleInstance{InstanceID: "test:fixed", DefinitionID: domain.RuleSpadesTrump, Sequence: len(state.Field)})
	state.Field = append(state.Field, domain.RuleInstance{InstanceID: "test:optional", DefinitionID: domain.RuleFollowOrTrump, Sequence: len(state.Field)})
	state.TrumpSuit = domain.Spades
	state.Hands["bot"], state.Hands["human"] = botHand, humanHand
	used := map[domain.CardID]bool{}
	for _, hand := range state.Hands {
		for _, id := range hand {
			used[id] = true
		}
	}
	state.StandardDraw = nil
	for _, card := range domain.StandardDeck() {
		if !used[card.ID] {
			state.StandardDraw = append(state.StandardDraw, card.ID)
		}
	}
	state.RuleDraw = nil
	for _, id := range catalog.EvolutionDeck(domain.ModeFinite) {
		if id != domain.RuleSpadesTrump && id != domain.RuleFollowOrTrump {
			state.RuleDraw = append(state.RuleDraw, id)
		}
	}
	room := &room{server: &Server{catalog: catalog}}
	for _, action := range []domain.Action{{Kind: domain.ActionPlayCard, Seat: "bot", Card: lead}, {Kind: domain.ActionPlayCard, Seat: "human", Card: off}} {
		next, events, err := domain.Step(state, action, catalog)
		if err != nil {
			t.Fatal(err)
		}
		room.recordPublicEventsLocked(&state, events)
		state = next
	}
	room.state = &state
	observation, err := room.botObservationLocked("bot")
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func TestOptionalTrumpVoidDeductions(t *testing.T) {
	cases := []struct {
		name       string
		lead, off  domain.CardID
		bot, human []domain.CardID
		want       domain.Suit
	}{
		{"off suit nontrump", "std:H:14", "std:C:05", []domain.CardID{"std:H:14", "std:H:03", "std:D:04", "std:S:13", "std:C:04"}, []domain.CardID{"std:C:05", "std:C:06", "std:D:05", "std:S:02", "std:S:03"}, domain.Hearts},
		{"trump lead not followed", "std:S:14", "std:C:05", []domain.CardID{"std:S:14", "std:H:03", "std:D:04", "std:S:13", "std:C:04"}, []domain.CardID{"std:C:05", "std:C:06", "std:D:05", "std:H:02", "std:H:04"}, domain.Spades},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			observation := observedOptionalTrick(t, tc.lead, tc.off, tc.bot, tc.human)
			prepared, err := prepareObservation(observation, domain.DefaultCatalog())
			if err != nil {
				t.Fatal(err)
			}
			if !prepared.voids["human"][tc.want] {
				t.Fatalf("missing public void inference for %s", tc.want)
			}
			world, err := prepared.sampleWorld(context.Background(), rand.New(rand.NewSource(3)), 3000, domain.DefaultCatalog())
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range world.Hands["human"] {
				card, _ := domain.CardDetails(id)
				if card.Suit == tc.want {
					t.Fatalf("sample assigned forbidden %s to void seat", id)
				}
			}
		})
	}
}

func TestPitchOpeningHeuristicAndContinuation(t *testing.T) {
	catalog := domain.DefaultCatalog()
	state, err := domain.NewMatch([]domain.SeatID{"human", "bot"}, domain.DefaultConfig(), catalog, domain.SeededShuffle(57))
	if err != nil {
		t.Fatal(err)
	}
	state.Field = append(state.Field, domain.RuleInstance{InstanceID: "test:pitch", DefinitionID: domain.RulePitchTrump, Sequence: len(state.Field)})
	state.Field = append(state.Field, domain.RuleInstance{InstanceID: "test:optional", DefinitionID: domain.RuleFollowOrTrump, Sequence: len(state.Field)})
	state.Hands["bot"] = []domain.CardID{"std:S:02", "std:S:03", "std:H:04", "std:C:04", "std:D:04"}
	state.Hands["human"] = []domain.CardID{"std:S:14", "std:H:14", "std:C:02", "std:D:02", "std:H:03"}
	used := map[domain.CardID]bool{}
	for _, hand := range state.Hands {
		for _, id := range hand {
			used[id] = true
		}
	}
	state.StandardDraw = nil
	for _, card := range domain.StandardDeck() {
		if !used[card.ID] {
			state.StandardDraw = append(state.StandardDraw, card.ID)
		}
	}
	state.RuleDraw = nil
	for _, id := range catalog.EvolutionDeck(domain.ModeFinite) {
		if id != domain.RulePitchTrump && id != domain.RuleFollowOrTrump {
			state.RuleDraw = append(state.RuleDraw, id)
		}
	}
	view, err := domain.ProjectForSeat(state, catalog, "bot")
	if err != nil {
		t.Fatal(err)
	}
	card, err := chooseBotCard(view, catalog)
	if err != nil || card != "std:S:03" || view.Public.TrumpSuit != "" {
		t.Fatalf("pitch opening choice %s, unset=%s: %v", card, view.Public.TrumpSuit, err)
	}
	state, events, err := domain.Step(state, domain.Action{Kind: domain.ActionPlayCard, Seat: "bot", Card: card}, catalog)
	if err != nil || state.TrumpSuit != domain.Spades || events[0].TrumpSuit != domain.Spades {
		t.Fatalf("first lead did not select trump: %+v %+v %v", state, events, err)
	}
	state, _, err = domain.Step(state, domain.Action{Kind: domain.ActionPlayCard, Seat: "human", Card: "std:S:14"}, catalog)
	if err != nil || state.Seats[0].Score != 1 {
		t.Fatalf("first trick did not use pitched trump: %+v %v", state.Seats, err)
	}
	state, _, err = domain.Step(state, domain.Action{Kind: domain.ActionPlayCard, Seat: "human", Card: "std:H:14"}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	view, err = domain.ProjectForSeat(state, catalog, "bot")
	if err != nil {
		t.Fatal(err)
	}
	legal, err := domain.LegalCards(view, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(legal, []domain.CardID{"std:S:02", "std:H:04"}) {
		t.Fatalf("optional pitched trump legality %v", legal)
	}
	card, err = chooseBotCard(view, catalog)
	if err != nil || card != "std:S:02" {
		t.Fatalf("bot missed pitched trump winner: %s %v", card, err)
	}
	state, _, err = domain.Step(state, domain.Action{Kind: domain.ActionPlayCard, Seat: "bot", Card: card}, catalog)
	if err != nil || state.Seats[1].Score != 1 || !state.BrokenSpades {
		t.Fatalf("pitched continuation: %+v %v", state, err)
	}
}

func pitchUnsetObservation(t *testing.T, catalog domain.Catalog) BotObservation {
	t.Helper()
	cfg := domain.Config{HandSize: 3, RevealEvery: 3, MaxHands: 4}
	pitchState, err := domain.NewMatch([]domain.SeatID{"human", "bot"}, cfg, catalog, domain.SeededShuffle(58))
	if err != nil {
		t.Fatal(err)
	}
	pitchState.Field = append(pitchState.Field, domain.RuleInstance{InstanceID: "test:pitch", DefinitionID: domain.RulePitchTrump, Sequence: len(pitchState.Field)})
	pitchState.Field = append(pitchState.Field, domain.RuleInstance{InstanceID: "test:optional", DefinitionID: domain.RuleFollowOrTrump, Sequence: len(pitchState.Field)})
	pitchState.RuleDraw = nil
	for _, id := range catalog.EvolutionDeck(domain.ModeFinite) {
		if id != domain.RulePitchTrump && id != domain.RuleFollowOrTrump {
			pitchState.RuleDraw = append(pitchState.RuleDraw, id)
		}
	}
	pitchView, err := domain.ProjectForSeat(pitchState, catalog, "bot")
	if err != nil {
		t.Fatal(err)
	}
	return BotObservation{View: pitchView}
}

func TestSeededTrumpRolloutsCompleteWithTestBudget(t *testing.T) {
	fixed, _, catalog := optionalTrumpObservation(t)
	pitch := pitchUnsetObservation(t, catalog)
	budget := RolloutBudget{SamplesPerCard: 2, MaxSteps: 500, MaxSampleNodes: 3000, MaxWallTime: time.Second, Objective: ObjectiveMatchMargin}
	for _, tc := range []struct {
		name string
		obs  BotObservation
		want []CandidateScore
	}{
		{"fixed and broken", fixed, []CandidateScore{{Card: "std:C:04", Sum: -6, Samples: 2}, {Card: "std:H:03", Sum: -6, Samples: 2}, {Card: "std:H:04", Sum: -6, Samples: 2}}},
		{"pitch unset", pitch, []CandidateScore{{Card: "std:C:10", Sum: 6, Samples: 2}, {Card: "std:C:12", Sum: 6, Samples: 2}, {Card: "std:S:05", Sum: 2, Samples: 2}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decision, err := decideRollout(context.Background(), tc.obs, catalog, 19, budget)
			if err != nil || decision.CompletedSamples != 2 || !reflect.DeepEqual(decision.Scores, tc.want) || decision.Reason != "sample_limit" {
				t.Fatalf("no complete evaluated sample: %+v %v", decision, err)
			}
			for _, score := range decision.Scores {
				if score.Samples != decision.CompletedSamples {
					t.Fatalf("candidate score omitted: %+v", decision)
				}
			}
		})
	}
}
