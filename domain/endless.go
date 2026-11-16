package domain

import "fmt"

// EndlessRound returns the number of complete hands and boundary event for a round.
// The late cadence is deliberately deterministic so a run can be replayed.
func EndlessRound(round int) (int, string) {
	switch round {
	case 1:
		return 3, "base"
	case 2, 4:
		return 1, "draft"
	case 3:
		return 1, "table"
	case 5:
		return 3, "hold"
	case 6:
		return 2, "table"
	}
	if round < 1 {
		return 0, ""
	}
	length := 2 + (round-7)%3
	if round%2 == 1 {
		return length, "draft"
	}
	if (round/2)%2 == 0 {
		return length, "hold"
	}
	return length, "table"
}

func eligibleEndlessRule(state MatchState, catalog Catalog, id RuleID) ([]RuleInstance, bool) {
	d, ok := catalog.Definition(id)
	if !ok || !d.InDeck || d.Status != StatusSupported {
		return nil, false
	}
	for _, r := range state.Field {
		if r.DefinitionID == id && !r.Removed {
			return nil, false
		}
	}
	field := append(append([]RuleInstance(nil), state.Field...), RuleInstance{
		InstanceID: fmt.Sprintf("rule:endless:%d", len(state.Field)), DefinitionID: id,
		Sequence: len(state.Field), IntroducedAfterHand: state.CompletedHands,
	})
	resolved, err := ResolveRules(field, catalog, state.Config)
	if err != nil || !resolved.History[len(field)-1].Active {
		return nil, false
	}
	if err = resolved.ValidateMatchEffects(state.Config); err != nil {
		return nil, false
	}
	if _, err = HandPlanForRules(state.Config, len(state.Seats), resolved); err != nil {
		return nil, false
	}
	if !preservesDependencies(state.Field, field, catalog, state.Config) {
		return nil, false
	}
	return field, true
}

func preservesDependencies(before, after []RuleInstance, catalog Catalog, config Config) bool {
	old, err := ResolveRules(before, catalog, config)
	if err != nil {
		return false
	}
	next, err := ResolveRules(after, catalog, config)
	if err != nil {
		return false
	}
	for i, r := range old.History {
		if !r.Active {
			continue
		}
		d, _ := catalog.Definition(before[i].DefinitionID)
		if len(d.Prerequisites)+len(d.PrerequisiteAny) == 0 {
			continue
		}
		if after[i].Removed {
			continue
		}
		// A directly superseded rule may disappear; an unrelated dependent may not.
		if !next.History[i].Active && next.History[i].InactiveReason == "prerequisite_inactive" {
			return false
		}
	}
	return true
}

func endlessOptions(state MatchState, catalog Catalog) []RuleID {
	options := make([]RuleID, 0, 3)
	if state.Round == 2 {
		for _, id := range []RuleID{RuleLastTrickBonus, RuleAvoidHearts, RuleExpandedHand} {
			if _, ok := eligibleEndlessRule(state, catalog, id); ok {
				options = append(options, id)
			}
		}
	}
	for _, id := range state.RuleDraw {
		if len(options) == 3 {
			break
		}
		found := false
		for _, existing := range options {
			found = found || existing == id
		}
		if found {
			continue
		}
		if _, ok := eligibleEndlessRule(state, catalog, id); ok {
			options = append(options, id)
		}
	}
	return options
}

func applyEndlessRule(state *MatchState, catalog Catalog, id RuleID) bool {
	field, ok := eligibleEndlessRule(*state, catalog, id)
	if !ok {
		return false
	}
	state.Field = field
	for i, draw := range state.RuleDraw {
		if draw == id {
			state.RuleDraw = append(state.RuleDraw[:i], state.RuleDraw[i+1:]...)
			break
		}
	}
	return true
}

func retireEndlessRule(state *MatchState, catalog Catalog) (RuleID, bool) {
	for i := len(state.Field) - 1; i >= 0; i-- {
		instance := state.Field[i]
		if instance.Starting || instance.Removed {
			continue
		}
		candidate := append([]RuleInstance(nil), state.Field...)
		candidate[i].Removed = true
		resolved, err := ResolveRules(candidate, catalog, state.Config)
		if err != nil || resolved.ValidateMatchEffects(state.Config) != nil {
			continue
		}
		if _, err := HandPlanForRules(state.Config, len(state.Seats), resolved); err != nil {
			continue
		}
		if !preservesDependencies(state.Field, candidate, catalog, state.Config) {
			continue
		}
		state.Field = candidate
		return instance.DefinitionID, true
	}
	return "", false
}

func settleEndlessBoundary(state *MatchState, catalog Catalog) error {
	length, _ := EndlessRound(state.Round)
	if state.RoundHand < length {
		state.TableCue = fmt.Sprintf("Round %d: %d hand(s) until the next round.", state.Round, length-state.RoundHand)
		return nil
	}
	state.Round++
	state.RoundHand = 0
	state.PendingBoundaryID = ""
	state.DraftOptions = nil
	_, event := EndlessRound(state.Round)
	switch event {
	case "draft":
		state.DraftOptions = endlessOptions(*state, catalog)
		if len(state.DraftOptions) > 0 {
			state.PendingBoundaryID = fmt.Sprintf("round:%d:after:%d", state.Round, state.CompletedHands)
			state.Phase = PhaseAwaitingChoice
			state.TableCue = fmt.Sprintf("Round %d: choose a rule or keep the field.", state.Round)
			return nil
		}
		state.TableCue = fmt.Sprintf("Round %d: no eligible change; rules stay the same.", state.Round)
	case "table":
		for _, id := range state.RuleDraw {
			if applyEndlessRule(state, catalog, id) {
				definition, _ := catalog.Definition(id)
				state.TableCue = "The table added " + definition.Title + "."
				return nil
			}
		}
		if id, ok := retireEndlessRule(state, catalog); ok {
			definition, _ := catalog.Definition(id)
			state.TableCue = "The table retired " + definition.Title + "."
		} else {
			state.TableCue = "The table found no legal change; rules stay the same."
		}
	case "hold":
		state.TableCue = fmt.Sprintf("Round %d: rules stay the same.", state.Round)
	}
	return nil
}

func stepChooseRule(state MatchState, action Action, seatIndex int, catalog Catalog) (MatchState, []Event, error) {
	if state.Config.Mode != ModeEndless || state.Phase != PhaseAwaitingChoice {
		return state, nil, transitionError(ErrWrongPhase, "no rule choice is pending")
	}
	if seatIndex != 0 {
		return state, nil, transitionError(ErrNotHost, "only the runner may choose")
	}
	if action.BoundaryID == "" || action.BoundaryID != state.PendingBoundaryID {
		return state, nil, transitionError(ErrInvalidState, "stale round boundary")
	}
	next := cloneState(state)
	if action.Rule != "" {
		valid := false
		for _, id := range state.DraftOptions {
			valid = valid || id == action.Rule
		}
		if !valid || !applyEndlessRule(&next, catalog, action.Rule) {
			return state, nil, transitionError(ErrInvalidState, "rule is not an eligible choice")
		}
		definition, _ := catalog.Definition(action.Rule)
		next.TableCue = "You chose " + definition.Title + "."
	} else {
		next.TableCue = "You kept the current rules."
	}
	next.PendingBoundaryID, next.DraftOptions = "", nil
	next.Phase = PhaseHandComplete
	next.Revision++
	return next, []Event{{Kind: EventRoundDecision, Revision: next.Revision, Hand: next.HandNumber, Rule: action.Rule}}, nil
}
