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
	if strings.Contains(selector, `data-initial-rule="`+string(domain.RulePointsPerTrick)+`"`) {
		t.Fatal("redundant restoration offered as a starting card")
	}
	for _, id := range []domain.RuleID{domain.RuleAvoidQueenHearts, domain.RuleAvoidHearts, domain.RuleLastTrickBonus} {
		if !strings.Contains(selector, `data-initial-rule="`+string(id)+`"`) {
			t.Fatalf("pregame selector omitted %s", id)
		}
	}
	view := fieldView(t, state, catalog)
	field := renderRuleField(view.Public, catalog).Render()
	for _, phrase := range []string{"Avoid Queen of Hearts", "Avoid Hearts", "Last Trick Bonus"} {
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
	view.Public.LastScorecard = &domain.Scorecard{HandNumber: view.Public.HandNumber, Players: []domain.ScorecardPlayer{{Seat: "host", Total: -4, Awards: view.Public.HandAwards}, {Seat: "guest"}}}
	message := reviewMessage(&view)
	html := gamePanel(message, "host", catalog).Render()
	for _, phrase := range []string{"Avoid Queen of Hearts · -5", "Avoid Hearts · -1", "Last Trick Bonus · +2", "Q♥", "Alice · hand total -4", "scorecard-visible", "scorecard-saved"} {
		if !strings.Contains(html, phrase) {
			t.Fatalf("score breakdown omitted %q", phrase)
		}
	}
}

func TestScorecardPitchMathTieAndNoAwardExplanation(t *testing.T) {
	card := &domain.Scorecard{HandNumber: 4, Round: 2, Players: []domain.ScorecardPlayer{
		{Seat: "a", Total: 2, Awards: []domain.HandAward{{Rule: domain.RuleHighTrump, Title: "High Trump Scores", Card: "std:S:14", Points: 1}, {Rule: domain.RuleLastTrickBonus, Title: "Last Trick Bonus", Trick: 5, Points: 2}, {Rule: domain.RuleAvoidHearts, Title: "Avoid Hearts", Card: "std:H:12", Trick: 3, Points: -1}}, GameTally: &domain.GameTally{Cards: []domain.GameTallyCard{{Label: "10♥", Value: 10}, {Label: "A♠", Value: 4}, {Label: "J♣", Value: 1}}, Total: 15}},
		{Seat: "b", Total: 0, GameTally: &domain.GameTally{Cards: []domain.GameTallyCard{{Label: "10♦", Value: 10}, {Label: "K♣", Value: 3}, {Label: "Q♣", Value: 2}}, Total: 15}},
	}, Notes: []domain.ScorecardNote{{Rule: domain.RuleGamePoint, Text: "Game Point tied at 15 tally; no match point awarded."}, {Rule: domain.RuleJackTrump, Text: "Trump Jack was not dealt; no point awarded."}}}
	name := func(id domain.SeatID) string {
		if id == "a" {
			return "Alice"
		}
		return "Bob"
	}
	html := renderScorecard(card, name, "scorecard-visible").Render()
	for _, phrase := range []string{"Alice · hand total +2", "Bob · hand total +0", "High Trump Scores", "Last Trick Bonus", "Avoid Hearts · -1", "10♥=10", "A♠=4", "J♣=1", "Game tally:", "= 15", "10=10, A=4, K=3, Q=2, J=1", "tied at 15", "Trump Jack was not dealt"} {
		if !strings.Contains(html, phrase) {
			t.Fatalf("scorecard omitted %q", phrase)
		}
	}
}

func TestScorecardGamePointSeparatesTallyFromMatchPoint(t *testing.T) {
	card := &domain.Scorecard{HandNumber: 1, Players: []domain.ScorecardPlayer{{Seat: "a", Total: 1, Awards: []domain.HandAward{{Rule: domain.RuleGamePoint, Title: "Game Point", Seat: "a", Tally: 20, Points: 1}}, GameTally: &domain.GameTally{Cards: []domain.GameTallyCard{{Label: "10♥", Value: 10}, {Label: "10♦", Value: 10}}, Total: 20}}, {Seat: "b", GameTally: &domain.GameTally{Total: 0}}}}
	html := renderScorecard(card, func(id domain.SeatID) string { return string(id) }, "scorecard-visible").Render()
	for _, phrase := range []string{"Game Point · +1 match point (captured-card tally 20)", "10♥=10 + 10♦=10 = 20", "no scoring cards = 0"} {
		if !strings.Contains(html, phrase) {
			t.Fatalf("Game Point display omitted %q", phrase)
		}
	}
}
