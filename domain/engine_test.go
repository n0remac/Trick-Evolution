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

func TestHostEndsStartedGameAtCurrentScore(t *testing.T) {
	state := mustMatch(t)
	_, _, err := Step(state, Action{Kind: ActionEndGame, Seat: "b"}, DefaultCatalog())
	requireCode(t, err, ErrNotHost)
	state, _ = mustStep(t, state, Action{Kind: ActionPlayCard, Seat: "b", Card: state.Hands["b"][0]})
	previous := state
	next, events := mustStep(t, state, Action{Kind: ActionEndGame, Seat: "a"})
	if next.Phase != PhaseGameComplete || !next.EndedEarly || next.EndedBy != "a" || next.Turn != "" ||
		next.Revision != previous.Revision+1 || next.CompletedHands != 0 || len(next.CurrentTrick) != 1 {
		t.Fatalf("early end changed match incorrectly: %+v", next)
	}
	if len(events) != 2 || events[0].Kind != EventGameEnded || events[0].Seat != "a" ||
		events[1].Kind != EventGameCompleted || !reflect.DeepEqual(events[1].Winners, []SeatID{"a", "b"}) {
		t.Fatalf("wrong end events: %+v", events)
	}
	assertCardConservation(t, next)
	public, err := ProjectPublic(next, DefaultCatalog())
	if err != nil || !public.EndedEarly || public.EndedBy != "a" || !reflect.DeepEqual(public.Winners, []SeatID{"a", "b"}) {
		t.Fatalf("end projection: %+v %v", public, err)
	}
	_, _, err = Step(next, Action{Kind: ActionPlayCard, Seat: "a", Card: next.Hands["a"][0]}, DefaultCatalog())
	requireCode(t, err, ErrWrongPhase)
	_, _, err = Step(next, Action{Kind: ActionEndGame, Seat: "a"}, DefaultCatalog())
	requireCode(t, err, ErrWrongPhase)

	state = mustMatch(t)
	for state.Phase == PhasePlayingTrick {
		state, _ = mustStep(t, state, Action{Kind: ActionPlayCard, Seat: state.Turn, Card: state.Hands[state.Turn][0]})
	}
	next, _ = mustStep(t, state, Action{Kind: ActionEndGame, Seat: "a"})
	if next.CompletedHands != 1 || next.Seats[0].Score+next.Seats[1].Score != 5 || next.Phase != PhaseGameComplete {
		t.Fatalf("end between hands changed scoring: %+v", next)
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
	if len(state.Field) != 15 {
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
	state, err := ReplayMatch([]SeatID{"a", "b"}, DefaultConfig(), emptyEvolutionCatalog(), []ShuffleRecord{{Kind: StandardDeckKind, Order: stringsOrder}, {Kind: RuleDeckKind}})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range rules {
		state.Field = appendRule(state.Field, id)
	}
	return state
}

func TestFollowSuitAndExplicitOverride(t *testing.T) {
	state := mixedState(t)
	state, _ = mustStep(t, state, Action{Kind: ActionPlayCard, Seat: "b", Card: "std:H:02"})
	before := cloneState(state)
	action := Action{Kind: ActionPlayCard, Seat: "a", Card: "std:S:02"}
	returned, events, err := Step(state, action, DefaultCatalog())
	requireCode(t, err, ErrMustFollowSuit)
	if len(events) != 0 || !reflect.DeepEqual(state, before) || !reflect.DeepEqual(returned, before) {
		t.Fatal("rejected play mutated state")
	}
	legal, _ := mustStep(t, state, Action{Kind: ActionPlayCard, Seat: "a", Card: "std:H:03"})
	if legal.Seats[0].Score != 1 {
		t.Fatal("high card did not win")
	}
	state.Field = appendRule(state.Field, RuleNoFollowRequired)
	next, events := mustStep(t, state, action)
	if next.Seats[1].Score != 1 || events[1].Seat != "b" {
		t.Fatal("off-suit high won or override failed")
	}
	resolved, _ := ResolveRules(next.Field, DefaultCatalog(), next.Config)
	follow := len(mixedState(t).Field) - 2
	if resolved.History[follow].Active || !resolved.History[len(next.Field)-1].Active {
		t.Fatal("follow replacement projection wrong")
	}
}

func TestSequentialReplacementBothDirections(t *testing.T) {
	state := mixedState(t, RuleNoFollowRequired, RuleFollowSuit)
	r, err := ResolveRules(state.Field, DefaultCatalog(), state.Config)
	if err != nil {
		t.Fatal(err)
	}
	play, _ := r.Effect(MechanicPlayRestriction)
	if play.Value != ValueFollow || r.History[len(state.Field)-2].Active {
		t.Fatal("follow did not replace no-follow")
	}
	state.Field = appendRule(state.Field, RuleLowCardWins)
	state.Field = appendRule(state.Field, RuleHighCardWins)
	r, err = ResolveRules(state.Field, DefaultCatalog(), state.Config)
	if err != nil {
		t.Fatal(err)
	}
	rank, _ := r.Effect(MechanicRankComparison)
	if rank.Value != ValueHigh || r.History[len(state.Field)-2].Active {
		t.Fatal("high did not replace low")
	}
}

func TestLowCardWinsAndHighIsInactive(t *testing.T) {
	base := mixedState(t)
	base, _ = mustStep(t, base, Action{Kind: ActionPlayCard, Seat: "b", Card: "std:H:02"})
	high, _ := mustStep(t, base, Action{Kind: ActionPlayCard, Seat: "a", Card: "std:H:03"})
	if high.Seats[0].Score != 1 {
		t.Fatal("high baseline failed")
	}
	base.Field = appendRule(base.Field, RuleLowCardWins)
	low, events := mustStep(t, base, Action{Kind: ActionPlayCard, Seat: "a", Card: "std:H:03"})
	if low.Seats[1].Score != 1 || events[1].Seat != "b" {
		t.Fatal("low comparison failed")
	}
	r, _ := ResolveRules(low.Field, DefaultCatalog(), low.Config)
	if r.History[len(r.History)-2].Active || !r.History[len(r.History)-1].Active {
		t.Fatal("rank replacement history wrong")
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
	catalog := emptyEvolutionCatalog()
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
	if len(revealHands) != 0 || state.CompletedHands != 10 || len(state.Field) != 15 || state.Revision != uint64(len(actions)) {
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

func TestEmptyPrivateHandProjectsAsArray(t *testing.T) {
	state := mustMatch(t)
	state.Hands["a"] = nil
	view, err := ProjectForSeat(state, DefaultCatalog(), "a")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"Hand":[]`) {
		t.Fatalf("empty hand was not a JSON array: %s", encoded)
	}
}

func TestBoundedFullGameRevealAppliesBetweenHands(t *testing.T) {
	catalog := emptyEvolutionCatalog()
	for i := range catalog.Definitions {
		if catalog.Definitions[i].ID == RuleLowCardWins {
			catalog.Definitions[i].InDeck = true
		}
	}
	cfg := Config{HandSize: 1, RevealEvery: 3, MaxHands: 4}
	state, err := NewMatch([]SeatID{"a", "b"}, cfg, catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	var reveals []int
	var lastWinner SeatID
	for state.Phase != PhaseGameComplete {
		if state.Phase == PhaseHandComplete {
			state, _, err = Step(state, Action{Kind: ActionStartHand, Seat: "a", DeckOrder: canonicalOrder()}, catalog)
			if err != nil {
				t.Fatal(err)
			}
			continue
		}
		view, err := ProjectForSeat(state, catalog, state.Turn)
		if err != nil {
			t.Fatal(err)
		}
		legal, err := LegalCards(view, catalog)
		if err != nil || len(legal) != 1 {
			t.Fatalf("legal cards %v %v", legal, err)
		}
		var events []Event
		state, events, err = Step(state, Action{Kind: ActionPlayCard, Seat: state.Turn, Card: legal[0]}, catalog)
		if err != nil {
			t.Fatal(err)
		}
		for _, event := range events {
			if event.Kind == EventRuleRevealed {
				reveals = append(reveals, event.Hand)
			}
			if event.Kind == EventTrickWon {
				lastWinner = event.Seat
			}
		}
		if state.CompletedHands == 3 && state.Phase == PhaseHandComplete {
			resolved, err := ResolveRules(state.Field, catalog, cfg)
			if err != nil {
				t.Fatal(err)
			}
			rank, _ := resolved.Effect(MechanicRankComparison)
			if rank.Value != ValueLow {
				t.Fatal("reveal did not activate for next hand")
			}
		}
	}
	if !reflect.DeepEqual(reveals, []int{3}) || state.CompletedHands != 4 || len(state.Field) != 16 || lastWinner != "a" {
		t.Fatalf("bounded match: reveals=%v winner=%s field=%d", reveals, lastWinner, len(state.Field))
	}
	if state.Seats[0].Score+state.Seats[1].Score != 4 {
		t.Fatal("trick scoring changed")
	}
}
