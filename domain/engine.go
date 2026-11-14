package domain

import (
	"fmt"
	"slices"
)

type ErrorCode string

const (
	ErrInvalidState      ErrorCode = "invalid_state"
	ErrUnknownAction     ErrorCode = "unknown_action"
	ErrUnknownSeat       ErrorCode = "unknown_seat"
	ErrWrongPhase        ErrorCode = "wrong_phase"
	ErrNotYourTurn       ErrorCode = "not_your_turn"
	ErrNotHost           ErrorCode = "not_host"
	ErrCardNotOwned      ErrorCode = "card_not_owned"
	ErrMustFollowSuit    ErrorCode = "must_follow_suit"
	ErrMustFollowOrTrump ErrorCode = "must_follow_or_trump"
	ErrIllegalLead       ErrorCode = "illegal_lead"
	ErrInvalidDeck       ErrorCode = "invalid_deck"
)

type TransitionError struct {
	Code   ErrorCode
	Detail string
}

func (e *TransitionError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Detail) }
func transitionError(code ErrorCode, detail string) error {
	return &TransitionError{Code: code, Detail: detail}
}

type EventKind string

const (
	EventHandStarted   EventKind = "hand_started"
	EventCardPlayed    EventKind = "card_played"
	EventTrickWon      EventKind = "trick_won"
	EventHandCompleted EventKind = "hand_completed"
	EventRuleRevealed  EventKind = "rule_revealed"
	EventGameEnded     EventKind = "game_ended"
	EventGameCompleted EventKind = "game_completed"
)

// Event has public facts only. A transport can broadcast it without exposing
// hands or a future deck order. Private hands come from ProjectForSeat.
type Event struct {
	Kind           EventKind
	Revision       uint64
	Hand           int
	Trick          int
	Seat           SeatID
	Card           CardID
	TrumpSuit      Suit
	BrokeSpades    bool
	BrokeHearts    bool
	Plays          []Play
	Score          int
	Rule           RuleID
	RuleInstanceID string
	Winners        []SeatID // Set only on game_completed; ties share the win.
}

// Step applies one action to an isolated copy. On any error, the returned
// state is the original value and the caller's maps/slices remain unchanged.
func Step(state MatchState, action Action, catalog Catalog) (MatchState, []Event, error) {
	if state.CatalogVersion != catalog.Version || state.Config.Validate(len(state.Seats)) != nil {
		return state, nil, transitionError(ErrInvalidState, "invalid match configuration or catalog version")
	}
	resolved, err := ResolveRules(state.Field, catalog, state.Config)
	if err != nil {
		return state, nil, transitionError(ErrInvalidState, err.Error())
	}
	if err := resolved.ValidateMatchEffects(state.Config); err != nil {
		return state, nil, transitionError(ErrInvalidState, err.Error())
	}
	if err := validateHandFacts(state, resolved); err != nil {
		return state, nil, transitionError(ErrInvalidState, err.Error())
	}
	seatIndex := indexOfSeat(state.Seats, action.Seat)
	if seatIndex < 0 {
		return state, nil, transitionError(ErrUnknownSeat, "seat is not in this match")
	}
	switch action.Kind {
	case ActionPlayCard:
		return stepPlay(state, action, seatIndex, catalog, resolved)
	case ActionStartHand:
		return stepStartHand(state, action, seatIndex, resolved)
	case ActionEndGame:
		return stepEndGame(state, action, seatIndex)
	default:
		return state, nil, transitionError(ErrUnknownAction, "action kind is not supported")
	}
}

// stepEndGame closes a started match at its current score. An unfinished
// trick earns no point; its public plays and private hands remain in the
// terminal snapshot for audit and reconnect.
func stepEndGame(state MatchState, action Action, seatIndex int) (MatchState, []Event, error) {
	if state.Phase != PhasePlayingTrick && state.Phase != PhaseHandComplete {
		return state, nil, transitionError(ErrWrongPhase, "game is not active")
	}
	if seatIndex != 0 {
		return state, nil, transitionError(ErrNotHost, "only the host may end the game")
	}
	next := cloneState(state)
	next.Revision++
	next.Phase = PhaseGameComplete
	next.Turn = ""
	next.EndedEarly = true
	next.EndedBy = action.Seat
	return next, []Event{
		{Kind: EventGameEnded, Revision: next.Revision, Hand: next.HandNumber, Seat: action.Seat},
		{Kind: EventGameCompleted, Revision: next.Revision, Hand: next.HandNumber, Winners: gameWinners(next.Seats)},
	}, nil
}

func stepStartHand(state MatchState, action Action, seatIndex int, resolved RuleResolution) (MatchState, []Event, error) {
	if state.Phase != PhaseHandComplete || state.CompletedHands >= state.Config.MaxHands {
		return state, nil, transitionError(ErrWrongPhase, "a new hand cannot start now")
	}
	if seatIndex != 0 {
		return state, nil, transitionError(ErrNotHost, "only seat zero may start a hand")
	}
	if len(state.CurrentTrick) != 0 {
		return state, nil, transitionError(ErrInvalidState, "unfinished trick")
	}
	for _, seat := range state.Seats {
		if len(state.Hands[seat.ID]) != 0 {
			return state, nil, transitionError(ErrInvalidState, "previous hand still has cards")
		}
	}
	order := make([]string, len(action.DeckOrder))
	for i, id := range action.DeckOrder {
		order[i] = string(id)
	}
	if err := validateOrder(order, standardIDs()); err != nil {
		return state, nil, transitionError(ErrInvalidDeck, err.Error())
	}
	next := cloneState(state)
	next.Revision++
	next.Phase = PhasePlayingTrick
	next.HandNumber++
	next.TrickNumber = 1
	next.CurrentTrick = nil
	next.BrokenSpades, next.BrokenHearts = false, false
	var err error
	next.TrumpSuit, err = TrumpForTrick(resolved, "", nil)
	if err != nil {
		return state, nil, transitionError(ErrInvalidState, err.Error())
	}
	next.Captured = make(map[SeatID][]CardID, len(next.Seats))
	dealerIndex := (indexOfSeat(next.Seats, state.Dealer) + 1) % len(next.Seats)
	next.Dealer = next.Seats[dealerIndex].ID
	leaderIndex := (dealerIndex + 1) % len(next.Seats)
	next.Leader, next.Turn = next.Seats[leaderIndex].ID, next.Seats[leaderIndex].ID
	for _, seat := range next.Seats {
		next.Hands[seat.ID] = make([]CardID, 0, next.Config.HandSize)
	}
	for card := 0; card < next.Config.HandSize; card++ {
		for offset := 1; offset <= len(next.Seats); offset++ {
			seat := next.Seats[(dealerIndex+offset)%len(next.Seats)].ID
			next.Hands[seat] = append(next.Hands[seat], action.DeckOrder[card*len(next.Seats)+offset-1])
		}
	}
	next.StandardDraw = append([]CardID(nil), action.DeckOrder[next.Config.HandSize*len(next.Seats):]...)
	next.Shuffles = append(next.Shuffles, ShuffleRecord{Kind: StandardDeckKind, Order: order})
	return next, []Event{{Kind: EventHandStarted, Revision: next.Revision, Hand: next.HandNumber, Seat: next.Leader}}, nil
}

func stepPlay(state MatchState, action Action, seatIndex int, catalog Catalog, resolvedRules RuleResolution) (MatchState, []Event, error) {
	if state.Phase != PhasePlayingTrick {
		return state, nil, transitionError(ErrWrongPhase, "cards can only be played during a trick")
	}
	if action.Seat != state.Turn {
		return state, nil, transitionError(ErrNotYourTurn, "another seat has the turn")
	}
	hand := state.Hands[action.Seat]
	cardIndex := -1
	for i, id := range hand {
		if id == action.Card {
			cardIndex = i
			break
		}
	}
	if cardIndex < 0 {
		return state, nil, transitionError(ErrCardNotOwned, "card is not in seat's hand")
	}
	_, ok := cardByID(action.Card)
	if !ok {
		return state, nil, transitionError(ErrInvalidState, "hand contains an unknown card")
	}
	legal, err := legalCardsWithRules(hand, state.CurrentTrick, resolvedRules, factsFromState(state))
	if err != nil {
		return state, nil, transitionError(ErrInvalidState, err.Error())
	}
	allowed := false
	for _, id := range legal {
		if id == action.Card {
			allowed = true
			break
		}
	}
	if !allowed {
		code := ErrMustFollowSuit
		if len(state.CurrentTrick) == 0 {
			code = ErrIllegalLead
		} else if play, _ := resolvedRules.Effect(MechanicPlayRestriction); play.Value == ValueFollowOrTrump {
			code = ErrMustFollowOrTrump
		}
		return state, nil, transitionError(code, IllegalPlayReason(hand, state.CurrentTrick, action.Card, resolvedRules, factsFromState(state)))
	}
	if len(state.CurrentTrick) >= len(state.Seats) {
		return state, nil, transitionError(ErrInvalidState, "trick already has all plays")
	}
	next := cloneState(state)
	next.Revision++
	next.Hands[action.Seat] = append(next.Hands[action.Seat][:cardIndex], next.Hands[action.Seat][cardIndex+1:]...)
	card, _ := cardByID(action.Card)
	played := Event{Kind: EventCardPlayed, Revision: next.Revision, Hand: next.HandNumber, Trick: next.TrickNumber, Seat: action.Seat, Card: action.Card}
	if len(state.CurrentTrick) == 0 && next.TrumpSuit == "" {
		trumpRule, _ := resolvedRules.Effect(MechanicTrump)
		if trumpRule.Value == ValueFirstLeadTrump && state.TrickNumber == 1 {
			next.TrumpSuit = card.Suit
			played.TrumpSuit = card.Suit
		}
	} else if len(state.CurrentTrick) > 0 {
		lead, _ := cardByID(state.CurrentTrick[0].Card)
		if card.Suit != lead.Suit {
			if card.Suit == Spades && !next.BrokenSpades {
				next.BrokenSpades, played.BrokeSpades = true, true
			}
			if card.Suit == Hearts && !next.BrokenHearts {
				next.BrokenHearts, played.BrokeHearts = true, true
			}
		}
	}
	next.CurrentTrick = append(next.CurrentTrick, Play{Seat: action.Seat, Card: action.Card})
	events := []Event{played}
	if len(next.CurrentTrick) < len(next.Seats) {
		next.Turn = next.Seats[(seatIndex+1)%len(next.Seats)].ID
		return next, events, nil
	}
	winningPlay, err := WinningPlay(next.CurrentTrick, resolvedRules, next.TrumpSuit)
	if err != nil {
		return state, nil, transitionError(ErrInvalidState, err.Error())
	}
	winner := winningPlay.Seat
	winnerIndex := indexOfSeat(next.Seats, winner)
	if winnerIndex < 0 {
		return state, nil, transitionError(ErrInvalidState, "trick winner is not a seat")
	}
	next.Seats[winnerIndex].TricksWon++
	next.Seats[winnerIndex].Score++ // Provisional Score by Tricks baseline.
	for _, play := range next.CurrentTrick {
		next.Captured[winner] = append(next.Captured[winner], play.Card)
	}
	events = append(events, Event{Kind: EventTrickWon, Revision: next.Revision, Hand: next.HandNumber, Trick: next.TrickNumber, Seat: winner,
		Plays: append([]Play(nil), next.CurrentTrick...), Score: next.Seats[winnerIndex].Score})
	next.CurrentTrick = nil
	next.Leader, next.Turn = winner, winner
	if !allHandsEmpty(next) {
		next.TrickNumber++
		return next, events, nil
	}
	next.Turn = ""
	next.Phase = PhaseHandComplete
	if next.CompletedHands+1 >= next.Config.MaxHands {
		next.CompletedHands++
		next.Phase = PhaseGameComplete
		events = append(events, Event{Kind: EventHandCompleted, Revision: next.Revision, Hand: next.HandNumber},
			Event{Kind: EventGameCompleted, Revision: next.Revision, Hand: next.HandNumber, Winners: gameWinners(next.Seats)})
		return next, events, nil
	}
	var revealed *RuleInstance
	next, revealed, err = RecordCompletedHand(next, catalog)
	if err != nil {
		return state, nil, transitionError(ErrInvalidState, err.Error())
	}
	events = append(events, Event{Kind: EventHandCompleted, Revision: next.Revision, Hand: next.HandNumber})
	if revealed != nil {
		events = append(events, Event{Kind: EventRuleRevealed, Revision: next.Revision, Hand: next.HandNumber, Rule: revealed.DefinitionID, RuleInstanceID: revealed.InstanceID})
	}
	return next, events, nil
}

func indexOfSeat(seats []Seat, id SeatID) int {
	for i, seat := range seats {
		if seat.ID == id {
			return i
		}
	}
	return -1
}

func allHandsEmpty(state MatchState) bool {
	for _, seat := range state.Seats {
		if len(state.Hands[seat.ID]) > 0 {
			return false
		}
	}
	return true
}

func gameWinners(seats []Seat) []SeatID {
	if len(seats) == 0 {
		return nil
	}
	best := seats[0].Score
	for _, seat := range seats[1:] {
		if seat.Score > best {
			best = seat.Score
		}
	}
	var winners []SeatID
	for _, seat := range seats {
		if seat.Score == best {
			winners = append(winners, seat.ID)
		}
	}
	return winners
}

func cardByID(id CardID) (StandardCard, bool) {
	for _, card := range StandardDeck() {
		if card.ID == id {
			return card, true
		}
	}
	return StandardCard{}, false
}

func activeEffects(field []RuleInstance, catalog Catalog) (SuitEffect, EffectKind, error) {
	resolved, err := ResolveRules(field, catalog, DefaultConfig())
	if err != nil {
		return "", "", err
	}
	play, err := resolved.Effect(MechanicPlayRestriction)
	if err != nil {
		return "", "", err
	}
	winner, err := resolved.Effect(MechanicRankComparison)
	if err != nil {
		return "", "", err
	}
	if _, err = resolved.Effect(MechanicWinnerEligibility); err != nil {
		return "", "", err
	}
	playEffect := EffectFollow
	if play.Value == ValueAnySuit {
		playEffect = EffectAnySuit
	}
	winnerEffect := EffectHighLead
	if winner.Value == ValueLow {
		winnerEffect = EffectLowLead
	}
	return playEffect, winnerEffect, nil
}

func cloneState(state MatchState) MatchState {
	next := state
	next.Seats = slices.Clone(state.Seats)
	next.Hands = make(map[SeatID][]CardID, len(state.Hands))
	for id, hand := range state.Hands {
		next.Hands[id] = slices.Clone(hand)
	}
	next.Captured = make(map[SeatID][]CardID, len(state.Captured))
	for id, cards := range state.Captured {
		next.Captured[id] = slices.Clone(cards)
	}
	next.StandardDraw = slices.Clone(state.StandardDraw)
	next.RuleDraw = slices.Clone(state.RuleDraw)
	next.CurrentTrick = slices.Clone(state.CurrentTrick)
	next.Field = slices.Clone(state.Field)
	next.Shuffles = make([]ShuffleRecord, len(state.Shuffles))
	for i, record := range state.Shuffles {
		next.Shuffles[i] = ShuffleRecord{Kind: record.Kind, Order: slices.Clone(record.Order)}
	}
	return next
}
