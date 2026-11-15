package domain

import (
	"reflect"
	"testing"
)

type identityShuffle struct{}

func (identityShuffle) Shuffle(_ int, _ func(i, j int)) {}

// Legacy fixtures exercise the Stage 1 baseline without evolution draws.
func emptyEvolutionCatalog() Catalog {
	c := DefaultCatalog()
	for i := range c.Definitions {
		c.Definitions[i].InDeck = false
	}
	return c
}

func TestStandardDeckUniqueAndStable(t *testing.T) {
	deck := StandardDeck()
	if len(deck) != 52 || !reflect.DeepEqual(deck, StandardDeck()) {
		t.Fatal("standard deck changed")
	}
	seen := map[CardID]bool{}
	for _, card := range deck {
		if seen[card.ID] || card.Rank < 2 || card.Rank > 14 {
			t.Fatalf("invalid card %+v", card)
		}
		seen[card.ID] = true
	}
}

func TestStage1SetupReplayAndProjection(t *testing.T) {
	ids := []SeatID{"north", "east", "south", "west"}
	cfg := DefaultConfig()
	catalog := DefaultCatalog()
	a, err := NewMatch(ids, cfg, catalog, SeededShuffle(73))
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewMatch(ids, cfg, catalog, SeededShuffle(73))
	if err != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("seeded setup diverged", err)
	}
	replay, err := ReplayMatch(ids, cfg, catalog, a.Shuffles)
	if err != nil || !reflect.DeepEqual(a, replay) {
		t.Fatal("replay diverged", err)
	}
	if a.Dealer != "north" || a.Leader != "east" || a.Turn != "east" || len(a.RuleDraw) != len(catalog.RuleDeck()) || len(a.StandardDraw) != 52-cfg.HandSize*len(ids) {
		t.Fatalf("bad setup %+v", a)
	}
	for _, id := range ids {
		if len(a.Hands[id]) != cfg.HandSize {
			t.Fatalf("hand %s", id)
		}
	}
	view, err := ProjectPublic(a, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.ActiveField) != 2 || view.ActiveField[0].Instance.DefinitionID != RuleFollowSuit || view.ActiveField[1].Instance.DefinitionID != RuleHighCardWins {
		t.Fatalf("visible field %+v", view.ActiveField)
	}
	if len(view.Rules) != len(StartingField(catalog)) || len(view.Effective) != 17 {
		t.Fatalf("baseline history/effects %d/%d", len(view.Rules), len(view.Effective))
	}
	for _, want := range []struct {
		mechanic Mechanic
		number   int
	}{{MechanicHandSize, cfg.HandSize}, {MechanicHandLimit, cfg.MaxHands}, {MechanicReveal, cfg.RevealEvery}} {
		r, err := ResolveRules(a.Field, catalog, cfg)
		if err != nil {
			t.Fatal(err)
		}
		effect, err := r.Effect(want.mechanic)
		if err != nil || effect.Number != want.number {
			t.Fatalf("%s = %+v %v", want.mechanic, effect, err)
		}
	}
	bad := append([]ShuffleRecord(nil), a.Shuffles...)
	bad[0].Order = append([]string(nil), bad[0].Order...)
	bad[0].Order[0] = bad[0].Order[1]
	if _, err := ReplayMatch(ids, cfg, catalog, bad); err == nil {
		t.Fatal("duplicate card accepted")
	}
	old := catalog
	old.Version = 1
	if _, err := ReplayMatch(ids, cfg, old, a.Shuffles); err == nil {
		t.Fatal("old catalog accepted")
	}
	oldRecord := append([]ShuffleRecord(nil), a.Shuffles...)
	oldRecord[1].Order = []string{string(RuleLowCardWins)}
	if _, err := ReplayMatch(ids, cfg, catalog, oldRecord); err == nil {
		t.Fatal("old rule shuffle accepted")
	}
}

func TestInjectedFixtureDeal(t *testing.T) {
	state, err := NewMatch([]SeatID{"a", "b"}, DefaultConfig(), DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(state.Hands["b"], []CardID{"std:H:02", "std:H:04", "std:H:06", "std:H:08", "std:H:10"}) || !reflect.DeepEqual(state.Hands["a"], []CardID{"std:H:03", "std:H:05", "std:H:07", "std:H:09", "std:H:11"}) {
		t.Fatalf("deal changed %+v", state.Hands)
	}
	if len(state.RuleDraw) != len(DefaultCatalog().RuleDeck()) {
		t.Fatal("default evolution deck size changed")
	}
}

func TestCatalogValidation(t *testing.T) {
	catalog := DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Definitions) != 82 {
		t.Fatalf("catalog entries %d", len(catalog.Definitions))
	}
	wantDeck := []RuleID{RuleSpadesTrump, RulePitchTrump, RuleFollowOrTrump, RuleBreakSpades, RuleBreakHearts, RuleHighTrump, RuleLowTrump, RuleCaptureLow, RuleKeepLow, RuleJackTrump, RuleGamePoint, RuleNoPoints, RulePointsPerTrick}
	if !reflect.DeepEqual(catalog.RuleDeck(), wantDeck) {
		t.Fatalf("default evolution deck: %v", catalog.RuleDeck())
	}
	cases := map[string]func(*Catalog){
		"duplicate":      func(c *Catalog) { c.Definitions = append(c.Definitions, c.Definitions[0]) },
		"unknown effect": func(c *Catalog) { c.Definitions[13].Effects[0].Value = "unknown" },
		"unknown target": func(c *Catalog) { c.Definitions[14].Overrides = []RuleID{"missing"} },
		"unsupported draw": func(c *Catalog) {
			for i := range c.Definitions {
				if c.Definitions[i].ID == "rule.must_underplay" {
					c.Definitions[i].InDeck = true
				}
			}
		},
		"starter in draw": func(c *Catalog) {
			for i := range c.Definitions {
				if c.Definitions[i].ID == RuleFollowSuit {
					c.Definitions[i].InDeck = true
				}
			}
		},
		"duplicate alternative prerequisite": func(c *Catalog) {
			for i := range c.Definitions {
				if c.Definitions[i].ID == RuleFollowOrTrump {
					c.Definitions[i].PrerequisiteAny = append(c.Definitions[i].PrerequisiteAny, RuleSpadesTrump)
				}
			}
		},
		"old version": func(c *Catalog) { c.Version = 1 },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			c := DefaultCatalog()
			change(&c)
			if err := c.Validate(); err == nil {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
}

func appendRule(field []RuleInstance, id RuleID) []RuleInstance {
	return append(field, RuleInstance{InstanceID: "test:" + string(id) + ":" + string(rune('a'+len(field))), DefinitionID: id, Sequence: len(field)})
}

func TestOrderedReplacementAndBaselineRestoration(t *testing.T) {
	c := DefaultCatalog()
	s, err := NewMatch([]SeatID{"a", "b"}, DefaultConfig(), c, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	base := len(s.Field)
	s.Field = appendRule(s.Field, RuleLowCardWins)
	s.Field = appendRule(s.Field, RuleHighCardWins)
	r, err := ResolveRules(s.Field, c, s.Config)
	if err != nil {
		t.Fatal(err)
	}
	if r.History[base].Active || !r.History[base+1].Active || r.History[base].InactiveReason != "superseded" {
		t.Fatalf("replacement history %+v", r.History[base:])
	}
	s.Field[base+1].Removed = true
	r, err = ResolveRules(s.Field, c, s.Config)
	if err != nil {
		t.Fatal(err)
	}
	rank, _ := r.Effect(MechanicRankComparison)
	if rank.Value != ValueHigh || rank.InstanceID != s.Field[base-1].InstanceID || r.History[base].Active {
		t.Fatalf("removal revived older evolution %+v", r)
	}
	s.Field[base].Removed = true
	r, err = ResolveRules(s.Field, c, s.Config)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.History) != base+2 || len(r.ActiveField) != 2 {
		t.Fatal("history was discarded")
	}
}

func TestRevealSkipsIneligibleAndStopsOnEmptyDeck(t *testing.T) {
	c := emptyEvolutionCatalog()
	for i := range c.Definitions {
		switch c.Definitions[i].ID {
		case RuleLowCardWins:
			c.Definitions[i].InDeck = true
			c.Definitions[i].Prerequisites = []RuleID{RuleNoFollowRequired}
		case RuleNoFollowRequired:
			c.Definitions[i].InDeck = true
		}
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	s, err := NewMatch([]SeatID{"a", "b"}, DefaultConfig(), c, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.RuleDraw, []RuleID{RuleNoFollowRequired, RuleLowCardWins}) {
		t.Fatalf("deck %+v", s.RuleDraw)
	}
	s.RuleDraw = []RuleID{RuleLowCardWins, RuleNoFollowRequired}
	for i := 0; i < 3; i++ {
		var revealed *RuleInstance
		s, revealed, err = RecordCompletedHand(s, c)
		if err != nil {
			t.Fatal(err)
		}
		if i == 2 && (revealed == nil || revealed.DefinitionID != RuleNoFollowRequired) {
			t.Fatalf("first reveal %+v", revealed)
		}
	}
	if !reflect.DeepEqual(s.RuleDraw, []RuleID{RuleLowCardWins}) {
		t.Fatalf("deferred rule lost %+v", s.RuleDraw)
	}
	for i := 0; i < 3; i++ {
		var revealed *RuleInstance
		s, revealed, err = RecordCompletedHand(s, c)
		if err != nil {
			t.Fatal(err)
		}
		if i == 2 && (revealed == nil || revealed.DefinitionID != RuleLowCardWins) {
			t.Fatalf("second reveal %+v", revealed)
		}
	}
	for i := 0; i < 3; i++ {
		var revealed *RuleInstance
		s, revealed, err = RecordCompletedHand(s, c)
		if err != nil || revealed != nil {
			t.Fatalf("empty deck reveal %+v %v", revealed, err)
		}
	}
	if s.CompletedHands != 9 {
		t.Fatal("hand count changed")
	}
}

func TestDependentRuleBecomesInactive(t *testing.T) {
	c := DefaultCatalog()
	for i := range c.Definitions {
		if c.Definitions[i].ID == RuleLowCardWins {
			c.Definitions[i].Prerequisites = []RuleID{RuleNoFollowRequired}
		}
	}
	s, err := NewMatch([]SeatID{"a", "b"}, DefaultConfig(), c, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	s.Field = appendRule(s.Field, RuleNoFollowRequired)
	s.Field = appendRule(s.Field, RuleLowCardWins)
	r, err := ResolveRules(s.Field, c, s.Config)
	if err != nil {
		t.Fatal(err)
	}
	rank, _ := r.Effect(MechanicRankComparison)
	if rank.Value != ValueLow {
		t.Fatal("dependent did not activate")
	}
	s.Field = appendRule(s.Field, RuleFollowSuit)
	r, err = ResolveRules(s.Field, c, s.Config)
	if err != nil {
		t.Fatal(err)
	}
	rank, _ = r.Effect(MechanicRankComparison)
	if rank.Value != ValueHigh || r.History[len(s.Field)-2].InactiveReason != "prerequisite_inactive" {
		t.Fatalf("dependency loss %+v", r)
	}
}

func TestConfigAndSeatValidation(t *testing.T) {
	c := DefaultCatalog()
	for _, cfg := range []Config{{HandSize: 5, RevealEvery: 2}, {HandSize: 5, RevealEvery: 6}, {HandSize: 14, RevealEvery: 3}} {
		if _, err := NewMatch([]SeatID{"a", "b"}, cfg, c, SeededShuffle(1)); err == nil {
			t.Fatalf("accepted %+v", cfg)
		}
	}
	if _, err := NewMatch([]SeatID{"a", "a"}, DefaultConfig(), c, SeededShuffle(1)); err == nil {
		t.Fatal("duplicate seats accepted")
	}
}

func TestHiddenPrerequisiteAndIneligibleDeckRetention(t *testing.T) {
	c := emptyEvolutionCatalog()
	for i := range c.Definitions {
		if c.Definitions[i].ID == RuleLowCardWins {
			c.Definitions[i].InDeck = true
			c.Definitions[i].Prerequisites = []RuleID{RuleNoTrump, RuleNoFollowRequired}
		}
	}
	s, err := NewMatch([]SeatID{"a", "b"}, DefaultConfig(), c, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		var revealed *RuleInstance
		s, revealed, err = RecordCompletedHand(s, c)
		if err != nil || revealed != nil {
			t.Fatalf("ineligible card revealed: %+v %v", revealed, err)
		}
	}
	if len(s.RuleDraw) != 1 || s.RuleDraw[0] != RuleLowCardWins {
		t.Fatal("ineligible card was lost")
	}
	s.Field = appendRule(s.Field, RuleNoFollowRequired)
	for i := 0; i < 3; i++ {
		var revealed *RuleInstance
		s, revealed, err = RecordCompletedHand(s, c)
		if err != nil {
			t.Fatal(err)
		}
		if i == 2 && (revealed == nil || revealed.DefinitionID != RuleLowCardWins) {
			t.Fatal("hidden prerequisite did not satisfy reveal")
		}
	}
}

func TestDependentReactivatesWhenStartingPrerequisiteReturns(t *testing.T) {
	c := DefaultCatalog()
	for i := range c.Definitions {
		if c.Definitions[i].ID == RuleLowCardWins {
			c.Definitions[i].Prerequisites = []RuleID{RuleFollowSuit}
		}
	}
	s, err := NewMatch([]SeatID{"a", "b"}, DefaultConfig(), c, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	s.Field = appendRule(s.Field, RuleLowCardWins)
	s.Field = appendRule(s.Field, RuleNoFollowRequired)
	r, err := ResolveRules(s.Field, c, s.Config)
	if err != nil {
		t.Fatal(err)
	}
	rank, _ := r.Effect(MechanicRankComparison)
	if rank.Value != ValueHigh {
		t.Fatal("dependent stayed active after prerequisite replacement")
	}
	s.Field[len(s.Field)-1].Removed = true
	r, err = ResolveRules(s.Field, c, s.Config)
	if err != nil {
		t.Fatal(err)
	}
	rank, _ = r.Effect(MechanicRankComparison)
	if rank.Value != ValueLow {
		t.Fatal("dependent failed to reactivate with starting prerequisite")
	}
}

func TestFinalCadenceDoesNotReveal(t *testing.T) {
	c := emptyEvolutionCatalog()
	for i := range c.Definitions {
		if c.Definitions[i].ID == RuleLowCardWins {
			c.Definitions[i].InDeck = true
		}
	}
	cfg := Config{HandSize: 5, RevealEvery: 3, MaxHands: 6}
	s, err := NewMatch([]SeatID{"a", "b"}, cfg, c, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	s.CompletedHands = 5
	next, revealed, err := RecordCompletedHand(s, c)
	if err != nil {
		t.Fatal(err)
	}
	if revealed != nil || next.CompletedHands != 6 || len(next.RuleDraw) != 1 || len(next.Field) != len(s.Field) {
		t.Fatal("final hand introduced an unused rule")
	}
}

func TestMechanicSpecificReplacementKeepsIndependentEffect(t *testing.T) {
	c := DefaultCatalog()
	const eligibility RuleID = "test.eligibility"
	c.Definitions = append(c.Definitions, RuleDefinition{ID: eligibility, Title: "Eligibility Fixture", Text: "Lead suit remains eligible.", Category: "rule", Status: StatusSupported, FieldCategory: FieldTrick, Effects: []TypedEffect{{Mechanic: MechanicWinnerEligibility, Value: ValueLeadSuit}}, Overrides: []RuleID{RuleFollowSuit}, StartingVisibility: NotStarting})
	s, err := NewMatch([]SeatID{"a", "b"}, DefaultConfig(), c, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	s.Field = appendRule(s.Field, eligibility)
	r, err := ResolveRules(s.Field, c, s.Config)
	if err != nil {
		t.Fatal(err)
	}
	play, _ := r.Effect(MechanicPlayRestriction)
	eligible, _ := r.Effect(MechanicWinnerEligibility)
	if play.Source != RuleFollowSuit || eligible.Source != eligibility || !r.History[len(s.Field)-3].Active {
		t.Fatalf("independent effect was lost: %+v", r)
	}
	if !reflect.DeepEqual(r.History[len(s.Field)-3].ActiveEffects, []Mechanic{MechanicPlayRestriction}) {
		t.Fatalf("follow suit effects %+v", r.History[len(s.Field)-3])
	}
	s.Field[len(s.Field)-1].Removed = true
	r, err = ResolveRules(s.Field, c, s.Config)
	if err != nil {
		t.Fatal(err)
	}
	eligible, _ = r.Effect(MechanicWinnerEligibility)
	if eligible.Source != RuleFollowSuit || len(r.History[len(s.Field)-3].ActiveEffects) != 2 {
		t.Fatal("partial replacement did not restore its starting effect")
	}
}
