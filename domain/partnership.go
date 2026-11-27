package domain

import "fmt"

// startHandRules freezes the scoring roster and reveals only the turn-up card.
// Personal ledgers are never pooled when a partnership starts or ends.
func startHandRules(state *MatchState, rules RuleResolution) error {
	owner, err := rules.Effect(MechanicScoreOwnership)
	if err != nil {
		return err
	}
	if owner.Value == ValuePartnership {
		if len(state.Seats) != 4 {
			return fmt.Errorf("partnerships require four seats")
		}
		if len(state.HandTeams) == 0 {
			state.TeamScores = map[int]int{0: 0, 1: 0}
		}
		state.HandTeams = make(map[SeatID]int, 4)
		for i, seat := range state.Seats {
			state.HandTeams[seat.ID] = i % 2
		}
	} else {
		state.HandTeams = nil
		state.TeamScores = nil
	}
	state.TurnedTrumpCard = ""
	if trump, err := rules.Effect(MechanicTrump); err == nil && trump.Value == ValueTurnUpTrump {
		hand := state.Hands[state.Dealer]
		if len(hand) == 0 {
			return fmt.Errorf("dealer has no turn-up card")
		}
		state.TurnedTrumpCard = hand[len(hand)-1]
		card, ok := cardByID(state.TurnedTrumpCard)
		if !ok || state.DealOwner[card.ID] != state.Dealer {
			return fmt.Errorf("invalid turn-up card")
		}
		state.TrumpSuit = card.Suit
	}
	return nil
}

func awardPoints(state *MatchState, award HandAward) {
	for i := range state.Seats {
		if state.Seats[i].ID == award.Seat {
			state.Seats[i].Score += award.Points
			if team, ok := state.HandTeams[award.Seat]; ok {
				if state.TeamScores == nil {
					state.TeamScores = make(map[int]int, 2)
				}
				state.TeamScores[team] += award.Points
			}
			state.HandAwards = append(state.HandAwards, award)
			return
		}
	}
}

func matchWinners(state MatchState) []SeatID {
	if len(state.HandTeams) == 0 {
		return gameWinners(state.Seats)
	}
	best := state.TeamScores[0]
	if state.TeamScores[1] > best {
		best = state.TeamScores[1]
	}
	var winners []SeatID
	for _, seat := range state.Seats {
		if state.TeamScores[state.HandTeams[seat.ID]] == best {
			winners = append(winners, seat.ID)
		}
	}
	return winners
}
