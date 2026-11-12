// Package domain contains the provisional, transport-independent F1 game model.
// The existing multiplayer prototype continues to use package cards until F3.
package domain

import (
	"fmt"
	"math/rand"
)

type CardID string
type SeatID string

type Suit string

const (
	Hearts   Suit = "H"
	Diamonds Suit = "D"
	Clubs    Suit = "C"
	Spades   Suit = "S"
)

type StandardCard struct {
	ID   CardID
	Suit Suit
	Rank uint8 // 2 through 14; ace is 14.
}

// StandardDeck has a stable order and identity independent of the print store.
func StandardDeck() []StandardCard {
	deck := make([]StandardCard, 0, 52)
	for _, suit := range []Suit{Hearts, Diamonds, Clubs, Spades} {
		for rank := uint8(2); rank <= 14; rank++ {
			deck = append(deck, StandardCard{
				ID:   CardID(fmt.Sprintf("std:%s:%02d", suit, rank)),
				Suit: suit, Rank: rank,
			})
		}
	}
	return deck
}

type Phase string

const (
	PhaseAwaitingStart Phase = "awaiting_start"
	PhasePlayingTrick  Phase = "playing_trick"
	PhaseHandComplete  Phase = "hand_complete"
	PhaseGameComplete  Phase = "game_complete"
)

type ActionKind string

const (
	ActionStartHand ActionKind = "start_hand"
	ActionPlayCard  ActionKind = "play_card"
)

// Action is the future Step input. Seat is always supplied by a trusted adapter.
type Action struct {
	Kind ActionKind
	Seat SeatID
	Card CardID // Only used for play_card.
}

type Seat struct {
	ID        SeatID
	Score     int
	TricksWon int
}

type Play struct {
	Seat SeatID
	Card CardID
}

type Config struct {
	HandSize    int
	RevealEvery int // Completed hands between rule reveals, 3 through 5.
}

func DefaultConfig() Config { return Config{HandSize: 5, RevealEvery: 3} }

func (c Config) Validate(seatCount int) error {
	if seatCount < 2 || seatCount > 4 {
		return fmt.Errorf("seat count must be 2 through 4")
	}
	if c.HandSize < 1 || c.HandSize > 13 || c.HandSize*seatCount > 52 {
		return fmt.Errorf("hand size cannot be dealt from one standard deck")
	}
	if c.RevealEvery < 3 || c.RevealEvery > 5 {
		return fmt.Errorf("reveal interval must be 3 through 5 completed hands")
	}
	return nil
}

type DeckKind string

const (
	StandardDeckKind DeckKind = "standard"
	RuleDeckKind     DeckKind = "rule"
)

// ShuffleRecord stores the complete resulting order, so future transitions do
// not need a random source and a setup can be replayed without guessing RNG state.
type ShuffleRecord struct {
	Kind  DeckKind
	Order []string
}

type MatchState struct {
	CatalogVersion int
	Config         Config
	Phase          Phase
	Seats          []Seat
	Hands          map[SeatID][]CardID
	StandardDraw   []CardID
	RuleDraw       []RuleID
	CurrentTrick   []Play
	Dealer         SeatID
	Leader         SeatID
	Turn           SeatID
	HandNumber     int
	TrickNumber    int
	CompletedHands int
	Field          []RuleInstance
	Shuffles       []ShuffleRecord
}

// ShuffleSource is injected only at setup. Callers can supply a seeded source
// or a fixture source; the resulting orders are always recorded in MatchState.
type ShuffleSource interface {
	Shuffle(n int, swap func(i, j int))
}

func SeededShuffle(seed int64) ShuffleSource { return rand.New(rand.NewSource(seed)) }

func NewMatch(ids []SeatID, cfg Config, catalog Catalog, source ShuffleSource) (MatchState, error) {
	if err := cfg.Validate(len(ids)); err != nil {
		return MatchState{}, err
	}
	if err := catalog.Validate(); err != nil {
		return MatchState{}, err
	}
	if source == nil {
		return MatchState{}, fmt.Errorf("shuffle source is required")
	}
	seen := map[SeatID]bool{}
	seats := make([]Seat, len(ids))
	for i, id := range ids {
		if id == "" || seen[id] {
			return MatchState{}, fmt.Errorf("seat IDs must be nonempty and unique")
		}
		seen[id] = true
		seats[i] = Seat{ID: id}
	}
	standard := StandardDeck()
	source.Shuffle(len(standard), func(i, j int) { standard[i], standard[j] = standard[j], standard[i] })
	standardOrder := make([]string, len(standard))
	for i, card := range standard {
		standardOrder[i] = string(card.ID)
	}
	rules := catalog.RuleDeck()
	source.Shuffle(len(rules), func(i, j int) { rules[i], rules[j] = rules[j], rules[i] })
	ruleOrder := make([]string, len(rules))
	for i, id := range rules {
		ruleOrder[i] = string(id)
	}
	return matchFromOrders(ids, cfg, catalog.Version, standardOrder, ruleOrder), nil
}

// ReplayMatch verifies recorded deck orders and reconstructs the same first deal.
func ReplayMatch(ids []SeatID, cfg Config, catalog Catalog, records []ShuffleRecord) (MatchState, error) {
	if err := cfg.Validate(len(ids)); err != nil {
		return MatchState{}, err
	}
	if err := catalog.Validate(); err != nil {
		return MatchState{}, err
	}
	if len(records) != 2 || records[0].Kind != StandardDeckKind || records[1].Kind != RuleDeckKind {
		return MatchState{}, fmt.Errorf("expected standard and rule shuffle records")
	}
	if err := validateOrder(records[0].Order, standardIDs()); err != nil {
		return MatchState{}, fmt.Errorf("standard shuffle: %w", err)
	}
	rules := catalog.RuleDeck()
	expected := make([]string, len(rules))
	for i, id := range rules {
		expected[i] = string(id)
	}
	if err := validateOrder(records[1].Order, expected); err != nil {
		return MatchState{}, fmt.Errorf("rule shuffle: %w", err)
	}
	seen := map[SeatID]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			return MatchState{}, fmt.Errorf("seat IDs must be nonempty and unique")
		}
		seen[id] = true
	}
	return matchFromOrders(ids, cfg, catalog.Version, records[0].Order, records[1].Order), nil
}

func standardIDs() []string {
	deck := StandardDeck()
	ids := make([]string, len(deck))
	for i, card := range deck {
		ids[i] = string(card.ID)
	}
	return ids
}

func validateOrder(got, expected []string) error {
	if len(got) != len(expected) {
		return fmt.Errorf("wrong deck length")
	}
	counts := make(map[string]int, len(expected))
	for _, id := range expected {
		counts[id]++
	}
	for _, id := range got {
		counts[id]--
		if counts[id] < 0 {
			return fmt.Errorf("unknown or duplicate ID %q", id)
		}
	}
	return nil
}

func matchFromOrders(ids []SeatID, cfg Config, version int, standardOrder, ruleOrder []string) MatchState {
	s := MatchState{CatalogVersion: version, Config: cfg, Phase: PhasePlayingTrick,
		Seats: make([]Seat, len(ids)), Hands: make(map[SeatID][]CardID, len(ids)),
		RuleDraw: make([]RuleID, len(ruleOrder)), HandNumber: 1, TrickNumber: 1,
		Field:    []RuleInstance{{InstanceID: "rule:initial:0", DefinitionID: RuleHighCardWins, Sequence: 0, IntroducedAfterHand: 0}},
		Shuffles: []ShuffleRecord{{Kind: StandardDeckKind, Order: append([]string(nil), standardOrder...)}, {Kind: RuleDeckKind, Order: append([]string(nil), ruleOrder...)}},
	}
	for i, id := range ids {
		s.Seats[i].ID = id
		s.Hands[id] = make([]CardID, 0, cfg.HandSize)
	}
	for i, id := range ruleOrder {
		s.RuleDraw[i] = RuleID(id)
	}
	// Deal clockwise, one card at a time, starting left of dealer (seat zero).
	for card := 0; card < cfg.HandSize; card++ {
		for offset := 1; offset <= len(ids); offset++ {
			seat := ids[offset%len(ids)]
			s.Hands[seat] = append(s.Hands[seat], CardID(standardOrder[card*len(ids)+offset-1]))
		}
	}
	for _, id := range standardOrder[cfg.HandSize*len(ids):] {
		s.StandardDraw = append(s.StandardDraw, CardID(id))
	}
	s.Dealer, s.Leader, s.Turn = ids[0], ids[1%len(ids)], ids[1%len(ids)]
	return s
}
