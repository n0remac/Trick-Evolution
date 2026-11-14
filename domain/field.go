package domain

import "fmt"

// StartingField constructs the catalog's public starting rule instances.
// It is shared by match setup and pregame rules review.
func StartingField(catalog Catalog) []RuleInstance {
	field := []RuleInstance{}
	for _, definition := range catalog.Definitions {
		if definition.StartActive {
			field = append(field, RuleInstance{InstanceID: fmt.Sprintf("rule:initial:%d", len(field)), DefinitionID: definition.ID, Sequence: len(field), Starting: true})
		}
	}
	return field
}

func StartingRules(config Config, catalog Catalog) (RuleResolution, error) {
	if err := config.Validate(2); err != nil {
		return RuleResolution{}, err
	}
	rules, err := ResolveRules(StartingField(catalog), catalog, config)
	if err != nil {
		return RuleResolution{}, err
	}
	if err := rules.ValidateMatchEffects(config); err != nil {
		return RuleResolution{}, err
	}
	return rules, nil
}

type RuleInstance struct {
	InstanceID          string
	DefinitionID        RuleID
	Sequence            int
	IntroducedAfterHand int
	Starting            bool
	Removed             bool // Removal retains history; superseded evolution rules never revive.
}

type ResolvedRule struct {
	Instance       RuleInstance
	Active         bool
	ActiveEffects  []Mechanic
	OverriddenBy   []string
	InactiveReason string
}

type EffectiveEffect struct {
	Mechanic   Mechanic
	Value      EffectValue
	Number     int
	Source     RuleID
	InstanceID string
}

type RuleResolution struct {
	History     []ResolvedRule
	ActiveField []ResolvedRule
	Effective   []EffectiveEffect
}

func ResolveField(field []RuleInstance, catalog Catalog) ([]ResolvedRule, error) {
	r, err := ResolveRules(field, catalog, DefaultConfig())
	return r.History, err
}

// ResolveRules is the single field interpreter for the engine and projections.
// Later explicit replacements supersede prior evolutions permanently. A removed
// controller falls back to the starting rule for its mechanic.
func ResolveRules(field []RuleInstance, catalog Catalog, config Config) (RuleResolution, error) {
	if err := catalog.Validate(); err != nil {
		return RuleResolution{}, err
	}
	r := RuleResolution{History: make([]ResolvedRule, len(field))}
	seen := map[string]bool{}
	controllers := map[Mechanic]int{}
	starting := map[Mechanic]int{}
	superseded := make([]map[Mechanic]bool, len(field))
	for i, instance := range field {
		d, ok := catalog.Definition(instance.DefinitionID)
		if !ok || d.Status != StatusSupported {
			return RuleResolution{}, fmt.Errorf("unknown or unsupported field rule %s", instance.DefinitionID)
		}
		if instance.InstanceID == "" || seen[instance.InstanceID] || instance.Sequence != i {
			return RuleResolution{}, fmt.Errorf("invalid field instance or sequence at %d", i)
		}
		if instance.Starting && (!d.StartActive || instance.IntroducedAfterHand != 0) {
			return RuleResolution{}, fmt.Errorf("invalid starting rule %s", d.ID)
		}
		seen[instance.InstanceID] = true
		r.History[i] = ResolvedRule{Instance: instance}
		superseded[i] = map[Mechanic]bool{}
		for _, effect := range d.Effects {
			if instance.Starting {
				if _, exists := starting[effect.Mechanic]; exists {
					return RuleResolution{}, fmt.Errorf("duplicate starting %s rule", effect.Mechanic)
				}
				starting[effect.Mechanic] = i
			}
			oldIndex, occupied := controllers[effect.Mechanic]
			if occupied {
				old, _ := catalog.Definition(field[oldIndex].DefinitionID)
				replaces := old.ID == d.ID
				for _, target := range d.Overrides {
					if target == old.ID {
						replaces = true
					}
				}
				if !replaces {
					return RuleResolution{}, fmt.Errorf("%s does not replace %s for %s", d.ID, old.ID, effect.Mechanic)
				}
				superseded[oldIndex][effect.Mechanic] = true
				recorded := false
				for _, id := range r.History[oldIndex].OverriddenBy {
					if id == instance.InstanceID {
						recorded = true
					}
				}
				if !recorded {
					r.History[oldIndex].OverriddenBy = append(r.History[oldIndex].OverriddenBy, instance.InstanceID)
				}
			}
			controllers[effect.Mechanic] = i
		}
	}
	// Each mechanic chooses its latest eligible controller. A removed or
	// prerequisite-inactive controller falls back to its original starting rule;
	// earlier evolution entries stay superseded.
	selected := map[Mechanic]int{}
	for mechanic, index := range starting {
		if !field[index].Removed {
			selected[mechanic] = index
		}
	}
	for iteration := 0; iteration <= len(field)+1; iteration++ {
		activeIDs := map[RuleID]bool{}
		for _, index := range selected {
			activeIDs[field[index].DefinitionID] = true
		}
		next := map[Mechanic]int{}
		for mechanic, index := range controllers {
			d, _ := catalog.Definition(field[index].DefinitionID)
			eligible := !field[index].Removed
			for _, prerequisite := range d.Prerequisites {
				if !activeIDs[prerequisite] {
					eligible = false
					break
				}
			}
			if len(d.PrerequisiteAny) > 0 {
				anyActive := false
				for _, prerequisite := range d.PrerequisiteAny {
					anyActive = anyActive || activeIDs[prerequisite]
				}
				eligible = eligible && anyActive
			}
			if eligible {
				next[mechanic] = index
			} else if baseline, ok := starting[mechanic]; ok && !field[baseline].Removed {
				next[mechanic] = baseline
			}
		}
		stable := len(selected) == len(next)
		for mechanic, index := range selected {
			if other, ok := next[mechanic]; !ok || other != index {
				stable = false
				break
			}
		}
		selected = next
		if stable {
			break
		}
		if iteration == len(field)+1 {
			return RuleResolution{}, fmt.Errorf("rule prerequisites did not settle")
		}
	}
	for i, instance := range field {
		d, _ := catalog.Definition(instance.DefinitionID)
		for _, effect := range d.Effects {
			if chosen, ok := selected[effect.Mechanic]; ok && chosen == i {
				r.History[i].ActiveEffects = append(r.History[i].ActiveEffects, effect.Mechanic)
			}
		}
		r.History[i].Active = len(r.History[i].ActiveEffects) > 0
		if !r.History[i].Active {
			switch {
			case instance.Removed:
				r.History[i].InactiveReason = "removed"
			case len(superseded[i]) == len(d.Effects):
				r.History[i].InactiveReason = "superseded"
			default:
				r.History[i].InactiveReason = "prerequisite_inactive"
			}
		}
		if r.History[i].Active && d.StartingVisibility != Hidden {
			r.ActiveField = append(r.ActiveField, r.History[i])
		}
		for _, effect := range d.Effects {
			if chosen, ok := selected[effect.Mechanic]; !ok || chosen != i {
				continue
			}
			number := 0
			switch effect.Config {
			case "hand_size":
				number = config.HandSize
			case "max_hands":
				number = config.MaxHands
			case "reveal_every":
				number = config.RevealEvery
			}
			r.Effective = append(r.Effective, EffectiveEffect{effect.Mechanic, effect.Value, number, d.ID, instance.InstanceID})
		}
	}
	return r, nil
}

func (r RuleResolution) Effect(mechanic Mechanic) (EffectiveEffect, error) {
	for _, e := range r.Effective {
		if e.Mechanic == mechanic {
			return e, nil
		}
	}
	return EffectiveEffect{}, fmt.Errorf("no active %s effect", mechanic)
}

// ValidateMatchEffects rejects catalogs or records that omit a required
// baseline or try to reinterpret it under the Stage 1 engine.
func (r RuleResolution) ValidateMatchEffects(config Config) error {
	checks := []struct {
		mechanic Mechanic
		value    EffectValue
		number   int
	}{
		{MechanicDeck, ValueStandardDeck, 0}, {MechanicHandSize, ValueConfigured, config.HandSize},
		{MechanicUnused, ValueOutOfPlay, 0}, {MechanicScoreOwnership, ValueIndividual, 0},
		{MechanicTurnOrder, ValueClockwise, 0}, {MechanicFirstLeader, ValueLeftOfDealer, 0},
		{MechanicNextLeader, ValueWinnerLeads, 0}, {MechanicTrump, "", 0},
		{MechanicPlayRestriction, "", 0}, {MechanicWinnerEligibility, ValueLeadSuit, 0},
		{MechanicRankComparison, "", 0}, {MechanicTieBreak, ValueFirstPlay, 0},
		{MechanicTrickPoints, ValueOnePoint, 0}, {MechanicHandLimit, ValueConfigured, config.MaxHands},
		{MechanicVictory, ValueHighestShared, 0}, {MechanicReveal, ValueConfigured, config.RevealEvery},
	}
	for _, check := range checks {
		effect, err := r.Effect(check.mechanic)
		if err != nil {
			return err
		}
		if check.value != "" && effect.Value != check.value || effect.Number != check.number {
			return fmt.Errorf("unsupported %s effect %s/%d", check.mechanic, effect.Value, effect.Number)
		}
	}
	return nil
}

func RecordCompletedHand(state MatchState, catalog Catalog) (MatchState, *RuleInstance, error) {
	if state.CatalogVersion != catalog.Version {
		return state, nil, fmt.Errorf("catalog version mismatch")
	}
	if err := state.Config.Validate(len(state.Seats)); err != nil {
		return state, nil, err
	}
	if _, err := ResolveRules(state.Field, catalog, state.Config); err != nil {
		return state, nil, err
	}
	next := state
	next.Field = append([]RuleInstance(nil), state.Field...)
	next.RuleDraw = append([]RuleID(nil), state.RuleDraw...)
	next.CompletedHands++
	if next.CompletedHands >= next.Config.MaxHands || next.CompletedHands%next.Config.RevealEvery != 0 {
		return next, nil, nil
	}
	for index, id := range next.RuleDraw {
		d, ok := catalog.Definition(id)
		if !ok || !d.InDeck || d.Status != StatusSupported {
			return state, nil, fmt.Errorf("invalid rule draw %s", id)
		}
		duplicate := false
		for _, instance := range next.Field {
			if instance.DefinitionID == id && !instance.Removed {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		instance := RuleInstance{InstanceID: fmt.Sprintf("rule:reveal:%d", len(next.Field)), DefinitionID: id, Sequence: len(next.Field), IntroducedAfterHand: next.CompletedHands}
		candidate := append(append([]RuleInstance(nil), next.Field...), instance)
		resolved, err := ResolveRules(candidate, catalog, next.Config)
		if err != nil || !resolved.History[len(candidate)-1].Active {
			continue
		}
		next.RuleDraw = append(next.RuleDraw[:index], next.RuleDraw[index+1:]...)
		next.Field = candidate
		return next, &instance, nil
	}
	return next, nil, nil
}
