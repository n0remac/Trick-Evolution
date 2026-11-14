package gameui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/n0remac/trick-evolution/domain"
)

func fieldFixture(t *testing.T) (domain.MatchState, domain.Catalog) {
	t.Helper()
	return reviewFixture(t, domain.DefaultConfig())
}

func appendFieldRule(state *domain.MatchState, id domain.RuleID, instance string) {
	state.Field = append(state.Field, domain.RuleInstance{InstanceID: instance, DefinitionID: id, Sequence: len(state.Field), IntroducedAfterHand: 3})
}

func fieldView(t *testing.T, state domain.MatchState, catalog domain.Catalog) domain.PlayerView {
	t.Helper()
	view, err := domain.ProjectForSeat(state, catalog, "host")
	if err != nil {
		t.Fatal(err)
	}
	return view
}

func TestRuleFieldStartingZonesAndDetails(t *testing.T) {
	state, catalog := fieldFixture(t)
	view := fieldView(t, state, catalog)
	html := renderRuleField(view.Public, catalog).Render()
	for _, category := range []string{`data-category="hand"`, `data-category="play"`, `data-category="trick"`, `data-category="score_victory"`} {
		if !strings.Contains(html, category) {
			t.Fatalf("missing category %s", category)
		}
	}
	if strings.Count(html, `class="rule rule-card"`) != 2 || !strings.Contains(html, "Follow Suit") || !strings.Contains(html, "High Card Wins") {
		t.Fatal("starting field must show precisely two cards")
	}
	if strings.Count(html, "Base rules only") != 2 || strings.Contains(html, "Standard Deck") {
		t.Fatal("empty zones or hidden baselines rendered incorrectly")
	}
	if !strings.Contains(html, "Follow suit when able") || !strings.Contains(html, "Lead suit can win") || !strings.Contains(html, "highest eligible rank") || !strings.Contains(html, "No prerequisite rules") {
		t.Fatal("field text and detail do not use effective mechanics")
	}
	for _, card := range state.Hands["guest"] {
		if strings.Contains(html, string(card)) {
			t.Fatalf("opponent card leaked into field: %s", card)
		}
	}
}

func TestRuleFieldReplacementAdditiveAndReconnect(t *testing.T) {
	state, catalog := fieldFixture(t)
	const tie domain.RuleID = "test.visible_tie"
	catalog.Definitions = append(catalog.Definitions, domain.RuleDefinition{ID: tie, Title: "Tie Fixture", Text: "Earlier play wins.", Category: "rule", Status: domain.StatusSupported, FieldCategory: domain.FieldTrick, Effects: []domain.TypedEffect{{Mechanic: domain.MechanicTieBreak, Value: domain.ValueFirstPlay}}, Overrides: []domain.RuleID{domain.RuleTieBreak}, StartingVisibility: domain.NotStarting})
	appendFieldRule(&state, domain.RuleLowCardWins, "low")
	appendFieldRule(&state, tie, "tie")
	appendFieldRule(&state, domain.RuleHighCardWins, "high-again")
	view := fieldView(t, state, catalog)
	ordered := fieldOrder(view.Public, catalog)
	ids := []string{}
	for _, rule := range ordered {
		ids = append(ids, rule.Instance.InstanceID)
	}
	if !reflect.DeepEqual(ids, []string{"tie", "rule:initial:13", "high-again"}) {
		// The starting Follow Suit instance is catalog index 13. The tie
		// replacement inherits its hidden baseline slot; the final high card
		// inherits the first high card's slot.
		t.Fatalf("unexpected stable rule positions: %v", ids)
	}
	html := renderRuleField(view.Public, catalog).Render()
	if strings.Count(html, `class="rule rule-card"`) != 3 || strings.Contains(html, `data-rule-title="Low Card Wins"`) || !strings.Contains(html, "Tie Fixture") || !strings.Contains(html, "Replaces: Low Card Wins") {
		t.Fatal("field did not retain only active replacements/additions")
	}
	reconnected := fieldView(t, state, catalog)
	if !reflect.DeepEqual(fieldOrder(reconnected.Public, catalog), ordered) || !reflect.DeepEqual(reconnected.Public.Effective, view.Public.Effective) {
		t.Fatal("reconnect changed field order or effective content")
	}
	changes := reviewHTML(t, reviewMessage(&view), "host", catalog, domain.DefaultConfig())
	if !strings.Contains(changes, "Low Card Wins") || !strings.Contains(changes, "Superseded") {
		t.Fatal("superseded card missing from shared history")
	}
}

func TestRuleFieldPartialAndInactiveDependency(t *testing.T) {
	state, catalog := fieldFixture(t)
	const eligibility domain.RuleID = "test.eligibility"
	catalog.Definitions = append(catalog.Definitions, domain.RuleDefinition{ID: eligibility, Title: "Eligibility Fixture", Text: "Lead suit wins.", Category: "rule", Status: domain.StatusSupported, FieldCategory: domain.FieldTrick, Effects: []domain.TypedEffect{{Mechanic: domain.MechanicWinnerEligibility, Value: domain.ValueLeadSuit}}, Overrides: []domain.RuleID{domain.RuleFollowSuit}, StartingVisibility: domain.NotStarting})
	appendFieldRule(&state, eligibility, "eligibility")
	view := fieldView(t, state, catalog)
	html := renderRuleField(view.Public, catalog).Render()
	if !strings.Contains(html, "Partly replaced") || !strings.Contains(html, "winner eligibility is no longer controlled by this card") || !strings.Contains(html, "Eligibility Fixture controls it now") {
		t.Fatal("partial replacement detail is stale")
	}
	state, catalog = fieldFixture(t)
	for i := range catalog.Definitions {
		if catalog.Definitions[i].ID == domain.RuleLowCardWins {
			catalog.Definitions[i].Prerequisites = []domain.RuleID{domain.RuleNoFollowRequired}
		}
	}
	appendFieldRule(&state, domain.RuleNoFollowRequired, "any")
	appendFieldRule(&state, domain.RuleLowCardWins, "low-dependent")
	appendFieldRule(&state, domain.RuleFollowSuit, "follow-again")
	view = fieldView(t, state, catalog)
	html = renderRuleField(view.Public, catalog).Render()
	if strings.Contains(html, "Low Card Wins") || !strings.Contains(html, "High Card Wins") {
		t.Fatal("inactive dependent card shown as effective")
	}
	changes := reviewHTML(t, reviewMessage(&view), "host", catalog, domain.DefaultConfig())
	if !strings.Contains(changes, "Inactive: prerequisite missing") {
		t.Fatal("inactive dependency missing from history")
	}
}

func TestRuleFieldWrapFixtureKeepsMultipleCardsInZone(t *testing.T) {
	state, catalog := fieldFixture(t)
	fixtures := []struct {
		id       domain.RuleID
		baseline domain.RuleID
		mechanic domain.Mechanic
		value    domain.EffectValue
		config   domain.ConfigKey
	}{
		{"test.deck", domain.RuleDeckStandard, domain.MechanicDeck, domain.ValueStandardDeck, ""},
		{"test.hand_size", domain.RuleHandSize, domain.MechanicHandSize, domain.ValueConfigured, "hand_size"},
		{"test.unused", domain.RuleUnusedCards, domain.MechanicUnused, domain.ValueOutOfPlay, ""},
	}
	for _, fixture := range fixtures {
		catalog.Definitions = append(catalog.Definitions, domain.RuleDefinition{ID: fixture.id, Title: string(fixture.id), Text: "Fixture effect", Category: "rule", Status: domain.StatusSupported, FieldCategory: domain.FieldHand, Effects: []domain.TypedEffect{{Mechanic: fixture.mechanic, Value: fixture.value, Config: fixture.config}}, Overrides: []domain.RuleID{fixture.baseline}, StartingVisibility: domain.NotStarting})
		appendFieldRule(&state, fixture.id, string(fixture.id))
	}
	view := fieldView(t, state, catalog)
	html := renderRuleField(view.Public, catalog).Render()
	if strings.Count(html, `class="rule rule-card"`) != 5 || strings.Count(html, `data-category="hand"`) != 1 {
		t.Fatal("synthetic field did not retain all cards in stable zones")
	}
	for _, fixture := range fixtures {
		if !strings.Contains(html, `data-rule-title="`+string(fixture.id)+`"`) {
			t.Fatalf("missing visible fixture %s", fixture.id)
		}
	}
}

func TestRevealCueNamesNextHand(t *testing.T) {
	catalog := domain.DefaultCatalog()
	got := eventNotice([]domain.Event{{Kind: domain.EventRuleRevealed, Rule: domain.RuleLowCardWins, Hand: 3}}, catalog, func(domain.SeatID) string { return "" })
	if !strings.Contains(got, "applies from hand 4") {
		t.Fatalf("reveal timing ambiguous: %s", got)
	}
}

func TestRuleFieldAndReviewShareLiveProjection(t *testing.T) {
	state, catalog := fieldFixture(t)
	appendFieldRule(&state, domain.RuleLowCardWins, "low-reveal")
	view := fieldView(t, state, catalog)
	message := reviewMessage(&view)
	message.Events = []domain.Event{{Kind: domain.EventRuleRevealed, Rule: domain.RuleLowCardWins, Hand: 3}}
	html := renderScreen(message, "host", catalog, domain.DefaultConfig())
	if !strings.Contains(html, `data-rule-title="Low Card Wins"`) || strings.Contains(html, `data-rule-title="High Card Wins"`) || !strings.Contains(html, "applies from hand 4") {
		t.Fatal("field and reveal cue disagree with current projection")
	}
	if !strings.Contains(html, "Replaced by: Low Card Wins") || !strings.Contains(html, "Superseded") || !strings.Contains(html, "The lowest eligible rank wins") {
		t.Fatal("Game Rules current/history disagree with field")
	}
	for _, card := range state.Hands["guest"] {
		if strings.Contains(html, string(card)) {
			t.Fatalf("opponent card leaked into live projection: %s", card)
		}
	}
}
