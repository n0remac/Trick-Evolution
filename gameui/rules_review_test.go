package gameui

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/n0remac/trick-evolution/domain"
)

func reviewFixture(t *testing.T, config domain.Config) (domain.MatchState, domain.Catalog) {
	t.Helper()
	catalog := domain.DefaultCatalog()
	state, err := domain.NewMatch([]domain.SeatID{"host", "guest"}, config, catalog, domain.SeededShuffle(81))
	if err != nil {
		t.Fatal(err)
	}
	return state, catalog
}
func reviewMessage(view *domain.PlayerView) serverMessage {
	kind := "lobby"
	revision := uint64(0)
	if view != nil {
		kind = "state"
		revision = view.Public.Revision
	}
	return serverMessage{Type: kind, Room: "review", Sequence: 2, Revision: revision, View: view, Seats: []seatSummary{{ID: "host", Name: "Alice", Host: true, Online: true}, {ID: "guest", Name: "Bob", Online: true}}}
}
func reviewHTML(t *testing.T, message serverMessage, seat domain.SeatID, catalog domain.Catalog, config domain.Config) string {
	t.Helper()
	node, err := rulesReviewContent(message, seat, catalog, config)
	if err != nil {
		t.Fatal(err)
	}
	return node.Render()
}
func reviewPanel(t *testing.T, html, id, next string) string {
	t.Helper()
	start := strings.Index(html, `id="review-panel-`+id+`"`)
	if start < 0 {
		t.Fatal("missing panel", id)
	}
	end := strings.Index(html[start:], `id="review-panel-`+next+`"`)
	if end < 0 {
		t.Fatal("missing next panel", next)
	}
	return html[start : start+end]
}

func TestRulesReviewPreviewMatchesActualConfiguration(t *testing.T) {
	config := domain.Config{HandSize: 7, MaxHands: 8, RevealEvery: 4}
	state, catalog := reviewFixture(t, config)
	starting, err := domain.StartingRules(config, catalog)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := domain.ProjectPublic(state, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(starting.History, actual.Rules) || !reflect.DeepEqual(starting.Effective, actual.Effective) || !reflect.DeepEqual(starting.ActiveField, actual.ActiveField) {
		t.Fatal("preview diverges from started match")
	}
	html := reviewHTML(t, reviewMessage(nil), "host", catalog, config)
	if !strings.Contains(html, "Deal 7 cards to each player") || !strings.Contains(html, "after 8 completed hands") || !strings.Contains(html, "every 4 completed hands") {
		t.Fatal("configured numbers missing", html)
	}
	startingPanel := reviewPanel(t, html, "starting", "changes")
	if !strings.Contains(startingPanel, "Starting field cards") || !strings.Contains(startingPanel, "Follow Suit") || !strings.Contains(startingPanel, "High Card Wins") || strings.Count(startingPanel, `class="review-rule-heading"`) < 2 {
		t.Fatal("starting cards missing")
	}
	for _, label := range []string{"Hand", "Play", "Trick", "Score &amp; Victory", "Hidden baseline", "Undealt cards", "clockwise", "left of the dealer", "winner leads", "no trump", "eligible rank", "earlier play", "one point", "share victory", "none remains eligible"} {
		if !strings.Contains(strings.ToLower(startingPanel), strings.ToLower(label)) {
			t.Fatalf("starting explanation missing %q", label)
		}
	}
	startInput := regexp.MustCompile(`<input[^>]*(name="type"[^>]*value="start"|value="start"[^>]*name="type")`)
	if !strings.Contains(html, `id="review-start-form"`) || !startInput.MatchString(html) {
		t.Fatal("host start action missing")
	}
	guest := reviewHTML(t, reviewMessage(nil), "guest", catalog, config)
	if strings.Contains(guest, `id="review-start-form"`) || !strings.Contains(guest, "Alice is the host") || !strings.Contains(guest, "Return to lobby") {
		t.Fatal("guest received host start action")
	}
}

func TestRulesReviewCurrentHistoryAndPrivacy(t *testing.T) {
	config := domain.DefaultConfig()
	state, catalog := reviewFixture(t, config)
	for _, id := range []domain.RuleID{domain.RuleNoFollowRequired, domain.RuleLowCardWins} {
		state.Field = append(state.Field, domain.RuleInstance{InstanceID: "fixture:" + string(id), DefinitionID: id, Sequence: len(state.Field), IntroducedAfterHand: 3})
	}
	view, err := domain.ProjectForSeat(state, catalog, "host")
	if err != nil {
		t.Fatal(err)
	}
	html := reviewHTML(t, reviewMessage(&view), "host", catalog, config)
	current := reviewPanel(t, html, "current", "starting")
	if !strings.Contains(current, "No Follow Required") || !strings.Contains(current, "Low Card Wins") || strings.Contains(current, "High Card Wins") || strings.Contains(current, "Follow Suit") {
		t.Fatal("current panel includes superseded cards")
	}
	if !strings.Contains(current, "lowest eligible rank") || !strings.Contains(current, "Only cards in the lead suit can win") {
		t.Fatal("current explanations disagree with effects")
	}
	changes := html[strings.Index(html, `id="review-panel-changes"`):]
	if !strings.Contains(changes, "Superseded") || !strings.Contains(changes, "Replaces: High Card Wins") || !strings.Contains(changes, "Replaced by: Low Card Wins") {
		t.Fatal("replacement history missing")
	}
	for _, card := range state.Hands["guest"] {
		if strings.Contains(html, string(card)) {
			t.Fatalf("opponent private card leaked: %s", card)
		}
	}
	for _, card := range state.StandardDraw {
		if strings.Contains(html, string(card)) {
			t.Fatalf("future standard card leaked: %s", card)
		}
	}
}

func TestRulesReviewHiddenBaselineAndInactiveDependency(t *testing.T) {
	config := domain.DefaultConfig()
	state, catalog := reviewFixture(t, config)
	const deckFixture domain.RuleID = "test.deck_fixture"
	catalog.Definitions = append(catalog.Definitions, domain.RuleDefinition{ID: deckFixture, Title: "Deck Fixture", Text: "Standard deck fixture", Category: "rule", Status: domain.StatusSupported, FieldCategory: domain.FieldHand, Effects: []domain.TypedEffect{{Mechanic: domain.MechanicDeck, Value: domain.ValueStandardDeck}}, Overrides: []domain.RuleID{domain.RuleDeckStandard}, StartingVisibility: domain.NotStarting})
	for i := range catalog.Definitions {
		if catalog.Definitions[i].ID == domain.RuleLowCardWins {
			catalog.Definitions[i].Prerequisites = []domain.RuleID{domain.RuleNoFollowRequired}
		}
	}
	for _, id := range []domain.RuleID{deckFixture, domain.RuleNoFollowRequired, domain.RuleLowCardWins, domain.RuleFollowSuit} {
		state.Field = append(state.Field, domain.RuleInstance{InstanceID: "fixture:" + string(id), DefinitionID: id, Sequence: len(state.Field), IntroducedAfterHand: 3})
	}
	view, err := domain.ProjectForSeat(state, catalog, "host")
	if err != nil {
		t.Fatal(err)
	}
	html := reviewHTML(t, reviewMessage(&view), "host", catalog, config)
	current := reviewPanel(t, html, "current", "starting")
	if !strings.Contains(current, "Deck Fixture") || strings.Contains(current, "Standard Deck") || strings.Contains(current, "Low Card Wins") {
		t.Fatal("inactive/hidden baseline shown as effective")
	}
	changes := html[strings.Index(html, `id="review-panel-changes"`):]
	if !strings.Contains(changes, "Standard Deck") || !strings.Contains(changes, "Replaced by: Deck Fixture") || !strings.Contains(changes, "Inactive: prerequisite missing") || !strings.Contains(changes, "Requires an active No Follow Required rule") {
		t.Fatal("hidden replacement or dependent status missing")
	}
}

func TestRulesReviewAfterEndAndReconnect(t *testing.T) {
	config := domain.DefaultConfig()
	state, catalog := reviewFixture(t, config)
	ended, _, err := domain.Step(state, domain.Action{Kind: domain.ActionEndGame, Seat: "host"}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	view, err := domain.ProjectForSeat(ended, catalog, "guest")
	if err != nil {
		t.Fatal(err)
	}
	first := reviewHTML(t, reviewMessage(&view), "guest", catalog, config)
	reconnected, err := domain.ProjectForSeat(ended, catalog, "guest")
	if err != nil || !reflect.DeepEqual(view, reconnected) {
		t.Fatal("authoritative reconnect projection changed", err)
	}
	second := reviewHTML(t, reviewMessage(&reconnected), "guest", catalog, config)
	for _, html := range []string{first, second} {
		if !strings.Contains(html, "Back to game") || !strings.Contains(html, "No rule changes yet") || !strings.Contains(html, "Current rules") {
			t.Fatal("ended-game review lost current rules or close action")
		}
	}
}

func TestPregameSettingsReviewAndLockedStartingRules(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.HandSize = 2
	cfg.RevealEvery = 1
	cfg.InitialRules = []domain.RuleID{domain.RuleSpadesTrump, domain.RuleLowTrump, domain.RuleKeepLow, domain.RuleNoPoints}
	state, catalog := reviewFixture(t, cfg)
	lobby := reviewHTML(t, reviewMessage(nil), "host", catalog, cfg)
	for _, want := range []string{`id="review-settings-form"`, `id="review-reset-form"`, `data-initial-rule="rule.low_trump_scores"`, `Completed hands between reveals`, `Keep Low`} {
		if !strings.Contains(lobby, want) {
			t.Fatalf("settings missing %s", want)
		}
	}
	guest := reviewHTML(t, reviewMessage(nil), "guest", catalog, cfg)
	if strings.Contains(guest, `id="review-settings-form"`) {
		t.Fatal("guest can edit settings")
	}
	view, err := domain.ProjectForSeat(state, catalog, "host")
	if err != nil {
		t.Fatal(err)
	}
	started := reviewHTML(t, reviewMessage(&view), "host", catalog, cfg)
	if strings.Contains(started, `id="review-settings-form"`) || !strings.Contains(started, "locked after play begins") || !strings.Contains(started, "original holder of Low Trump") {
		t.Fatal("started rules not locked or low policy missing")
	}
}

func TestRulesReviewDisplaysSavedRoomSettingsStatus(t *testing.T) {
	cfg := domain.DefaultConfig()
	_, catalog := reviewFixture(t, cfg)
	message := reviewMessage(nil)
	message.SettingsStatus = "Settings saved for this room. They apply when the game starts."
	html := reviewHTML(t, message, "host", catalog, cfg)
	if !strings.Contains(html, `id="review-settings-feedback"`) || !strings.Contains(html, message.SettingsStatus) || !strings.Contains(html, `data-setting="hand-size"`) {
		t.Fatal("saved settings feedback missing")
	}
}
