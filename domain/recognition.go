package domain

// RecognizeKnownGame compares resolved mechanics and the actual deal. Hidden
// baselines and card names are irrelevant; any extra play/scoring mechanic
// prevents a match. Pitch is labelled a supported variant because this engine
// has no draw/discard or forced trump lead.
func RecognizeKnownGame(s MatchState, r RuleResolution) string {
	if len(s.Seats) != 4 {
		return ""
	}
	base := map[Mechanic]EffectValue{
		MechanicDeck: ValueStandardDeck, MechanicUnused: ValueOutOfPlay,
		MechanicTurnOrder: ValueClockwise, MechanicFirstLeader: ValueLeftOfDealer,
		MechanicNextLeader: ValueWinnerLeads, MechanicBreakHearts: "",
		MechanicBreakSpades: "", MechanicPlayRestriction: ValueFollow,
		MechanicWinnerEligibility: ValueLeadSuit, MechanicRankComparison: ValueHigh,
		MechanicTieBreak: ValueFirstPlay, MechanicLowOwner: ValueCaptureLow,
		MechanicBidMetric: ValueBidPoints,
	}
	for _, e := range r.Effective {
		if want, ok := base[e.Mechanic]; ok && want != "" && want != e.Value {
			return ""
		}
		if e.Mechanic == MechanicBreakHearts || e.Mechanic == MechanicBreakSpades {
			return ""
		}
	}
	whist := map[Mechanic]EffectValue{
		MechanicTrump: ValueTurnUpTrump, MechanicTrickPoints: ValueAboveHalf,
		MechanicVictory: ValueFirstTo5,
	}
	pitch := map[Mechanic]EffectValue{
		MechanicTrump: ValueFirstLeadTrump, MechanicTrickPoints: ValueNoPoints,
		MechanicVictory: ValueFirstTo21, MechanicAuction: ValueAuction,
		MechanicBidMetric: ValueBidPoints, MechanicSetBidder: ValueAward,
		MechanicWinOnBid: ValueWinOnBid, MechanicHighTrump: ValueAward,
		MechanicLowTrump: ValueAward, MechanicJackTrump: ValueAward,
		MechanicGamePoint: ValueAward,
	}
	match := func(want map[Mechanic]EffectValue) bool {
		ignored := map[Mechanic]bool{MechanicHandSize: true, MechanicHandLimit: true, MechanicReveal: true,
			MechanicScoreOwnership: true, MechanicFullDeal: true, MechanicSixCardHand: true}
		for _, e := range r.Effective {
			if ignored[e.Mechanic] {
				continue
			}
			if expected, ok := base[e.Mechanic]; ok {
				if expected != e.Value {
					return false
				}
				continue
			}
			if expected, ok := want[e.Mechanic]; !ok || expected != e.Value {
				return false
			}
		}
		for mechanic, value := range want {
			e, err := r.Effect(mechanic)
			if err != nil || e.Value != value {
				return false
			}
		}
		return true
	}
	owner, _ := r.Effect(MechanicScoreOwnership)
	if s.DealSize == 13 && s.TricksPerHand == 13 && match(whist) {
		if owner.Value == ValuePartnership {
			return "Classic short Whist (without honours)"
		}
		if owner.Value == ValueIndividual {
			return "Individual Whist-style variant"
		}
	}
	if s.DealSize == 6 && s.TricksPerHand == 6 && match(pitch) {
		min, max, metric, err := AuctionBounds(s, r)
		if err != nil || min != 2 || max != 4 || metric != ValueBidPoints {
			return ""
		}
		if owner.Value == ValuePartnership {
			return "Four-point partnership Pitch variant"
		}
		if owner.Value == ValueIndividual {
			return "Four-point cutthroat Pitch variant"
		}
	}
	return ""
}
