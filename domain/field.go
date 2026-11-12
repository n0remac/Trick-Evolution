package domain

import "fmt"

type RuleInstance struct {
	InstanceID          string
	DefinitionID        RuleID
	Sequence            int
	IntroducedAfterHand int
}

type ResolvedRule struct {
	Instance     RuleInstance
	Active       bool
	OverriddenBy []string // Explicit overriding instance IDs, in activation order.
}

// ResolveField is a projection: it never removes or changes a field instance.
// Only explicit definition IDs in Overrides can disable another instance.
// The source may arrive before its target; the rule deck is shuffled.
func ResolveField(field []RuleInstance, catalog Catalog) ([]ResolvedRule, error) {
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	resolved := make([]ResolvedRule, len(field))
	seenInstances := map[string]bool{}
	for i, instance := range field {
		definition, ok := catalog.Definition(instance.DefinitionID)
		if !ok || definition.Status != StatusSupported {
			return nil, fmt.Errorf("unknown or unsupported field rule %s", instance.DefinitionID)
		}
		if instance.InstanceID == "" || seenInstances[instance.InstanceID] || instance.Sequence != i {
			return nil, fmt.Errorf("invalid field instance or sequence at %d", i)
		}
		seenInstances[instance.InstanceID] = true
		resolved[i] = ResolvedRule{Instance: instance, Active: true}
	}
	for i, instance := range field {
		definition, _ := catalog.Definition(instance.DefinitionID)
		for j := range resolved {
			if i == j {
				continue
			}
			for _, target := range definition.Overrides {
				if resolved[j].Instance.DefinitionID == target {
					resolved[j].Active = false
					resolved[j].OverriddenBy = append(resolved[j].OverriddenBy, instance.InstanceID)
				}
			}
		}
	}
	return resolved, nil
}

// RecordCompletedHand handles only F1 reveal bookkeeping. F2 must call it
// after a valid hand-completion transition; it does not award tricks or deal.
// On deck exhaustion, no rule is revealed and the completed count still grows.
func RecordCompletedHand(state MatchState, catalog Catalog) (MatchState, *RuleInstance, error) {
	if state.CatalogVersion != catalog.Version {
		return state, nil, fmt.Errorf("catalog version mismatch")
	}
	if err := state.Config.Validate(len(state.Seats)); err != nil {
		return state, nil, err
	}
	if _, err := ResolveField(state.Field, catalog); err != nil {
		return state, nil, err
	}
	next := state // Copy slices before changing either one.
	next.Field = append([]RuleInstance(nil), state.Field...)
	next.RuleDraw = append([]RuleID(nil), state.RuleDraw...)
	next.CompletedHands++
	if next.CompletedHands%next.Config.RevealEvery != 0 || len(next.RuleDraw) == 0 {
		return next, nil, nil
	}
	id := next.RuleDraw[0]
	definition, ok := catalog.Definition(id)
	if !ok || !definition.InDeck {
		return state, nil, fmt.Errorf("invalid rule draw %s", id)
	}
	instance := RuleInstance{InstanceID: fmt.Sprintf("rule:reveal:%d", len(next.Field)), DefinitionID: id,
		Sequence: len(next.Field), IntroducedAfterHand: next.CompletedHands}
	next.RuleDraw = next.RuleDraw[1:]
	next.Field = append(next.Field, instance)
	if _, err := ResolveField(next.Field, catalog); err != nil {
		return state, nil, err
	}
	return next, &instance, nil
}
