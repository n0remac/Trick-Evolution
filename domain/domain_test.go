package domain

import (
	"reflect"
	"testing"
)

type identityShuffle struct{}

func (identityShuffle) Shuffle(_ int, _ func(i, j int)) {}

func TestStandardDeckUniqueAndStable(t *testing.T) {
	deck := StandardDeck()
	if len(deck) != 52 || !reflect.DeepEqual(deck, StandardDeck()) {
		t.Fatal("standard deck is not stable and complete")
	}
	seen := map[CardID]bool{}
	for _, card := range deck {
		if seen[card.ID] || card.Rank < 2 || card.Rank > 14 {
			t.Fatalf("invalid card: %+v", card)
		}
		seen[card.ID] = true
	}
}

func TestSeededSetupAndReplay(t *testing.T) {
	ids := []SeatID{"north", "east", "south", "west"}
	catalog := DefaultCatalog()
	a, err := NewMatch(ids, DefaultConfig(), catalog, SeededShuffle(73))
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewMatch(ids, DefaultConfig(), catalog, SeededShuffle(73))
	if err != nil || !reflect.DeepEqual(a, b) {
		t.Fatalf("same seed changed setup: %v", err)
	}
	replay, err := ReplayMatch(ids, DefaultConfig(), catalog, a.Shuffles)
	if err != nil || !reflect.DeepEqual(a, replay) {
		t.Fatalf("recorded replay changed setup: %v", err)
	}
	if a.HandNumber != 1 || a.TrickNumber != 1 || a.CompletedHands != 0 || a.Phase != PhasePlayingTrick || a.Dealer != "north" || a.Leader != "east" || a.Turn != "east" {
		t.Fatalf("incorrect initial state: %+v", a)
	}
	if len(a.RuleDraw) != 3 || len(a.Field) != 1 || a.Field[0].DefinitionID != RuleHighCardWins {
		t.Fatalf("incorrect rule setup: %+v", a)
	}
	seen := map[CardID]bool{}
	for _, id := range ids {
		if len(a.Hands[id]) != 5 {
			t.Fatalf("%s has %d cards", id, len(a.Hands[id]))
		}
		for _, card := range a.Hands[id] {
			if seen[card] {
				t.Fatalf("duplicate dealt card %s", card)
			}
			seen[card] = true
		}
	}
	for _, card := range a.StandardDraw {
		if seen[card] {
			t.Fatalf("duplicate draw card %s", card)
		}
		seen[card] = true
	}
	if len(seen) != 52 {
		t.Fatalf("card conservation: %d", len(seen))
	}
	bad := append([]ShuffleRecord(nil), a.Shuffles...)
	bad[0].Order = append([]string(nil), bad[0].Order...)
	bad[0].Order[0] = bad[0].Order[1]
	if _, err := ReplayMatch(ids, DefaultConfig(), catalog, bad); err == nil {
		t.Fatal("duplicate recorded card accepted")
	}
}

func TestInjectedFixtureDeal(t *testing.T) {
	state, err := NewMatch([]SeatID{"a", "b"}, DefaultConfig(), DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(state.Hands["b"], []CardID{"std:H:02", "std:H:04", "std:H:06", "std:H:08", "std:H:10"}) ||
		!reflect.DeepEqual(state.Hands["a"], []CardID{"std:H:03", "std:H:05", "std:H:07", "std:H:09", "std:H:11"}) {
		t.Fatalf("clockwise one-at-a-time deal changed: %+v", state.Hands)
	}
	if !reflect.DeepEqual(state.RuleDraw, []RuleID{RuleFollowSuit, RuleNoFollowRequired, RuleLowCardWins}) {
		t.Fatalf("rule catalog order changed: %+v", state.RuleDraw)
	}
}

func TestCatalogValidation(t *testing.T) {
	catalog := DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Definitions) != 55 {
		t.Fatalf("expected all 55 printed entries, got %d", len(catalog.Definitions))
	}
	for _, d := range catalog.Definitions {
		if d.Status == StatusUnsupported && d.InDeck {
			t.Fatalf("unsupported rule in deck: %s", d.ID)
		}
	}
	cases := map[string]func(*Catalog){
		"duplicate ID":        func(c *Catalog) { c.Definitions = append(c.Definitions, c.Definitions[0]) },
		"unknown effect":      func(c *Catalog) { c.Definitions[0].PlayEffect = "unrecognized" },
		"bad parameter":       func(c *Catalog) { c.Definitions[0].Parameters.LeadSuitOnly = true },
		"unknown override":    func(c *Catalog) { c.Definitions[1].Overrides = []RuleID{"missing"} },
		"unsupported in deck": func(c *Catalog) { c.Definitions[2].InDeck = true },
		"unknown version":     func(c *Catalog) { c.Version++ },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			copyCatalog := DefaultCatalog()
			change(&copyCatalog)
			if err := copyCatalog.Validate(); err == nil {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
}

func TestRevealCadenceAndOverride(t *testing.T) {
	for _, interval := range []int{3, 4, 5} {
		t.Run(string(rune('0'+interval)), func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.RevealEvery = interval
			catalog := DefaultCatalog()
			state, err := NewMatch([]SeatID{"a", "b"}, cfg, catalog, SeededShuffle(1))
			if err != nil {
				t.Fatal(err)
			}
			// Fixture draw order makes the additive rule first and its explicit
			// override second, regardless of the setup seed.
			state.RuleDraw = []RuleID{RuleFollowSuit, RuleNoFollowRequired, RuleLowCardWins}
			for completed := 1; completed <= 2*interval; completed++ {
				before := state
				var revealed *RuleInstance
				state, revealed, err = RecordCompletedHand(state, catalog)
				if err != nil {
					t.Fatal(err)
				}
				if before.CompletedHands != completed-1 {
					t.Fatal("input mutated")
				}
				if completed%interval == 0 {
					if revealed == nil || revealed.IntroducedAfterHand != completed {
						t.Fatalf("missing reveal at %d", completed)
					}
				} else if revealed != nil {
					t.Fatalf("early reveal at %d", completed)
				}
			}
			resolved, err := ResolveField(state.Field, catalog)
			if err != nil {
				t.Fatal(err)
			}
			if len(resolved) != 3 || !resolved[0].Active || resolved[1].Active || !resolved[2].Active || !reflect.DeepEqual(resolved[1].OverriddenBy, []string{resolved[2].Instance.InstanceID}) {
				t.Fatalf("incorrect override projection: %+v", resolved)
			}
			if state.Field[1].DefinitionID != RuleFollowSuit {
				t.Fatal("overridden instance removed")
			}
		})
	}
}

func TestInvalidRevealLeavesStateUntouched(t *testing.T) {
	catalog := DefaultCatalog()
	state, err := NewMatch([]SeatID{"a", "b"}, DefaultConfig(), catalog, SeededShuffle(3))
	if err != nil {
		t.Fatal(err)
	}
	state.CompletedHands = 2
	state.RuleDraw = []RuleID{"missing"}
	before := state
	_, _, err = RecordCompletedHand(state, catalog)
	if err == nil || !reflect.DeepEqual(state, before) {
		t.Fatal("invalid reveal mutated input or succeeded")
	}
}

func TestLowCardExplicitlyOverridesBaseline(t *testing.T) {
	catalog := DefaultCatalog()
	field := []RuleInstance{
		{InstanceID: "initial", DefinitionID: RuleHighCardWins, Sequence: 0},
		{InstanceID: "additive", DefinitionID: RuleFollowSuit, Sequence: 1},
		{InstanceID: "low", DefinitionID: RuleLowCardWins, Sequence: 2},
	}
	resolved, err := ResolveField(field, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if resolved[0].Active || !reflect.DeepEqual(resolved[0].OverriddenBy, []string{"low"}) || !resolved[1].Active || !resolved[2].Active {
		t.Fatalf("unexpected resolved field: %+v", resolved)
	}
	if len(field[0].InstanceID) == 0 || field[0].DefinitionID != RuleHighCardWins {
		t.Fatal("field was mutated")
	}
}

func TestConfigAndSeatValidation(t *testing.T) {
	catalog := DefaultCatalog()
	for _, cfg := range []Config{{HandSize: 5, RevealEvery: 2}, {HandSize: 5, RevealEvery: 6}, {HandSize: 14, RevealEvery: 3}} {
		if _, err := NewMatch([]SeatID{"a", "b"}, cfg, catalog, SeededShuffle(1)); err == nil {
			t.Fatalf("accepted invalid config %+v", cfg)
		}
	}
	if _, err := NewMatch([]SeatID{"a", "a"}, DefaultConfig(), catalog, SeededShuffle(1)); err == nil {
		t.Fatal("duplicate seats accepted")
	}
	if _, err := NewMatch([]SeatID{"a"}, DefaultConfig(), catalog, SeededShuffle(1)); err == nil {
		t.Fatal("one seat accepted")
	}
}
