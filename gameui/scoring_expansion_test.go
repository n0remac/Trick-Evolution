package gameui

import (
	"strings"
	"testing"

	"github.com/n0remac/trick-evolution/domain"
)

func TestScoringExpansionSignedBreakdownAndRuleDetails(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.InitialRules = []domain.RuleID{domain.RuleAvoidQueenHearts, domain.RuleAvoidHearts, domain.RuleLastTrickBonus}
	state, catalog := reviewFixture(t, cfg)
	selector := reviewHTML(t, reviewMessage(nil), "host", catalog, domain.DefaultConfig())
	for _, id := range []domain.RuleID{domain.RuleAvoidQueenHearts, domain.RuleAvoidHearts, domain.RuleLastTrickBonus} {
		if !strings.Contains(selector, `data-initial-rule="`+string(id)+`"`) {
			t.Fatalf("pregame selector omitted %s", id)
		}
	}
	view := fieldView(t, state, catalog)
	field := renderRuleField(view.Public, catalog).Render()
	for _, phrase := range []string{"Avoid Queen of Hearts", "Avoid Hearts", "Last Trick Bonus", "No prerequisite rules"} {
		if !strings.Contains(field, phrase) {
			t.Fatalf("field omitted %q", phrase)
		}
	}
	view.Public.Phase = domain.PhaseHandComplete
	view.Public.HandAwards = []domain.HandAward{
		{Rule: domain.RuleAvoidQueenHearts, Title: "Avoid Queen of Hearts", Seat: "host", Card: "std:H:12", Trick: 1, Points: -5},
		{Rule: domain.RuleAvoidHearts, Title: "Avoid Hearts", Seat: "host", Card: "std:H:12", Trick: 1, Points: -1},
		{Rule: domain.RuleLastTrickBonus, Title: "Last Trick Bonus", Seat: "host", Trick: 5, Points: 2},
	}
	message := reviewMessage(&view)
	html := gamePanel(message, "host", catalog).Render()
	for _, phrase := range []string{"Avoid Queen of Hearts · Alice -5", "Avoid Hearts · Alice -1", "Last Trick Bonus · Alice +2", "Q♥", "Alice · hand total -4"} {
		if !strings.Contains(html, phrase) {
			t.Fatalf("score breakdown omitted %q", phrase)
		}
	}
}
