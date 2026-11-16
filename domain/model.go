// Package domain contains the transport-independent game model and pure engine.
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
	PhaseAwaitingStart  Phase = "awaiting_start"
	PhasePlayingTrick   Phase = "playing_trick"
	PhaseHandComplete   Phase = "hand_complete"
	PhaseAwaitingChoice Phase = "awaiting_choice"
	PhaseGameComplete   Phase = "game_complete"
)

type ActionKind string

const (
	ActionStartHand  ActionKind = "start_hand"
	ActionPlayCard   ActionKind = "play_card"
	ActionEndGame    ActionKind = "end_game"
	ActionLeaveGame  ActionKind = "leave_game"
	ActionChooseRule ActionKind = "choose_rule"
)

// Action is the future Step input. Seat is always supplied by a trusted adapter.
type Action struct {
	Kind       ActionKind
	Seat       SeatID
	Card       CardID   // Only used for play_card.
	DeckOrder  []CardID // Full chosen order, required for start_hand.
	Rule       RuleID
	BoundaryID string
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
	Mode         MatchMode
	HandSize     int
	RevealEvery  int      // Completed hands between rule reveals.
	MaxHands     int      // The match ends after this many completed hands.
	InitialRules []RuleID // Optional visible replacements, in selection order.
}

type MatchMode string

const (
	ModeFinite  MatchMode = ""
	ModeEndless MatchMode = "endless"
)

func DefaultConfig() Config { return Config{HandSize: 5, RevealEvery: 3, MaxHands: 10} }

func (c Config) Validate(seatCount int) error {
	if c.Mode != ModeFinite && c.Mode != ModeEndless {
		return fmt.Errorf("unknown match mode %q", c.Mode)
	}
	if seatCount < 2 || seatCount > 4 {
		return fmt.Errorf("seat count must be 2 through 4")
	}
	if c.HandSize < 1 || c.HandSize > 26 || c.HandSize*seatCount > 52 {
		return fmt.Errorf("hand size cannot be dealt from one standard deck")
	}
	if c.RevealEvery < 1 || c.RevealEvery > 100 {
		return fmt.Errorf("reveal interval must be 1 through 100 completed hands")
	}
	if c.MaxHands < 1 || c.MaxHands > 100 {
		return fmt.Errorf("max hands must be between 1 and 100")
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
	Revision          uint64
	CatalogVersion    int
	Config            Config
	Phase             Phase
	Seats             []Seat
	Hands             map[SeatID][]CardID
	Carried           map[SeatID]CardID // Incoming private card for this hand, if any.
	Retained          map[SeatID]CardID // Private leftover after a completed Keep One hand.
	Captured          map[SeatID][]CardID
	DealOwner         map[CardID]SeatID // Private until hand settlement.
	HandAwards        []HandAward
	StandardDraw      []CardID
	RuleDraw          []RuleID
	CurrentTrick      []Play
	TrumpSuit         Suit // Empty before Pitch's first lead, or with No Trump.
	BrokenSpades      bool // Public facts, tracked even before Break Spades is active.
	BrokenHearts      bool
	Dealer            SeatID
	Leader            SeatID
	Turn              SeatID
	HandNumber        int
	TrickNumber       int
	DealSize          int // Effective cards per player, frozen at this hand's deal.
	TricksPerHand     int // Completed tricks required for this hand.
	CompletedHands    int
	EndedEarly        bool
	EndedBy           SeatID
	Field             []RuleInstance
	Round             int
	RoundHand         int
	PendingBoundaryID string
	DraftOptions      []RuleID
	TableCue          string
	Shuffles          []ShuffleRecord
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
	if err := ValidateInitialSetup(cfg, len(ids), catalog); err != nil {
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
	selected := map[RuleID]bool{}
	for _, id := range cfg.InitialRules {
		selected[id] = true
	}
	filtered := rules[:0]
	for _, id := range rules {
		if !selected[id] {
			filtered = append(filtered, id)
		}
	}
	rules = filtered
	source.Shuffle(len(rules), func(i, j int) { rules[i], rules[j] = rules[j], rules[i] })
	ruleOrder := make([]string, len(rules))
	for i, id := range rules {
		ruleOrder[i] = string(id)
	}
	starting, err := StartingRules(cfg, catalog)
	if err != nil {
		return MatchState{}, err
	}
	plan, err := HandPlanForRules(cfg, len(ids), starting)
	if err != nil {
		return MatchState{}, err
	}
	state := matchFromOrders(ids, cfg, catalog, standardOrder, ruleOrder, plan)
	resolved, err := ResolveRules(state.Field, catalog, cfg)
	if err != nil {
		return MatchState{}, err
	}
	if err := resolved.ValidateMatchEffects(cfg); err != nil {
		return MatchState{}, err
	}
	state.TrumpSuit, err = TrumpForTrick(resolved, "", nil)
	if err != nil {
		return MatchState{}, err
	}
	return state, nil
}

// ReplayMatch verifies recorded deck orders and reconstructs the same first deal.
func ReplayMatch(ids []SeatID, cfg Config, catalog Catalog, records []ShuffleRecord) (MatchState, error) {
	if err := cfg.Validate(len(ids)); err != nil {
		return MatchState{}, err
	}
	if err := catalog.Validate(); err != nil {
		return MatchState{}, err
	}
	if err := ValidateInitialSetup(cfg, len(ids), catalog); err != nil {
		return MatchState{}, err
	}
	if len(records) != 2 || records[0].Kind != StandardDeckKind || records[1].Kind != RuleDeckKind {
		return MatchState{}, fmt.Errorf("expected standard and rule shuffle records")
	}
	if err := validateOrder(records[0].Order, standardIDs()); err != nil {
		return MatchState{}, fmt.Errorf("standard shuffle: %w", err)
	}
	rules := catalog.RuleDeck()
	selected := map[RuleID]bool{}
	for _, id := range cfg.InitialRules {
		selected[id] = true
	}
	filtered := rules[:0]
	for _, id := range rules {
		if !selected[id] {
			filtered = append(filtered, id)
		}
	}
	rules = filtered
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
	starting, err := StartingRules(cfg, catalog)
	if err != nil {
		return MatchState{}, err
	}
	plan, err := HandPlanForRules(cfg, len(ids), starting)
	if err != nil {
		return MatchState{}, err
	}
	state := matchFromOrders(ids, cfg, catalog, records[0].Order, records[1].Order, plan)
	resolved, err := ResolveRules(state.Field, catalog, cfg)
	if err != nil {
		return MatchState{}, err
	}
	if err := resolved.ValidateMatchEffects(cfg); err != nil {
		return MatchState{}, err
	}
	state.TrumpSuit, err = TrumpForTrick(resolved, "", nil)
	if err != nil {
		return MatchState{}, err
	}
	return state, nil
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

func matchFromOrders(ids []SeatID, cfg Config, catalog Catalog, standardOrder, ruleOrder []string, plan HandPlan) MatchState {
	s := MatchState{CatalogVersion: catalog.Version, Config: cfg, Phase: PhasePlayingTrick,
		Seats: make([]Seat, len(ids)), Hands: make(map[SeatID][]CardID, len(ids)), Captured: make(map[SeatID][]CardID, len(ids)),
		RuleDraw: make([]RuleID, len(ruleOrder)), HandNumber: 1, TrickNumber: 1, DealSize: plan.DealSize, TricksPerHand: plan.Tricks,
		Field: StartingFieldWithConfig(catalog, cfg), DealOwner: make(map[CardID]SeatID),
		Shuffles: []ShuffleRecord{{Kind: StandardDeckKind, Order: append([]string(nil), standardOrder...)}, {Kind: RuleDeckKind, Order: append([]string(nil), ruleOrder...)}},
	}
	for i, id := range ids {
		s.Seats[i].ID = id
		s.Hands[id] = make([]CardID, 0, plan.DealSize)
	}
	for i, id := range ruleOrder {
		s.RuleDraw[i] = RuleID(id)
	}
	// Deal clockwise, one card at a time, starting left of dealer (seat zero).
	for card := 0; card < plan.DealSize; card++ {
		for offset := 1; offset <= len(ids); offset++ {
			seat := ids[offset%len(ids)]
			id := CardID(standardOrder[card*len(ids)+offset-1])
			s.Hands[seat] = append(s.Hands[seat], id)
			s.DealOwner[id] = seat
		}
	}
	for _, id := range standardOrder[plan.DealSize*len(ids):] {
		s.StandardDraw = append(s.StandardDraw, CardID(id))
	}
	s.Dealer, s.Leader, s.Turn = ids[0], ids[1%len(ids)], ids[1%len(ids)]
	if cfg.Mode == ModeEndless {
		s.Round, s.RoundHand = 1, 1
		s.TableCue = "The first round lasts three hands."
	}
	return s
}
