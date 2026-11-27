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
		changed := len(state.HandTeams) != 4
		for i, seat := range state.Seats {
			if team, ok := state.HandTeams[seat.ID]; !ok || team != i%2 {
				changed = true
			}
		}
		if changed {
			state.TeamScores = map[int]int{0: 0, 1: 0}
			state.TeamAwardBalances = make(map[SeatID]int, 4)
		}
		state.HandTeams = make(map[SeatID]int, 4)
		for i, seat := range state.Seats {
			state.HandTeams[seat.ID] = i % 2
		}
	} else {
		state.HandTeams = nil
		state.TeamScores = nil
		state.TeamAwardBalances = nil
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
				if award.TeamWide {
					if state.TeamAwardBalances == nil {
						state.TeamAwardBalances = make(map[SeatID]int, 4)
					}
					state.TeamAwardBalances[award.Seat] += award.Points
				}
			}
			state.HandAwards = append(state.HandAwards, award)
			return
		}
	}
}

// Attribute a team-wide award to the member with less of its shared awards.
// Deal parity breaks ties so equal balances do not always favor the first seat.
func teamAwardSeat(state *MatchState, team int) SeatID {
	var members []SeatID
	for _, seat := range state.Seats {
		if state.HandTeams[seat.ID] == team {
			members = append(members, seat.ID)
		}
	}
	if len(members) != 2 {
		return ""
	}
	a, b := members[0], members[1]
	if state.TeamAwardBalances[a] < state.TeamAwardBalances[b] {
		return a
	}
	if state.TeamAwardBalances[b] < state.TeamAwardBalances[a] {
		return b
	}
	return members[state.HandNumber%2]
}

func matchWinners(state MatchState) []SeatID {
	if state.Phase == PhaseGameComplete && !state.EndedEarly && state.GameResult != nil && state.GameResult.Hand == state.HandNumber {
		return append([]SeatID(nil), state.GameResult.Winners...)
	}
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
