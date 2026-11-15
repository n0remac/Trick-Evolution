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
			config = domain.Config{HandSize: public.HandSize, MaxHands: public.MaxHands, RevealEvery: public.RevealEvery, InitialRules: public.InitialRules}
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
		panel("current", defaultTab != "current", renderCurrentRules(current, catalog, lobby, message.View)),
		panel("starting", defaultTab != "starting", renderStartingRules(starting, catalog, config, lobby, host, message.SettingsStatus)),
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

func renderCurrentRules(resolved domain.RuleResolution, catalog domain.Catalog, lobby bool, view *domain.PlayerView) *h.Node {
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
			status := ""
			if view != nil {
				for _, effect := range resolved.Effective {
					if effect.InstanceID == rule.Instance.InstanceID && (effect.Mechanic == domain.MechanicTrump || effect.Mechanic == domain.MechanicBreakSpades || effect.Mechanic == domain.MechanicBreakHearts) {
						status = shortEffect(effect, rule, view.Public)
					}
				}
			}
			entries = append(entries, renderEffectiveRule(rule, definition, resolved.Effective, status))
		}
		if len(entries) > 0 {
			parts = append(parts, reviewGroup(group.label, entries))
		}
	}
	return h.Div(parts...)
}

func renderStartingRules(starting domain.RuleResolution, catalog domain.Catalog, config domain.Config, lobby, host bool, settingsStatus string) *h.Node {
	cards := []*h.Node{}
	for _, rule := range starting.ActiveField {
		definition, ok := catalog.Definition(rule.Instance.DefinitionID)
		if ok {
			cards = append(cards, renderEffectiveRule(rule, definition, starting.Effective, ""))
		}
	}
	parts := []*h.Node{h.NewNode("h3", []*h.Node{h.T("Starting field cards")}), h.P(h.Class("muted"), h.T("The selected visible cards are shown on the table when play begins. Hidden baselines still apply where no card replaces them.")), h.Div(append([]*h.Node{h.Class("review-start-cards")}, cards...)...), h.NewNode("h3", []*h.Node{h.T("Active starting baselines")})}
	for _, group := range reviewCategories {
		entries := []*h.Node{}
		for _, rule := range starting.History {
			if !rule.Active {
				continue
			}
			definition, ok := catalog.Definition(rule.Instance.DefinitionID)
			if ok && definition.StartingVisibility == domain.Hidden && definition.FieldCategory == group.category {
				entries = append(entries, renderEffectiveRule(rule, definition, starting.Effective, ""))
			}
		}
		if len(entries) > 0 {
			parts = append(parts, reviewGroup(group.label, entries))
		}
	}
	if lobby && host {
		parts = append(parts, renderSettings(config, catalog, settingsStatus))
	}
	if !lobby {
		parts = append(parts, h.P(h.Class("muted"), h.T("Starting settings are locked after play begins.")))
	}
	return h.Div(parts...)
}

func renderSettings(config domain.Config, catalog domain.Catalog, settingsStatus string) *h.Node {
	if settingsStatus == "" {
		settingsStatus = "Save changes before starting. Saved settings apply to this room's next game."
	}
	selected := map[domain.RuleID]bool{}
	for _, id := range config.InitialRules {
		selected[id] = true
	}
	nodes := []*h.Node{h.Id("review-settings-form"), h.Attr("ws-send", ""), h.Class("review-settings"),
		h.H3(h.T("Game settings")), h.P(h.Id("review-settings-feedback"), h.Role("status"), h.Attr("aria-live", "polite"), h.T(settingsStatus)), h.P(h.Class("muted"), h.T("Changes apply to this room. Choose prerequisites before dependent cards; conflicting alternatives cannot both start. Unselected mechanics use their hidden or visible baseline.")),
		h.Input(h.Type("hidden"), h.Name("type"), h.Value("configure")), h.Input(h.Type("hidden"), h.Name("revision"), h.Value("0")), h.Input(h.Type("hidden"), h.Name("settings"), h.Value("")),
		h.Label(h.T("Cards per player"), h.Input(h.Type("number"), h.Attr("data-setting", "hand-size"), h.Attr("min", "1"), h.Attr("max", "26"), h.Value(fmt.Sprint(config.HandSize)))),
		h.Label(h.T("Completed hands between reveals"), h.Input(h.Type("number"), h.Attr("data-setting", "reveal-every"), h.Attr("min", "1"), h.Attr("max", "100"), h.Value(fmt.Sprint(config.RevealEvery)))),
	}
	for _, group := range reviewCategories {
		choices := []*h.Node{}
		for _, d := range catalog.Definitions {
			if d.Status != domain.StatusSupported || d.StartActive || d.FieldCategory != group.category {
				continue
			}
			input := []*h.Node{h.Type("checkbox"), h.Attr("data-initial-rule", string(d.ID)), h.Value(string(d.ID))}
			if selected[d.ID] {
				input = append(input, h.Attr("checked", ""))
			}
			description := d.Text
			if len(d.Prerequisites) > 0 || len(d.PrerequisiteAny) > 0 {
				names := []string{}
				for _, id := range append(append([]domain.RuleID(nil), d.Prerequisites...), d.PrerequisiteAny...) {
					if needed, ok := catalog.Definition(id); ok {
						names = append(names, needed.Title)
					}
				}
				connector := " and "
				if len(d.PrerequisiteAny) > 0 && len(d.Prerequisites) == 0 {
					connector = " or "
				}
				description += " Requires " + strings.Join(names, connector) + "."
			}
			choices = append(choices, h.Label(h.Class("review-setting-rule"), h.Input(input...), h.Span(h.NewNode("strong", []*h.Node{h.T(d.Title)}), h.T(" — "+description))))
		}
		if len(choices) > 0 {
			nodes = append(nodes, reviewGroup(group.label, choices))
		}
	}
	nodes = append(nodes, h.Button(h.Type("submit"), h.T("Save game settings")))
	reset := h.Form(h.Id("review-reset-form"), h.Attr("ws-send", ""), h.Input(h.Type("hidden"), h.Name("type"), h.Value("reset_rules")), h.Input(h.Type("hidden"), h.Name("revision"), h.Value("0")), h.Button(h.Type("submit"), h.T("Reset to defaults")))
	return h.Div(h.Form(nodes...), reset)
}

func reviewGroup(label string, entries []*h.Node) *h.Node {
	nodes := []*h.Node{h.Class("review-group"), h.NewNode("h4", []*h.Node{h.T(label)})}
	nodes = append(nodes, entries...)
	return h.Section(nodes...)
}

func renderEffectiveRule(rule domain.ResolvedRule, definition domain.RuleDefinition, effects []domain.EffectiveEffect, status string) *h.Node {
	kind := "Field card"
	if definition.StartingVisibility == domain.Hidden {
		kind = "Hidden baseline"
	}
	nodes := []*h.Node{h.Class("review-rule"), h.Attr("data-rule-id", string(definition.ID)), h.Div(h.Class("review-rule-heading"), h.NewNode("strong", []*h.Node{h.T(definition.Title)}), h.Span(h.Class("review-rule-kind"), h.T(kind))), h.P(h.T(definition.Text))}
	for _, effect := range effects {
		if effect.InstanceID == rule.Instance.InstanceID {
			nodes = append(nodes, h.P(h.T(effectDescription(effect))))
		}
	}
	if definition.ID == domain.RuleLowTrump {
		for _, effect := range effects {
			if effect.Mechanic == domain.MechanicLowOwner {
				nodes = append(nodes, h.P(h.T(effectDescription(effect))))
				break
			}
		}
	}
	if status != "" {
		nodes = append(nodes, h.P(h.Class("review-rule-status"), h.T(status)))
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
		switch e.Value {
		case domain.ValueSpadesTrump:
			return "Spades are trump. If a trump card is played, only trump cards can win; otherwise the lead suit can win. The active rank and tie rules compare eligible cards."
		case domain.ValueFirstLeadTrump:
			return "The first card led each hand chooses trump for that hand, including its first trick. If trump is played, only trump can win; otherwise the lead suit can win. The normal opening leader still leads."
		}
		return "There is no trump suit."
	case domain.MechanicBreakSpades:
		return "Do not lead spades until a spade has been played on a different suit lead, unless only spades remain. If both active lead bans leave no legal lead, any card may be led. This resets each hand."
	case domain.MechanicBreakHearts:
		return "Do not lead hearts until a heart has been played on a different suit lead, unless only hearts remain. If both active lead bans leave no legal lead, any card may be led. This resets each hand."
	case domain.MechanicPlayRestriction:
		if e.Value == domain.ValueFollow {
			return "Play the lead suit when you hold it."
		}
		if e.Value == domain.ValueFollowOrTrump {
			return "On a nontrump lead, follow suit or play trump when holding the lead suit; if void in the lead suit, play anything. On a trump lead, follow trump when able."
		}
		return "You may play any card, even when you hold the lead suit."
	case domain.MechanicWinnerEligibility:
		return "Only cards in the lead suit can win; active trump overrides this when played."
	case domain.MechanicRankComparison:
		if e.Value == domain.ValueLow {
			return "The lowest eligible rank wins, from 2 up to ace."
		}
		return "The highest eligible rank wins, with ace high and 2 low."
	case domain.MechanicTieBreak:
		return "If eligible ranks tie, the earlier play wins."
	case domain.MechanicTrickPoints:
		if e.Value == domain.ValueNoPoints {
			return "Captured tricks give no direct points."
		}
		return "Each captured trick scores one point."
	case domain.MechanicLowOwner:
		if e.Value == domain.ValueKeepLow {
			return "The original holder of Low Trump receives its point."
		}
		return "The captor of Low Trump receives its point (default)."
	case domain.MechanicHighTrump, domain.MechanicLowTrump, domain.MechanicJackTrump, domain.MechanicGamePoint:
		return "One point at completed-hand settlement when the card or tally qualifies."
	case domain.MechanicAvoidQueenHearts:
		return "Capturing the Queen of Hearts costs 5 match points when its trick completes. This stacks with Avoid Hearts."
	case domain.MechanicAvoidHearts:
		return "Each captured heart costs 1 match point when its trick completes, including the Queen of Hearts."
	case domain.MechanicLastTrickBonus:
		return "The final trick winner gains 2 match points when the hand completes. No bonus is awarded for an unfinished hand."
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
		evolved := rule.Instance.IntroducedAfterHand > 0
		for _, child := range rule.OverriddenBy {
			for _, candidate := range current.History {
				if candidate.Instance.InstanceID == child && candidate.Instance.IntroducedAfterHand > 0 {
					evolved = true
				}
			}
		}
		if !evolved {
			continue
		}
		definition, ok := catalog.Definition(rule.Instance.DefinitionID)
		if !ok {
			continue
		}
		label := "Added rule"
		if rule.Instance.IntroducedAfterHand == 0 {
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
			if len(names) > 0 {
				nodes = append(nodes, h.P(h.T("Requires an active "+strings.Join(names, ", ")+" rule.")))
			}
			if len(definition.PrerequisiteAny) > 0 {
				alternatives := []string{}
				for _, id := range definition.PrerequisiteAny {
					if d, ok := catalog.Definition(id); ok {
						alternatives = append(alternatives, d.Title)
					}
				}
				nodes = append(nodes, h.P(h.T("Requires one active rule: "+strings.Join(alternatives, " or ")+".")))
			}
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
