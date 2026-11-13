package gameui

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"time"

	"github.com/n0remac/trick-evolution/domain"
)

type BotMode string

const (
	BotModeHeuristic BotMode = "heuristic"
	BotModeRollout   BotMode = "rollout"
)

type RolloutObjective string

const (
	ObjectiveMatchMargin RolloutObjective = "match_margin"
	ObjectiveHandPoints  RolloutObjective = "hand_points"
)

type RolloutBudget struct {
	SamplesPerCard int
	MaxSteps       int
	MaxSampleNodes int
	MaxWallTime    time.Duration
	Objective      RolloutObjective
}

func DefaultRolloutBudget() RolloutBudget {
	return RolloutBudget{SamplesPerCard: 8, MaxSteps: 1000, MaxSampleNodes: 3000, MaxWallTime: 80 * time.Millisecond, Objective: ObjectiveMatchMargin}
}

func (b RolloutBudget) validate() error {
	if b.SamplesPerCard < 1 || b.SamplesPerCard > 64 || b.MaxSteps < 1 || b.MaxSteps > 5000 ||
		b.MaxSampleNodes < 1 || b.MaxSampleNodes > 20000 || b.MaxWallTime < time.Millisecond || b.MaxWallTime > time.Second ||
		(b.Objective != ObjectiveMatchMargin && b.Objective != ObjectiveHandPoints) {
		return fmt.Errorf("rollout budget is outside safe bounds")
	}
	return nil
}

type BotOptions struct {
	Mode        BotMode
	Budget      RolloutBudget
	SeedFactory func() int64
}

type BotDecision struct {
	Card             domain.CardID
	Seed             int64
	CompletedSamples int
	Steps            int
	Reason           string
	Scores           []CandidateScore
}

type CandidateScore struct {
	Card    domain.CardID
	Sum     int
	Samples int
}

// decideRollout receives no live match state. Every candidate in a complete
// comparison round uses the same sampled world. Budget or cancellation paths
// return the deterministic A1 choice.
func decideRollout(ctx context.Context, observation BotObservation, catalog domain.Catalog, seed int64, budget RolloutBudget) (BotDecision, error) {
	fallback, err := chooseBotCard(observation.View, catalog)
	if err != nil {
		return BotDecision{}, err
	}
	decision := BotDecision{Card: fallback, Seed: seed, Reason: "heuristic_fallback"}
	if err := budget.validate(); err != nil {
		return decision, err
	}
	legal, err := domain.LegalCards(observation.View, catalog)
	if err != nil {
		return decision, err
	}
	if len(legal) == 1 {
		decision.Reason = "forced_card"
		return decision, nil
	}
	if ctx.Err() != nil {
		decision.Reason = "canceled"
		return decision, nil
	}
	ctx, cancel := context.WithTimeout(ctx, budget.MaxWallTime)
	defer cancel()
	prepared, err := prepareObservation(observation, catalog)
	if err != nil {
		decision.Reason = "invalid_observation"
		return decision, nil
	}
	sort.Slice(legal, func(i, j int) bool { return legal[i] < legal[j] })
	random := rand.New(rand.NewSource(seed))
	sums := make([]int, len(legal))
	completed := 0
	stopReason := "sample_limit"
	for round := 0; round < budget.SamplesPerCard; round++ {
		if ctx.Err() != nil {
			decision.Reason = "wall_or_room_canceled"
			return decision, nil
		}
		world, err := prepared.sampleWorld(ctx, random, budget.MaxSampleNodes, catalog)
		if err != nil {
			if ctx.Err() != nil {
				decision.Reason = "wall_or_room_canceled"
			} else {
				decision.Reason = "sample_failed"
			}
			return decision, nil
		}
		roundScores := make([]int, len(legal))
		complete := true
		for index, candidate := range legal {
			if ctx.Err() != nil {
				decision.Reason = "wall_or_room_canceled"
				return decision, nil
			}
			if decision.Steps >= budget.MaxSteps {
				complete = false
				stopReason = "step_budget"
				break
			}
			state, _, err := domain.Step(world, domain.Action{Kind: domain.ActionPlayCard, Seat: observation.View.Seat, Card: candidate}, catalog)
			decision.Steps++
			if err != nil {
				decision.Reason = "simulation_error"
				return decision, nil
			}
			for state.Phase == domain.PhasePlayingTrick {
				if ctx.Err() != nil {
					decision.Reason = "wall_or_room_canceled"
					return decision, nil
				}
				if decision.Steps >= budget.MaxSteps {
					complete = false
					stopReason = "step_budget"
					break
				}
				view, err := domain.ProjectForSeat(state, catalog, state.Turn)
				if err != nil {
					decision.Reason = "simulation_error"
					return decision, nil
				}
				card, err := chooseBotCard(view, catalog)
				if err != nil {
					decision.Reason = "simulation_error"
					return decision, nil
				}
				state, _, err = domain.Step(state, domain.Action{Kind: domain.ActionPlayCard, Seat: state.Turn, Card: card}, catalog)
				decision.Steps++
				if err != nil {
					decision.Reason = "simulation_error"
					return decision, nil
				}
			}
			if !complete {
				break
			}
			roundScores[index] = rolloutScore(state, observation.View, budget.Objective)
		}
		if !complete {
			break
		}
		for i, score := range roundScores {
			sums[i] += score
		}
		completed++
		decision.CompletedSamples = completed
		decision.Scores = decision.Scores[:0]
		for i, card := range legal {
			decision.Scores = append(decision.Scores, CandidateScore{Card: card, Sum: sums[i], Samples: completed})
		}
	}
	if ctx.Err() != nil {
		decision.Reason = "wall_or_room_canceled"
		return decision, nil
	}
	if completed == 0 {
		decision.Reason = stopReason
		return decision, nil
	}
	best, bestSum := fallback, -int(^uint(0)>>1)-1
	for i, card := range legal {
		if sums[i] > bestSum || (sums[i] == bestSum && card == fallback) {
			best, bestSum = card, sums[i]
		}
	}
	decision.Card, decision.Reason = best, stopReason
	return decision, nil
}

func rolloutScore(state domain.MatchState, initial domain.PlayerView, objective RolloutObjective) int {
	bot, otherBest, baseline := 0, -int(^uint(0)>>1)-1, 0
	for _, seat := range initial.Public.Seats {
		if seat.ID == initial.Seat {
			baseline = seat.Score
		}
	}
	for _, seat := range state.Seats {
		if seat.ID == initial.Seat {
			bot = seat.Score
		} else if seat.Score > otherBest {
			otherBest = seat.Score
		}
	}
	if objective == ObjectiveHandPoints {
		return bot - baseline
	}
	return bot - otherBest
}
