package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func mustMatch(t *testing.T) MatchState {
	t.Helper()
	state, err := NewMatch([]SeatID{"a", "b"}, DefaultConfig(), DefaultCatalog(), identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func mustStep(t *testing.T, state MatchState, action Action) (MatchState, []Event) {
	t.Helper()
	next, events, err := Step(state, action, DefaultCatalog())
	if err != nil {
		t.Fatalf("%+v: %v", action, err)
	}
	return next, events
}

func requireCode(t *testing.T, err error, code ErrorCode) {
	t.Helper()
	var transition *TransitionError
	if !errors.As(err, &transition) || transition.Code != code {
		t.Fatalf("wanted %s, got %v", code, err)
	}
}

func assertCardConservation(t *testing.T, state MatchState) {
	t.Helper()
	seen := map[CardID]bool{}
	add := func(card CardID) {
		if seen[card] {
			t.Fatalf("card %s counted twice", card)
		}
		seen[card] = true
	}
	for _, card := range state.StandardDraw {
		add(card)
	}
	for _, seat := range state.Seats {
		for _, card := range state.Hands[seat.ID] {
			add(card)
		}
		for _, card := range state.Captured[seat.ID] {
			add(card)
		}
	}
	for _, play := range state.CurrentTrick {
		add(play.Card)
	}
	if len(seen) != 52 {
		t.Fatalf("card conservation: got %d", len(seen))
	}
}

func TestFixedCompleteHand(t *testing.T) {
	state := mustMatch(t)
	assertCardConservation(t, state)
	trickWins := 0
	for state.Phase == PhasePlayingTrick {
		card := state.Hands[state.Turn][0]
		previousRevision := state.Revision
		next, events := mustStep(t, state, Action{Kind: ActionPlayCard, Seat: state.Turn, Card: card})
		if next.Revision != previousRevision+1 {
			t.Fatal("revision did not advance exactly once")
		}
		for _, event := range events {
			if event.Kind == EventTrickWon {
				trickWins++
				if event.Seat != "a" || len(event.Plays) != 2 {
					t.Fatalf("incorrect trick award: %+v", event)
				}
			}
		}
		state = next
		assertCardConservation(t, state)
	}
	if trickWins != 5 || state.CompletedHands != 1 || state.Phase != PhaseHandComplete || state.Seats[0].Score != 5 || state.Seats[0].TricksWon != 5 || state.Seats[1].Score != 0 || state.Turn != "" {
		t.Fatalf("incorrect completed hand: %+v", state)
	}
	if len(state.Field) != 1 {
		t.Fatal("rule revealed before cadence boundary")
	}
	public, err := ProjectPublic(state, DefaultCatalog())
	if err != nil || public.Seats[0].HandCount != 0 || public.Seats[0].CapturedCount != 10 {
		t.Fatalf("bad public view: %+v, %v", public, err)
	}
}

func mixedDeck() []CardID {
	first := []CardID{"std:H:02", "std:H:03", "std:C:02", "std:S:02", "std:H:04", "std:D:02", "std:S:04", "std:H:05", "std:D:04", "std:C:03"}
	used := map[CardID]bool{}
	for _, card := range first {
		used[card] = true
	}
	for _, card := range StandardDeck() {
		if !used[card.ID] {
			first = append(first, card.ID)
		}
	}
	return first
}

func mixedState(t *testing.T, rules ...RuleID) MatchState {
	t.Helper()
	order := mixedDeck()
	stringsOrder := make([]string, len(order))
	for i, id := range order {
		stringsOrder[i] = string(id)
	}
	ruleOrder := []string{string(RuleFollowSuit), string(RuleNoFollowRequired), string(RuleLowCardWins)}
	state, err := ReplayMatch([]SeatID{"a", "b"}, DefaultConfig(), DefaultCatalog(), []ShuffleRecord{{Kind: StandardDeckKind, Order: stringsOrder}, {Kind: RuleDeckKind, Order: ruleOrder}})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range rules {
		state.Field = append(state.Field, RuleInstance{InstanceID: string(id), DefinitionID: id, Sequence: len(state.Field)})
	}
	return state
}

func TestFollowSuitAndExplicitOverride(t *testing.T) {
	state := mixedState(t, RuleFollowSuit)
	state, _ = mustStep(t, state, Action{Kind: ActionPlayCard, Seat: "b", Card: "std:H:02"})
	before := cloneState(state)
	action := Action{Kind: ActionPlayCard, Seat: "a", Card: "std:S:02"}
	returned, events, err := Step(state, action, DefaultCatalog())
	requireCode(t, err, ErrMustFollowSuit)
	if len(events) != 0 || !reflect.DeepEqual(state, before) || !reflect.DeepEqual(returned, before) {
		t.Fatal("rejected play mutated state or emitted events")
	}
	legal, _ := mustStep(t, state, Action{Kind: ActionPlayCard, Seat: "a", Card: "std:H:03"})
	if legal.Seats[0].Score != 1 {
		t.Fatal("legal follow-suit play awarded wrong seat")
	}
	state.Field = append(state.Field, RuleInstance{InstanceID: "no-follow", DefinitionID: RuleNoFollowRequired, Sequence: len(state.Field)})
	next, events := mustStep(t, state, action)
	if next.Seats[1].Score != 1 || events[1].Seat != "b" {
		t.Fatal("off-suit card won or override failed")
	}
	resolved, _ := ResolveField(next.Field, DefaultCatalog())
	if resolved[1].Active || !resolved[2].Active || !resolved[0].Active {
		t.Fatalf("incorrect active rules: %+v", resolved)
	}
}

func TestOverrideDrawnBeforeTarget(t *testing.T) {
	state := mixedState(t, RuleNoFollowRequired, RuleFollowSuit)
	resolved, err := ResolveField(state.Field, DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if !resolved[1].Active || resolved[2].Active || !reflect.DeepEqual(resolved[2].OverriddenBy, []string{string(RuleNoFollowRequired)}) {
		t.Fatalf("earlier explicit override did not disable its target: %+v", resolved)
	}
	state, _ = mustStep(t, state, Action{Kind: ActionPlayCard, Seat: "b", Card: "std:H:02"})
	if _, _, err := Step(state, Action{Kind: ActionPlayCard, Seat: "a", Card: "std:S:02"}, DefaultCatalog()); err != nil {
		t.Fatalf("overridden follow-suit rule still blocked play: %v", err)
	}
}

func TestLowCardWinsAndHighIsInactive(t *testing.T) {
	base := mixedState(t)
	base, _ = mustStep(t, base, Action{Kind: ActionPlayCard, Seat: "b", Card: "std:H:02"})
	high, _ := mustStep(t, base, Action{Kind: ActionPlayCard, Seat: "a", Card: "std:H:03"})
	if high.Seats[0].Score != 1 {
		t.Fatal("baseline high card failed")
	}
	base.Field = append(base.Field, RuleInstance{InstanceID: "low", DefinitionID: RuleLowCardWins, Sequence: 1})
	low, events := mustStep(t, base, Action{Kind: ActionPlayCard, Seat: "a", Card: "std:H:03"})
	if low.Seats[1].Score != 1 || events[1].Seat != "b" {
		t.Fatal("low card did not override high")
	}
	resolved, _ := ResolveField(low.Field, DefaultCatalog())
	if resolved[0].Active || !reflect.DeepEqual(resolved[0].OverriddenBy, []string{"low"}) || !resolved[1].Active {
		t.Fatalf("bad override bookkeeping: %+v", resolved)
	}
}

func TestInvalidActionsDoNotMutate(t *testing.T) {
	state := mustMatch(t)
	badDeck := append([]CardID(nil), state.Hands["a"]...)
	cases := []struct {
		action Action
		code   ErrorCode
	}{
		{Action{Kind: ActionPlayCard, Seat: "ghost", Card: state.Hands["b"][0]}, ErrUnknownSeat},
		{Action{Kind: ActionPlayCard, Seat: "a", Card: state.Hands["a"][0]}, ErrNotYourTurn},
		{Action{Kind: ActionPlayCard, Seat: "b", Card: state.Hands["a"][0]}, ErrCardNotOwned},
		{Action{Kind: ActionStartHand, Seat: "a", DeckOrder: badDeck}, ErrWrongPhase},
		{Action{Kind: "unknown", Seat: "a"}, ErrUnknownAction},
	}
	for _, tc := range cases {
		before := cloneState(state)
		returned, events, err := Step(state, tc.action, DefaultCatalog())
		requireCode(t, err, tc.code)
		if len(events) != 0 || !reflect.DeepEqual(state, before) || !reflect.DeepEqual(returned, before) {
			t.Fatalf("mutation after %s", tc.code)
		}
	}
}

func legalCard(state MatchState, catalog Catalog) CardID {
	hand := state.Hands[state.Turn]
	play, _, _ := activeEffects(state.Field, catalog)
	if play == EffectFollow && len(state.CurrentTrick) != 0 {
		lead, _ := cardByID(state.CurrentTrick[0].Card)
		for _, id := range hand {
			card, _ := cardByID(id)
			if card.Suit == lead.Suit {
				return id
			}
		}
	}
	return hand[0]
}

func canonicalOrder() []CardID {
	deck := StandardDeck()
	order := make([]CardID, len(deck))
	for i, card := range deck {
		order[i] = card.ID
	}
	return order
}

func TestCompleteGameReplayAndCadence(t *testing.T) {
	catalog := DefaultCatalog()
	initial, err := NewMatch([]SeatID{"a", "b"}, DefaultConfig(), catalog, SeededShuffle(29))
	if err != nil {
		t.Fatal(err)
	}
	state := initial
	var actions []Action
	var allEvents [][]Event
	var revealHands []int
	for state.Phase != PhaseGameComplete {
		var action Action
		if state.Phase == PhaseHandComplete {
			action = Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()}
		} else {
			action = Action{Kind: ActionPlayCard, Seat: state.Turn, Card: legalCard(state, catalog)}
		}
		var events []Event
		state, events, err = Step(state, action, catalog)
		if err != nil {
			t.Fatal(err)
		}
		actions = append(actions, action)
		allEvents = append(allEvents, events)
		for _, event := range events {
			if event.Kind == EventRuleRevealed {
				revealHands = append(revealHands, event.Hand)
			}
		}
		assertCardConservation(t, state)
	}
	if !reflect.DeepEqual(revealHands, []int{3, 6, 9}) || state.CompletedHands != 10 || len(state.Field) != 4 || state.Revision != uint64(len(actions)) {
		t.Fatalf("incorrect game boundary or cadence: reveals=%v, state=%+v", revealHands, state)
	}
	if state.Seats[0].Score+state.Seats[1].Score != 50 {
		t.Fatalf("expected 50 awarded tricks, got %+v", state.Seats)
	}
	finalView, err := ProjectPublic(state, catalog)
	if err != nil || len(finalView.Winners) == 0 {
		t.Fatalf("missing game winner: %+v, %v", finalView, err)
	}
	if len(state.Shuffles) != 11 {
		t.Fatalf("expected initial standard+rule and 9 further deck records, got %d", len(state.Shuffles))
	}
	replayed := initial
	for i, action := range actions {
		var events []Event
		replayed, events, err = Step(replayed, action, catalog)
		if err != nil || !reflect.DeepEqual(events, allEvents[i]) {
			t.Fatalf("replay diverged at %d: %v", i, err)
		}
	}
	if !reflect.DeepEqual(replayed, state) {
		t.Fatal("final replay state diverged")
	}
	returned, events, err := Step(state, Action{Kind: ActionPlayCard, Seat: "a", Card: "std:H:02"}, catalog)
	requireCode(t, err, ErrWrongPhase)
	if len(events) != 0 || !reflect.DeepEqual(returned, state) {
		t.Fatal("game accepted a post-end action")
	}
}

func TestStartHandValidationAndViewPrivacy(t *testing.T) {
	state := mustMatch(t)
	for state.Phase == PhasePlayingTrick {
		state, _ = mustStep(t, state, Action{Kind: ActionPlayCard, Seat: state.Turn, Card: state.Hands[state.Turn][0]})
	}
	before, _ := json.Marshal(state)
	_, _, err := Step(state, Action{Kind: ActionStartHand, Seat: "b", DeckOrder: canonicalOrder()}, DefaultCatalog())
	requireCode(t, err, ErrNotHost)
	bad := canonicalOrder()
	bad[0] = bad[1]
	_, _, err = Step(state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: bad}, DefaultCatalog())
	requireCode(t, err, ErrInvalidDeck)
	after, _ := json.Marshal(state)
	if !reflect.DeepEqual(after, before) {
		t.Fatal("invalid start mutated state")
	}
	state, events := mustStep(t, state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()})
	if state.HandNumber != 2 || state.Dealer != "b" || state.Leader != "a" || state.Turn != "a" || events[0].Kind != EventHandStarted {
		t.Fatal("wrong next-hand setup")
	}
	public, err := ProjectPublic(state, DefaultCatalog())
	if err != nil {
		t.Fatal(err)
	}
	aView, err := ProjectForSeat(state, DefaultCatalog(), "a")
	if err != nil {
		t.Fatal(err)
	}
	bView, err := ProjectForSeat(state, DefaultCatalog(), "b")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(aView.Hand, state.Hands["a"]) || !reflect.DeepEqual(bView.Hand, state.Hands["b"]) || reflect.DeepEqual(aView.Hand, bView.Hand) {
		t.Fatal("private hand projection wrong")
	}
	publicJSON, _ := json.Marshal(public)
	eventsJSON, _ := json.Marshal(events)
	for _, card := range append(append([]CardID(nil), aView.Hand...), bView.Hand...) {
		if strings.Contains(string(publicJSON), string(card)) || strings.Contains(string(eventsJSON), string(card)) {
			t.Fatalf("private card %s leaked into public output", card)
		}
	}
	if _, err := ProjectForSeat(state, DefaultCatalog(), "ghost"); err == nil {
		t.Fatal("unknown seat received view")
	}
}
