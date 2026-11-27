package domain

import "fmt"

// AuctionBounds derives a finite legal range from this deal's effective rules.
func AuctionBounds(state MatchState, rules RuleResolution) (int, int, EffectValue, error) {
	metric, err := rules.Effect(MechanicBidMetric)
	if err != nil {
		return 0, 0, "", err
	}
	maxBid := state.TricksPerHand
	if metric.Value == ValueBidPoints {
		maxBid = 0
		trick, _ := rules.Effect(MechanicTrickPoints)
		switch trick.Value {
		case ValueOnePoint:
			maxBid += state.TricksPerHand
		case ValueAboveHalf:
			threshold := state.TricksPerHand / len(state.Seats)
			if owner, err := rules.Effect(MechanicScoreOwnership); err == nil && owner.Value == ValuePartnership {
				threshold = state.TricksPerHand / 2
			}
			maxBid += state.TricksPerHand - threshold
		}
		for _, mechanic := range []Mechanic{MechanicHighTrump, MechanicLowTrump, MechanicJackTrump, MechanicGamePoint} {
			if _, err := rules.Effect(mechanic); err == nil {
				maxBid++
			}
		}
		if _, err := rules.Effect(MechanicLastTrickBonus); err == nil {
			maxBid += 2
		}
	}
	if maxBid < 1 {
		return 0, 0, "", fmt.Errorf("auction requires at least one attainable point or trick")
	}
	minBid := 1
	// Four-point Pitch: the four hand awards, no ordinary trick points.
	trick, _ := rules.Effect(MechanicTrickPoints)
	fourAwards := true
	for _, mechanic := range []Mechanic{MechanicHighTrump, MechanicLowTrump, MechanicJackTrump, MechanicGamePoint} {
		_, err := rules.Effect(mechanic)
		fourAwards = fourAwards && err == nil
	}
	if metric.Value == ValueBidPoints && trick.Value == ValueNoPoints && fourAwards && maxBid == 4 && activeEndlessRule(rules, RulePitchTrump) {
		minBid = 2
	}
	return minBid, maxBid, metric.Value, nil
}

func startAuction(state *MatchState, rules RuleResolution) error {
	state.Auction = nil
	if _, err := rules.Effect(MechanicAuction); err != nil {
		return nil
	}
	minBid, maxBid, metric, err := AuctionBounds(*state, rules)
	if err != nil {
		return err
	}
	state.Auction = &AuctionState{Metric: metric, Min: minBid, Max: maxBid, Passed: make(map[SeatID]bool)}
	state.Phase = PhaseAuction
	return nil
}

func stepAuction(state MatchState, action Action) (MatchState, []Event, error) {
	if state.Phase != PhaseAuction || state.Auction == nil || state.Auction.Contract != nil {
		return state, nil, transitionError(ErrWrongPhase, "no auction is open")
	}
	if action.Seat != state.Turn {
		return state, nil, transitionError(ErrNotYourTurn, "another seat has the auction turn")
	}
	if action.Kind == ActionPass && action.Bid != 0 {
		return state, nil, transitionError(ErrInvalidBid, "pass cannot include a bid")
	}
	if action.Kind == ActionBid && (action.Bid < state.Auction.Min || action.Bid > state.Auction.Max || action.Bid <= state.Auction.HighBid) {
		return state, nil, transitionError(ErrInvalidBid, "bid is outside the legal rising range")
	}
	next := cloneState(state)
	a := next.Auction
	call := AuctionCall{Seat: action.Seat}
	if action.Kind == ActionPass {
		a.Passed[action.Seat] = true
	} else {
		call.Bid = action.Bid
		a.HighBid, a.HighBidder = action.Bid, action.Seat
	}
	a.Calls = append(a.Calls, call)
	next.Revision++
	remaining := 0
	for _, seat := range next.Seats {
		if !a.Passed[seat.ID] && seat.ID != a.HighBidder {
			remaining++
		}
	}
	if remaining == 0 || a.HighBid == a.Max {
		fallback := a.HighBidder == ""
		if a.HighBidder == "" {
			a.HighBidder, a.HighBid = next.Dealer, a.Min
		}
		team := -1
		if id, ok := next.HandTeams[a.HighBidder]; ok {
			team = id
		}
		a.Contract = &Contract{Bidder: a.HighBidder, Team: team, Bid: a.HighBid, Metric: a.Metric, Fallback: fallback}
		next.Phase, next.Leader, next.Turn = PhasePlayingTrick, a.HighBidder, a.HighBidder
		return next, []Event{{Kind: EventAuctionCompleted, Revision: next.Revision, Hand: next.HandNumber, Seat: a.HighBidder, Bid: a.HighBid, Fallback: fallback}}, nil
	}
	index := indexOfSeat(next.Seats, action.Seat)
	for offset := 1; offset <= len(next.Seats); offset++ {
		id := next.Seats[(index+offset)%len(next.Seats)].ID
		if !a.Passed[id] && id != a.HighBidder {
			next.Turn = id
			break
		}
	}
	return next, []Event{{Kind: EventAuctionCall, Revision: next.Revision, Hand: next.HandNumber, Seat: action.Seat, Bid: call.Bid}}, nil
}

func settleContract(state *MatchState, rules RuleResolution) {
	if state.Auction == nil || state.Auction.Contract == nil {
		return
	}
	c := state.Auction.Contract
	for _, seat := range state.Seats {
		inContract := seat.ID == c.Bidder || c.Team >= 0 && state.HandTeams[seat.ID] == c.Team
		if !inContract {
			continue
		}
		if c.Metric == ValueBidTricks {
			c.Actual += len(state.Captured[seat.ID]) / len(state.Seats)
		}
	}
	if c.Metric == ValueBidPoints {
		for _, award := range state.HandAwards {
			if award.Seat == c.Bidder || c.Team >= 0 && state.HandTeams[award.Seat] == c.Team {
				c.Actual += award.Points
			}
		}
	}
	c.Made = c.Actual >= c.Bid
	if c.Made {
		return
	}
	if _, err := rules.Effect(MechanicSetBidder); err != nil {
		return
	}
	kept := state.HandAwards[:0]
	for _, award := range state.HandAwards {
		if award.Seat != c.Bidder && (c.Team < 0 || state.HandTeams[award.Seat] != c.Team) {
			kept = append(kept, award)
			continue
		}
		for i := range state.Seats {
			if state.Seats[i].ID == award.Seat {
				state.Seats[i].Score -= award.Points
				break
			}
		}
		if c.Team >= 0 {
			state.TeamScores[c.Team] -= award.Points
		}
		if award.TeamWide {
			state.TeamAwardBalances[award.Seat] -= award.Points
		}
	}
	state.HandAwards = kept
	seat := c.Bidder
	if c.Team >= 0 {
		seat = teamAwardSeat(state, c.Team)
	}
	awardPoints(state, HandAward{Rule: RuleSetBidder, Title: "Set the Bidder", Seat: seat, Points: -c.Bid, TeamWide: c.Team >= 0})
	c.Penalty = c.Bid
}
