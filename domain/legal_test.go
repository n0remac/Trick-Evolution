package domain

import (
	"reflect"
	"testing"
)

func TestLegalCardsUsesResolvedFollowOverride(t *testing.T) {
	catalog := DefaultCatalog()
	field := []RuleInstance{
		{InstanceID: "high", DefinitionID: RuleHighCardWins, Sequence: 0},
		{InstanceID: "follow", DefinitionID: RuleFollowSuit, Sequence: 1},
	}
	rules, err := ResolveField(field, catalog)
	if err != nil {
		t.Fatal(err)
	}
	view := PlayerView{Seat: "bot:1", Hand: []CardID{"std:H:03", "std:S:02"}, Public: PublicView{
		Phase: PhasePlayingTrick, Turn: "bot:1", CurrentTrick: []Play{{Seat: "human", Card: "std:H:09"}}, Rules: rules,
	}}
	got, err := LegalCards(view, catalog)
	if err != nil || !reflect.DeepEqual(got, []CardID{"std:H:03"}) {
		t.Fatalf("follow suit: %v, %v", got, err)
	}
	field = append(field, RuleInstance{InstanceID: "any", DefinitionID: RuleNoFollowRequired, Sequence: 2})
	view.Public.Rules, err = ResolveField(field, catalog)
	if err != nil {
		t.Fatal(err)
	}
	got, err = LegalCards(view, catalog)
	if err != nil || !reflect.DeepEqual(got, view.Hand) {
		t.Fatalf("no-follow override: %v, %v", got, err)
	}
	view.Public.Turn = "human"
	if _, err := LegalCards(view, catalog); err == nil {
		t.Fatal("out-of-turn bot received legal cards")
	}
}
