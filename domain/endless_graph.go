package domain

// Follow-ups are ordered. These are draftable IDs; the baseline Capture Low
// controller is deliberately absent.
var endlessFollowUps = map[RuleID][]RuleID{
	RuleExpandedHand:      {RuleKeepOne, RuleGamePoint, RuleSixCardHand, RuleWhistDeal},
	RuleSixCardHand:       {RuleWhistDeal, RuleKeepOne},
	RuleWhistDeal:         {RuleWhistMajority, RuleWhistTrump},
	RulePartnership:       {RuleRestoreIndividual, RuleWhistMajority},
	RuleRestoreIndividual: {RulePartnership},
	RuleWhistTrump:        {RuleHighTrump, RuleLowTrump, RuleJackTrump},
	RuleWhistMajority:     {RulePointsPerTrick, RuleGamePoint},
	RuleKeepOne:           {RuleLastTrickBonus, RuleAvoidHearts},
	RuleSpadesTrump:       {RuleBreakSpades, RuleHighTrump, RuleFollowOrTrump},
	RulePitchTrump:        {RuleHighTrump, RuleJackTrump, RuleFollowOrTrump, RuleWhistTrump},
	RuleFollowOrTrump:     {RuleJackTrump, RuleLowTrump},
	RuleBreakSpades:       {RuleJackTrump, RuleHighTrump},
	RuleHighTrump:         {RuleLowTrump, RuleJackTrump},
	RuleLowTrump:          {RuleKeepLow, RuleCaptureLow, RuleHighTrump},
	RuleKeepLow:           {RuleCaptureLow, RuleJackTrump},
	RuleCaptureLow:        {RuleKeepLow, RuleJackTrump},
	RuleJackTrump:         {RuleGamePoint, RuleLowTrump},
	RuleGamePoint:         {RuleNoPoints, RuleLastTrickBonus, RulePartnership},
	RuleAvoidHearts:       {RuleBreakHearts, RuleAvoidQueenHearts},
	RuleBreakHearts:       {RuleAvoidHearts, RuleAvoidQueenHearts},
	RuleAvoidQueenHearts:  {RuleAvoidHearts, RuleBreakHearts},
	RuleLastTrickBonus:    {RuleKeepOne, RuleNoPoints},
	RuleNoPoints:          {RulePointsPerTrick, RuleGamePoint, RuleLastTrickBonus},
	RulePointsPerTrick:    {RuleGamePoint, RuleLastTrickBonus, RuleWhistMajority},
}

var endlessStarters = []RuleID{RuleSpadesTrump, RulePitchTrump, RuleAvoidHearts, RuleGamePoint, RuleLastTrickBonus}

func endlessDrawPosition(state MatchState) map[RuleID]int {
	position := make(map[RuleID]int, len(state.RuleDraw))
	for i, id := range state.RuleDraw {
		position[id] = i
	}
	return position
}

func endlessFollowUpChoices(state MatchState, catalog Catalog) []RuleID {
	resolved, err := ResolveRules(state.Field, catalog, state.Config)
	if err != nil {
		return nil
	}
	draw := endlessDrawPosition(state)
	seen := map[RuleID]bool{}
	var choices []RuleID
	// A restoration takes precedence while ordinary trick points are disabled.
	if activeEndlessRule(resolved, RuleNoPoints) && endlessCandidate(state, catalog, draw, RulePointsPerTrick) {
		choices = append(choices, RulePointsPerTrick)
		seen[RulePointsPerTrick] = true
	}
	for i := len(resolved.History) - 1; i >= 0; i-- {
		source := resolved.History[i]
		if !source.Active {
			continue
		}
		for _, id := range endlessFollowUps[source.Instance.DefinitionID] {
			if !seen[id] && endlessCandidate(state, catalog, draw, id) {
				choices = append(choices, id)
				seen[id] = true
			}
		}
	}
	return choices
}

func activeEndlessRule(resolved RuleResolution, id RuleID) bool {
	for _, rule := range resolved.History {
		if rule.Active && rule.Instance.DefinitionID == id {
			return true
		}
	}
	return false
}

func endlessCandidate(state MatchState, catalog Catalog, draw map[RuleID]int, id RuleID) bool {
	if _, ok := draw[id]; !ok {
		return false
	}
	_, ok := eligibleEndlessRule(state, catalog, id)
	return ok
}

func endlessAutomaticChoice(state MatchState, catalog Catalog) (RuleID, bool) {
	choices := endlessFollowUpChoices(state, catalog)
	if len(choices) == 0 {
		return "", false
	}
	return choices[0], true
}
