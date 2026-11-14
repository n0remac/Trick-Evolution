package gameui

import (
	"fmt"
	"strings"

	h "github.com/n0remac/GoDom/html"
	"github.com/n0remac/trick-evolution/domain"
)

var reviewCategories = []struct {
	category domain.FieldCategory
	label    string
}{
	{domain.FieldHand, "Hand"}, {domain.FieldPlay, "Play"}, {domain.FieldTrick, "Trick"}, {domain.FieldScoreVictory, "Score & Victory"},
}

// rulesReviewContent is a live OOB fragment inside a persistent modal shell.
// It uses only the public projection and server config, never private hands or
// the order of the remaining evolution deck.
func rulesReviewContent(message serverMessage, seat domain.SeatID, catalog domain.Catalog, config domain.Config) (*h.Node, error) {
	if message.View != nil {
		public := message.View.Public
		if public.HandSize > 0 && public.MaxHands > 0 && public.RevealEvery > 0 {
			config = domain.Config{HandSize: public.HandSize, MaxHands: public.MaxHands, RevealEvery: public.RevealEvery}
		}
	}
	starting, err := domain.StartingRules(config, catalog)
	if err != nil {
		return nil, err
	}
	current := starting
	if message.View != nil {
		current, err = domain.EffectiveForView(message.View.Public, catalog)
		if err != nil {
			return nil, err
		}
	}
	lobby := message.Type == "lobby"
	host := len(message.Seats) > 0 && message.Seats[0].ID == seat
	defaultTab := "current"
	if lobby {
		defaultTab = "starting"
	}
	tabs := []struct{ id, label string }{{"current", "Current rules"}, {"starting", "Starting rules"}, {"changes", "Changes"}}
	tabNodes := []*h.Node{h.Attr("role", "tablist"), h.Attr("aria-label", "Game Rules views"), h.Class("review-tabs")}
	for _, tab := range tabs {
		selected := tab.id == defaultTab
		tabIndex := "-1"
		if selected {
			tabIndex = "0"
		}
		tabNodes = append(tabNodes, h.Button(h.Type("button"), h.Id("review-tab-"+tab.id), h.Attr("role", "tab"), h.Attr("data-review-tab", tab.id), h.Attr("data-review-focus", "tab-"+tab.id), h.Attr("aria-controls", "review-panel-"+tab.id), h.Attr("aria-selected", fmt.Sprint(selected)), h.Attr("tabindex", tabIndex), h.T(tab.label)))
	}
	panel := func(id string, hidden bool, content *h.Node) *h.Node {
		nodes := []*h.Node{h.Id("review-panel-" + id), h.Attr("role", "tabpanel"), h.Attr("aria-labelledby", "review-tab-"+id), h.Attr("data-review-panel", id), content}
		if hidden {
			nodes = append(nodes, h.Attr("hidden", ""))
		}
		return h.Section(nodes...)
	}
	intro := "These rules apply before the first hand."
	if !lobby {
		intro = fmt.Sprintf("Hand %d · %d completed", message.View.Public.HandNumber, message.View.Public.CompletedHands)
	}
	actions := []*h.Node{h.Class("review-actions")}
	if lobby && host {
		start := []*h.Node{h.Id("review-start-wrap"), h.Attr("hidden", ""), h.Form(h.Id("review-start-form"), h.Attr("ws-send", ""), h.Input(h.Type("hidden"), h.Name("type"), h.Value("start")), h.Input(h.Type("hidden"), h.Name("revision"), h.Value("0")), reviewStartButton(len(message.Seats) < 2))}
		actions = append(actions, h.Div(start...))
	}
	if lobby && !host && len(message.Seats) > 0 {
		actions = append(actions, h.P(h.Class("muted"), h.T(message.Seats[0].Name+" is the host and can start the game.")))
	}
	closeLabel := "Back to game"
	if lobby {
		closeLabel = "Return to lobby"
	}
	actions = append(actions, h.Button(h.Type("button"), h.Attr("data-action", "review-close"), h.Attr("data-review-focus", "return"), h.T(closeLabel)))
	return h.Div(
		h.Id("rules-review-content"), h.Class("review-content"), h.Attr("hx-swap-oob", "outerHTML"),
		h.Attr("data-kind", message.Type), h.Attr("data-update", fmt.Sprint(message.Sequence)), h.Attr("data-host", fmt.Sprint(host)),
		h.P(h.Class("review-context"), h.T(intro)),
		h.NewNode("div", tabNodes),
		panel("current", defaultTab != "current", renderCurrentRules(current, catalog, lobby)),
		panel("starting", defaultTab != "starting", renderStartingRules(starting, catalog)),
		panel("changes", defaultTab != "changes", renderRuleChanges(current, catalog)),
		h.Div(actions...),
	), nil
}

func reviewStartButton(disabled bool) *h.Node {
	nodes := []*h.Node{h.Type("submit"), h.Attr("data-review-focus", "start"), h.T("Start game")}
	if disabled {
		nodes = append(nodes, h.Attr("disabled", ""))
	}
	return h.Button(nodes...)
}

func renderCurrentRules(resolved domain.RuleResolution, catalog domain.Catalog, lobby bool) *h.Node {
	title := "Rules in effect now"
	if lobby {
		title = "Rules ready for the first hand"
	}
	parts := []*h.Node{h.NewNode("h3", []*h.Node{h.T(title)})}
	for _, group := range reviewCategories {
		entries := []*h.Node{}
		for _, rule := range resolved.History {
			if !rule.Active {
				continue
			}
			definition, ok := catalog.Definition(rule.Instance.DefinitionID)
			if !ok || definition.FieldCategory != group.category {
				continue
			}
			entries = append(entries, renderEffectiveRule(rule, definition, resolved.Effective))
		}
		if len(entries) > 0 {
			parts = append(parts, reviewGroup(group.label, entries))
		}
	}
	return h.Div(parts...)
}

func renderStartingRules(starting domain.RuleResolution, catalog domain.Catalog) *h.Node {
	cards := []*h.Node{}
	for _, rule := range starting.ActiveField {
		definition, ok := catalog.Definition(rule.Instance.DefinitionID)
		if ok {
			cards = append(cards, renderEffectiveRule(rule, definition, starting.Effective))
		}
	}
	parts := []*h.Node{h.NewNode("h3", []*h.Node{h.T("Starting field cards")}), h.P(h.Class("muted"), h.T("These are the two cards visible on the table when play begins.")), h.Div(append([]*h.Node{h.Class("review-start-cards")}, cards...)...), h.NewNode("h3", []*h.Node{h.T("Active starting baselines")})}
	for _, group := range reviewCategories {
		entries := []*h.Node{}
		for _, rule := range starting.History {
			if !rule.Active {
				continue
			}
			definition, ok := catalog.Definition(rule.Instance.DefinitionID)
			if ok && definition.StartingVisibility == domain.Hidden && definition.FieldCategory == group.category {
				entries = append(entries, renderEffectiveRule(rule, definition, starting.Effective))
			}
		}
		if len(entries) > 0 {
			parts = append(parts, reviewGroup(group.label, entries))
		}
	}
	return h.Div(parts...)
}

func reviewGroup(label string, entries []*h.Node) *h.Node {
	nodes := []*h.Node{h.Class("review-group"), h.NewNode("h4", []*h.Node{h.T(label)})}
	nodes = append(nodes, entries...)
	return h.Section(nodes...)
}

func renderEffectiveRule(rule domain.ResolvedRule, definition domain.RuleDefinition, effects []domain.EffectiveEffect) *h.Node {
	kind := "Field card"
	if definition.StartingVisibility == domain.Hidden {
		kind = "Hidden baseline"
	}
	nodes := []*h.Node{h.Class("review-rule"), h.Attr("data-rule-id", string(definition.ID)), h.Div(h.Class("review-rule-heading"), h.NewNode("strong", []*h.Node{h.T(definition.Title)}), h.Span(h.Class("review-rule-kind"), h.T(kind)))}
	for _, effect := range effects {
		if effect.InstanceID == rule.Instance.InstanceID {
			nodes = append(nodes, h.P(h.T(effectDescription(effect))))
		}
	}
	return h.NewNode("article", nodes)
}

func effectDescription(e domain.EffectiveEffect) string {
	switch e.Mechanic {
	case domain.MechanicDeck:
		return "Use a standard 52-card deck."
	case domain.MechanicHandSize:
		return fmt.Sprintf("Deal %d cards to each player each hand.", e.Number)
	case domain.MechanicUnused:
		return "Undealt cards are out of play for this hand."
	case domain.MechanicScoreOwnership:
		return "Each player keeps an individual score."
	case domain.MechanicTurnOrder:
		return "Players take turns clockwise."
	case domain.MechanicFirstLeader:
		return "The player left of the dealer opens each hand; the dealer rotates each hand."
	case domain.MechanicNextLeader:
		return "The winner leads the next trick."
	case domain.MechanicTrump:
		return "There is no trump suit."
	case domain.MechanicPlayRestriction:
		if e.Value == domain.ValueFollow {
			return "Play the lead suit when you hold it."
		}
		return "You may play any card, even when you hold the lead suit."
	case domain.MechanicWinnerEligibility:
		return "Only cards in the lead suit can win."
	case domain.MechanicRankComparison:
		if e.Value == domain.ValueLow {
			return "The lowest eligible rank wins, from 2 up to ace."
		}
		return "The highest eligible rank wins, with ace high and 2 low."
	case domain.MechanicTieBreak:
		return "If eligible ranks tie, the earlier play wins."
	case domain.MechanicTrickPoints:
		return "Each captured trick scores one point."
	case domain.MechanicHandLimit:
		return fmt.Sprintf("The match ends after %d completed hands.", e.Number)
	case domain.MechanicVictory:
		return "The highest individual score wins; tied leaders share victory."
	case domain.MechanicReveal:
		return fmt.Sprintf("After every %d completed hands, reveal one eligible rule for the next hand. If none remains eligible, nothing changes. No rule is revealed after the final hand.", e.Number)
	}
	return string(e.Value)
}

func renderRuleChanges(current domain.RuleResolution, catalog domain.Catalog) *h.Node {
	byInstance := map[string]domain.RuleDefinition{}
	for _, rule := range current.History {
		if definition, ok := catalog.Definition(rule.Instance.DefinitionID); ok {
			byInstance[rule.Instance.InstanceID] = definition
		}
	}
	entries := []*h.Node{}
	for _, rule := range current.History {
		if rule.Instance.Starting && len(rule.OverriddenBy) == 0 && rule.InactiveReason == "" {
			continue
		}
		definition, ok := catalog.Definition(rule.Instance.DefinitionID)
		if !ok {
			continue
		}
		label := "Added rule"
		if rule.Instance.Starting {
			label = "Starting rule"
		} else if rule.Instance.IntroducedAfterHand > 0 {
			label = fmt.Sprintf("Revealed after hand %d", rule.Instance.IntroducedAfterHand)
		}
		status := "Active"
		if rule.Active && len(rule.ActiveEffects) < len(definition.Effects) {
			status = "Partially active"
		}
		if !rule.Active {
			switch rule.InactiveReason {
			case "superseded":
				status = "Superseded"
			case "removed":
				status = "Removed"
			case "prerequisite_inactive":
				status = "Inactive: prerequisite missing"
			default:
				status = "Inactive"
			}
		}
		nodes := []*h.Node{h.Class("review-change"), h.Attr("data-rule-id", string(definition.ID)), h.NewNode("strong", []*h.Node{h.T(definition.Title)}), h.P(h.Class("muted"), h.T(label+" · "+status))}
		if len(rule.ActiveEffects) > 0 {
			nodes = append(nodes, h.P(h.T("Still controls: "+mechanicNames(rule.ActiveEffects))))
		}
		if len(rule.OverriddenBy) > 0 {
			names := []string{}
			for _, id := range rule.OverriddenBy {
				if d, ok := byInstance[id]; ok {
					names = append(names, d.Title)
				}
			}
			nodes = append(nodes, h.P(h.T("Replaced by: "+strings.Join(names, ", "))))
		}
		replaced := []string{}
		for _, prior := range current.History {
			for _, id := range prior.OverriddenBy {
				if id == rule.Instance.InstanceID {
					if d, ok := byInstance[prior.Instance.InstanceID]; ok {
						replaced = append(replaced, d.Title)
					}
				}
			}
		}
		if len(replaced) > 0 {
			nodes = append(nodes, h.P(h.T("Replaces: "+strings.Join(replaced, ", "))))
		}
		if rule.InactiveReason == "prerequisite_inactive" {
			names := []string{}
			for _, id := range definition.Prerequisites {
				if d, ok := catalog.Definition(id); ok {
					names = append(names, d.Title)
				}
			}
			nodes = append(nodes, h.P(h.T("Requires an active "+strings.Join(names, ", ")+" rule.")))
		}
		entries = append(entries, h.NewNode("article", nodes))
	}
	if len(entries) == 0 {
		return h.Div(h.NewNode("h3", []*h.Node{h.T("Changes")}), h.P(h.T("No rule changes yet.")))
	}
	return h.Div(append([]*h.Node{h.NewNode("h3", []*h.Node{h.T("Changes")})}, entries...)...)
}

func mechanicNames(effects []domain.Mechanic) string {
	names := make([]string, len(effects))
	for i, e := range effects {
		names[i] = strings.ReplaceAll(string(e), "_", " ")
	}
	return strings.Join(names, ", ")
}
