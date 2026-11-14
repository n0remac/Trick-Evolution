package gameui

import (
	"fmt"
	"sort"
	"strings"

	h "github.com/n0remac/GoDom/html"
	"github.com/n0remac/trick-evolution/domain"
)

// fieldOrder uses explicit replacement history only for presentation position.
// Mechanics and activity have already been resolved by the domain projection.
func fieldOrder(public domain.PublicView, catalog domain.Catalog) []domain.ResolvedRule {
	ordered := append([]domain.ResolvedRule(nil), public.ActiveField...)
	byID := make(map[string]domain.ResolvedRule, len(public.Rules))
	parents := make(map[string][]string)
	for _, rule := range public.Rules {
		byID[rule.Instance.InstanceID] = rule
		for _, child := range rule.OverriddenBy {
			parents[child] = append(parents[child], rule.Instance.InstanceID)
		}
	}
	anchor := func(rule domain.ResolvedRule) int {
		position := rule.Instance.Sequence
		definition, ok := catalog.Definition(rule.Instance.DefinitionID)
		if !ok {
			return position
		}
		seen := map[string]bool{}
		var walk func(string)
		walk = func(id string) {
			if seen[id] {
				return
			}
			seen[id] = true
			for _, parentID := range parents[id] {
				parent, exists := byID[parentID]
				if !exists {
					continue
				}
				prior, ok := catalog.Definition(parent.Instance.DefinitionID)
				if ok && prior.FieldCategory == definition.FieldCategory {
					if parent.Instance.Sequence < position {
						position = parent.Instance.Sequence
					}
					walk(parentID)
				}
			}
		}
		walk(rule.Instance.InstanceID)
		return position
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		left, right := anchor(ordered[i]), anchor(ordered[j])
		if left != right {
			return left < right
		}
		// A partial replacement occupies its ancestor's slot before the
		// ancestor's remaining independent effects.
		return ordered[i].Instance.Sequence > ordered[j].Instance.Sequence
	})
	return ordered
}

func renderRuleField(public domain.PublicView, catalog domain.Catalog) *h.Node {
	ordered := fieldOrder(public, catalog)
	zones := []*h.Node{h.Id("game-rules"), h.Class("rule-field"), h.Attr("aria-label", "Rules in play")}
	for _, group := range reviewCategories {
		cards := []*h.Node{}
		for _, rule := range ordered {
			definition, ok := catalog.Definition(rule.Instance.DefinitionID)
			if ok && rule.Active && definition.FieldCategory == group.category {
				cards = append(cards, renderFieldCard(rule, definition, public, catalog, group.label))
			}
		}
		if len(cards) == 0 {
			cards = append(cards, h.P(h.Class("rule-zone-empty"), h.T("Base rules only")))
		}
		zone := []*h.Node{h.Class("rule-zone"), h.Attr("data-category", string(group.category)),
			h.NewNode("h3", []*h.Node{h.T(group.label)}),
			h.Div(append([]*h.Node{h.Class("rule-zone-cards")}, cards...)...)}
		zones = append(zones, h.Section(zone...))
	}
	return h.Section(zones...)
}

func renderFieldCard(rule domain.ResolvedRule, definition domain.RuleDefinition, public domain.PublicView, catalog domain.Catalog, category string) *h.Node {
	short := []string{}
	for _, effect := range public.Effective {
		if effect.InstanceID == rule.Instance.InstanceID {
			short = append(short, shortEffect(effect))
		}
	}
	partial := len(rule.ActiveEffects) < len(definition.Effects)
	label := definition.Title + ", " + category + ". " + strings.Join(short, "; ")
	if partial {
		label += ". Some effects have been replaced; open details."
	}
	children := []*h.Node{h.Type("button"), h.Class("rule rule-card"), h.Attr("data-rule-instance", rule.Instance.InstanceID),
		h.Attr("data-rule-title", definition.Title), h.Attr("data-rule-detail", fieldCardDetail(rule, definition, public, catalog)),
		h.Attr("aria-label", label), h.Attr("aria-haspopup", "dialog"),
		h.Span(h.Class("rule-card-category"), h.T(category)),
		h.NewNode("strong", []*h.Node{h.Class("rule-card-title"), h.T(definition.Title)}),
		h.Span(h.Class("rule-card-effect"), h.T(strings.Join(short, "; ")))}
	if partial {
		children = append(children, h.Span(h.Class("rule-card-partial"), h.T("Partly replaced")))
	}
	return h.Button(children...)
}

func shortEffect(effect domain.EffectiveEffect) string {
	switch effect.Mechanic {
	case domain.MechanicPlayRestriction:
		if effect.Value == domain.ValueFollow {
			return "Follow suit when able"
		}
		return "Any suit may be played"
	case domain.MechanicWinnerEligibility:
		return "Lead suit can win"
	case domain.MechanicRankComparison:
		if effect.Value == domain.ValueLow {
			return "Lowest eligible card wins"
		}
		return "Highest eligible card wins"
	}
	return effectDescription(effect)
}

func fieldCardDetail(rule domain.ResolvedRule, definition domain.RuleDefinition, public domain.PublicView, catalog domain.Catalog) string {
	lines := []string{"Category: " + categoryLabel(definition.FieldCategory) + "."}
	if rule.Instance.Starting {
		lines = append(lines, "Starting field card.")
	} else {
		lines = append(lines, fmt.Sprintf("Revealed after hand %d; applies from hand %d.", rule.Instance.IntroducedAfterHand, rule.Instance.IntroducedAfterHand+1))
	}
	for _, effect := range public.Effective {
		if effect.InstanceID == rule.Instance.InstanceID {
			lines = append(lines, effectDescription(effect))
		}
	}
	for _, declared := range definition.Effects {
		active := false
		for _, mechanic := range rule.ActiveEffects {
			if mechanic == declared.Mechanic {
				active = true
			}
		}
		if active {
			continue
		}
		controller := "another rule"
		for _, effect := range public.Effective {
			if effect.Mechanic == declared.Mechanic {
				if current, ok := catalog.Definition(effect.Source); ok {
					controller = current.Title
				}
				break
			}
		}
		lines = append(lines, fmt.Sprintf("%s is no longer controlled by this card; %s controls it now.", mechanicNames([]domain.Mechanic{declared.Mechanic}), controller))
	}
	if len(definition.Prerequisites) > 0 {
		for _, id := range definition.Prerequisites {
			prerequisite, _ := catalog.Definition(id)
			status := "inactive"
			for _, candidate := range public.Rules {
				if candidate.Instance.DefinitionID == id && candidate.Active {
					status = "active"
					break
				}
			}
			lines = append(lines, "Requires "+prerequisite.Title+" ("+status+").")
		}
	} else {
		lines = append(lines, "No prerequisite rules.")
	}
	replaced := []string{}
	for _, candidate := range public.Rules {
		for _, id := range candidate.OverriddenBy {
			if id == rule.Instance.InstanceID {
				if prior, ok := catalog.Definition(candidate.Instance.DefinitionID); ok {
					replaced = append(replaced, prior.Title)
				}
			}
		}
	}
	if len(replaced) > 0 {
		lines = append(lines, "Replaces: "+strings.Join(replaced, ", ")+".")
	}
	return strings.Join(lines, "\n")
}

func categoryLabel(category domain.FieldCategory) string {
	for _, group := range reviewCategories {
		if group.category == category {
			return group.label
		}
	}
	return string(category)
}
