package gameui

import (
	"context"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/n0remac/trick-evolution/domain"
)

func initialBotObservation(t *testing.T, seats int) (BotObservation, domain.MatchState, domain.Catalog) {
	t.Helper()
	catalog := domain.DefaultCatalog()
	ids := []domain.SeatID{"human", "bot:1", "bot:2", "bot:3"}[:seats]
	state, err := domain.NewMatch(ids, domain.DefaultConfig(), catalog, domain.SeededShuffle(2401))
	if err != nil {
		t.Fatal(err)
	}
	view, err := domain.ProjectForSeat(state, catalog, "bot:1")
	if err != nil {
		t.Fatal(err)
	}
	return BotObservation{View: view}, state, catalog
}

func TestRolloutSamplingAndReplay(t *testing.T) {
	for _, seats := range []int{2, 4} {
		t.Run(string(rune('0'+seats)), func(t *testing.T) {
			observation, real, catalog := initialBotObservation(t, seats)
			prepared, err := prepareObservation(observation, catalog)
			if err != nil {
				t.Fatal(err)
			}
			world, err := prepared.sampleWorld(context.Background(), rand.New(rand.NewSource(713)), 3000, catalog)
			if err != nil {
				t.Fatal(err)
			}
			seen := make(map[domain.CardID]bool)
			for _, hand := range world.Hands {
				for _, card := range hand {
					if seen[card] {
						t.Fatalf("duplicate sampled card %s", card)
					}
					seen[card] = true
				}
			}
			for _, card := range world.StandardDraw {
				if seen[card] {
					t.Fatalf("duplicate draw card %s", card)
				}
				seen[card] = true
			}
			if len(seen) != 52 || !reflect.DeepEqual(world.Hands["bot:1"], observation.View.Hand) {
				t.Fatal("sample did not conserve cards or bot hand")
			}
			// Only the projected view is passed to A2. Changing true hidden cards
			// behind that projection cannot change its seeded decision.
			real.Hands["human"][0], real.StandardDraw[0] = real.StandardDraw[0], real.Hands["human"][0]
			otherView, err := domain.ProjectForSeat(real, catalog, "bot:1")
			if err != nil || !reflect.DeepEqual(otherView, observation.View) {
				t.Fatal("hidden change altered public observation")
			}
			budget := RolloutBudget{SamplesPerCard: 2, MaxSteps: 1000, MaxSampleNodes: 3000, MaxWallTime: time.Second, Objective: ObjectiveMatchMargin}
			first, err := decideRollout(context.Background(), observation, catalog, 713, budget)
			if err != nil {
				t.Fatal(err)
			}
			second, err := decideRollout(context.Background(), BotObservation{View: otherView}, catalog, 713, budget)
			if err != nil || !reflect.DeepEqual(first, second) {
				t.Fatalf("replay mismatch: %+v vs %+v: %v", first, second, err)
			}
			if first.CompletedSamples == 0 || first.CompletedSamples > 2 {
				t.Fatalf("no complete sample: %+v", first)
			}
			if len(first.Scores) == 0 {
				t.Fatal("candidate scores were not recorded")
			}
			budget.Objective = ObjectiveHandPoints
			handDecision, err := decideRollout(context.Background(), observation, catalog, 713, budget)
			if err != nil || handDecision.CompletedSamples == 0 {
				t.Fatalf("hand-points objective did not complete: %+v %v", handDecision, err)
			}
			legal, err := domain.LegalCards(observation.View, catalog)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, card := range legal {
				if card == first.Card {
					found = true
				}
			}
			if !found {
				t.Fatalf("rollout picked illegal card %s", first.Card)
			}
		})
	}
}

func TestRolloutBoundedFallbacks(t *testing.T) {
	observation, _, catalog := initialBotObservation(t, 2)
	fallback, err := chooseBotCard(observation.View, catalog)
	if err != nil {
		t.Fatal(err)
	}
	budget := RolloutBudget{SamplesPerCard: 2, MaxSteps: 1, MaxSampleNodes: 3000, MaxWallTime: time.Second, Objective: ObjectiveHandPoints}
	decision, err := decideRollout(context.Background(), observation, catalog, 41, budget)
	if err != nil || decision.Card != fallback || decision.CompletedSamples != 0 || decision.Reason != "step_budget" {
		t.Fatalf("step fallback: %+v %v", decision, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	decision, err = decideRollout(ctx, observation, catalog, 41, DefaultRolloutBudget())
	if err != nil || decision.Card != fallback || decision.Reason != "canceled" {
		t.Fatalf("cancel fallback: %+v %v", decision, err)
	}
	observation.View.Public.Seats[0].HandCount++
	decision, err = decideRollout(context.Background(), observation, catalog, 41, DefaultRolloutBudget())
	if err != nil || decision.Card != fallback || decision.Reason != "invalid_observation" {
		t.Fatalf("invalid observation fallback: %+v %v", decision, err)
	}
}

func TestRolloutBudgetValidationAndDefault(t *testing.T) {
	s, err := NewServerWithConfig(domain.DefaultConfig(), func() domain.ShuffleSource { return domain.SeededShuffle(1) })
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.botOptions.Mode != BotModeRollout {
		t.Fatal("production constructor did not select rollout bots")
	}
	bad := DefaultRolloutBudget()
	bad.SamplesPerCard = 65
	if _, err := NewServerWithBotOptions(domain.DefaultConfig(), func() domain.ShuffleSource { return domain.SeededShuffle(1) }, BotOptions{Mode: BotModeRollout, Budget: bad}); err == nil {
		t.Fatal("oversized rollout budget accepted")
	}
}

func TestVoidInferenceRespectsNoFollowOverride(t *testing.T) {
	for _, noFollow := range []bool{false, true} {
		observation := completedOffSuitObservation(t, noFollow)
		prepared, err := prepareObservation(observation, domain.DefaultCatalog())
		if err != nil {
			t.Fatal(err)
		}
		if prepared.voids["human"][domain.Hearts] == noFollow {
			t.Fatalf("wrong void inference with noFollow=%v", noFollow)
		}
		world, err := prepared.sampleWorld(context.Background(), rand.New(rand.NewSource(5)), 3000, domain.DefaultCatalog())
		if err != nil {
			t.Fatal(err)
		}
		if !noFollow {
			for _, id := range world.Hands["human"] {
				card, _ := domain.CardDetails(id)
				if card.Suit == domain.Hearts {
					t.Fatal("void seat received heart")
				}
			}
		}
	}
	observation := completedOffSuitObservation(t, false)
	prepared, err := prepareObservation(observation, domain.DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	prepared.voids["human"] = map[domain.Suit]bool{domain.Hearts: true, domain.Diamonds: true, domain.Clubs: true, domain.Spades: true}
	if _, err := prepared.sampleWorld(context.Background(), rand.New(rand.NewSource(5)), 3000, domain.DefaultCatalog()); err == nil {
		t.Fatal("impossible void sample succeeded")
	}
}

func completedOffSuitObservation(t *testing.T, noFollow bool) BotObservation {
	t.Helper()
	catalog := domain.DefaultCatalog()
	state, err := domain.NewMatch([]domain.SeatID{"human", "bot"}, domain.DefaultConfig(), catalog, domain.SeededShuffle(9))
	if err != nil {
		t.Fatal(err)
	}
	state.Hands["bot"] = []domain.CardID{"std:H:02", "std:H:03", "std:H:04", "std:H:05", "std:H:06"}
	state.Hands["human"] = []domain.CardID{"std:D:02", "std:D:03", "std:D:04", "std:D:05", "std:D:06"}
	if noFollow {
		state.Hands["human"][4] = "std:H:07"
	}
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
	state.Field = append(state.Field, domain.RuleInstance{InstanceID: "rule:follow", DefinitionID: domain.RuleFollowSuit, Sequence: 1})
	if noFollow {
		state.Field = append(state.Field, domain.RuleInstance{InstanceID: "rule:nofollow", DefinitionID: domain.RuleNoFollowRequired, Sequence: 2})
	}
	state.RuleDraw = nil
	for _, rule := range catalog.RuleDeck() {
		if rule != domain.RuleFollowSuit && (!noFollow || rule != domain.RuleNoFollowRequired) {
			state.RuleDraw = append(state.RuleDraw, rule)
		}
	}
	r := &room{server: &Server{catalog: catalog}}
	for _, action := range []domain.Action{{Kind: domain.ActionPlayCard, Seat: "bot", Card: "std:H:02"}, {Kind: domain.ActionPlayCard, Seat: "human", Card: "std:D:02"}} {
		next, events, err := domain.Step(state, action, catalog)
		if err != nil {
			t.Fatal(err)
		}
		r.recordPublicEventsLocked(&state, events)
		state = next
	}
	r.state = &state
	observation, err := r.botObservationLocked("bot")
	if err != nil {
		t.Fatal(err)
	}
	return observation
}
