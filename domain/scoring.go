package domain

import "fmt"

// HandAward records settled points. Completed-trick awards are projected at
// early game end; completed-hand awards are projected after hand settlement.
type HandAward struct {
	Rule       RuleID
	Title      string
	Seat       SeatID
	Card       CardID
	CardLabel  string // Public historical scorecards use a label, never a reusable deck ID.
	HiddenCard bool   // Public projection redacts a card still retained in a hand.
	Trick      int
	Tally      int
	Points     int
}

func gameCardValue(rank uint8) int {
	switch rank {
	case 10:
		return 10
	case 14:
		return 4
	case 13:
		return 3
	case 12:
		return 2
	case 11:
		return 1
	}
	return 0
}

// scoreCapturedTrick applies only public captured-card penalties. It runs when
// the trick is complete, so an early game end keeps these earned scores.
func scoreCapturedTrick(state *MatchState, rules RuleResolution, winnerIndex int) error {
	_, queenActive := rules.Effect(MechanicAvoidQueenHearts)
	_, heartsActive := rules.Effect(MechanicAvoidHearts)
	for _, play := range state.CurrentTrick {
		card, ok := cardByID(play.Card)
		if !ok {
			return fmt.Errorf("unknown captured card %s", play.Card)
		}
		if card.Suit != Hearts {
			continue
		}
		if card.Rank == 12 && queenActive == nil {
			state.Seats[winnerIndex].Score -= 5
			state.HandAwards = append(state.HandAwards, HandAward{Rule: RuleAvoidQueenHearts, Title: "Avoid Queen of Hearts", Seat: state.Seats[winnerIndex].ID, Card: card.ID, Trick: state.TrickNumber, Points: -5})
		}
		if heartsActive == nil {
			state.Seats[winnerIndex].Score--
			state.HandAwards = append(state.HandAwards, HandAward{Rule: RuleAvoidHearts, Title: "Avoid Hearts", Seat: state.Seats[winnerIndex].ID, Card: card.ID, Trick: state.TrickNumber, Points: -1})
		}
	}
	return nil
}

func scoreLastTrickBonus(state *MatchState, rules RuleResolution, winnerIndex int) {
	if _, err := rules.Effect(MechanicLastTrickBonus); err != nil {
		return
	}
	state.Seats[winnerIndex].Score += 2
	state.HandAwards = append(state.HandAwards, HandAward{Rule: RuleLastTrickBonus, Title: "Last Trick Bonus", Seat: state.Seats[winnerIndex].ID, Trick: state.TrickNumber, Points: 2})
}

// settleHandScores runs once on the last completed trick, before any reveal.
func settleHandScores(state *MatchState, rules RuleResolution) error {
	award := func(rule RuleID, title string, seat SeatID, card CardID, tally int) {
		for i := range state.Seats {
			if state.Seats[i].ID == seat {
				state.Seats[i].Score++
				state.HandAwards = append(state.HandAwards, HandAward{Rule: rule, Title: title, Seat: seat, Card: card, Tally: tally, Points: 1})
				return
			}
		}
	}
	active := func(mechanic Mechanic) bool { _, err := rules.Effect(mechanic); return err == nil }
	if state.TrumpSuit != "" {
		var high, low StandardCard
		for id := range state.DealOwner {
			card, ok := cardByID(id)
			if !ok {
				return fmt.Errorf("unknown dealt card %s", id)
			}
			if card.Suit != state.TrumpSuit {
				continue
			}
			if high.ID == "" || card.Rank > high.Rank {
				high = card
			}
			if low.ID == "" || card.Rank < low.Rank {
				low = card
			}
		}
		if high.ID != "" && active(MechanicHighTrump) {
			award(RuleHighTrump, "High Trump Scores", state.DealOwner[high.ID], high.ID, 0)
		}
		captor := func(id CardID) SeatID {
			for seat, cards := range state.Captured {
				for _, captured := range cards {
					if captured == id {
						return seat
					}
				}
			}
			return ""
		}
		if low.ID != "" && active(MechanicLowTrump) {
			owner := captor(low.ID)
			policy, _ := rules.Effect(MechanicLowOwner)
			if policy.Value == ValueKeepLow {
				owner = state.DealOwner[low.ID]
			}
			if owner != "" {
				award(RuleLowTrump, "Low Trump Scores", owner, low.ID, 0)
			}
		}
		if active(MechanicJackTrump) {
			for id := range state.DealOwner {
				card, _ := cardByID(id)
				if card.Suit == state.TrumpSuit && card.Rank == 11 {
					if owner := captor(id); owner != "" {
						award(RuleJackTrump, "Trump Jack Scores", owner, id, 0)
					}
					break
				}
			}
		}
	}
	if active(MechanicGamePoint) {
		best, winner, tied := -1, SeatID(""), false
		for _, seat := range state.Seats {
			tally := 0
			for _, id := range state.Captured[seat.ID] {
				card, ok := cardByID(id)
				if !ok {
					return fmt.Errorf("unknown captured card %s", id)
				}
				tally += gameCardValue(card.Rank)
			}
			if tally > best {
				best, winner, tied = tally, seat.ID, false
			} else if tally == best {
				tied = true
			}
		}
		if !tied {
			award(RuleGamePoint, "Game Point", winner, "", best)
		}
	}
	return nil
}
