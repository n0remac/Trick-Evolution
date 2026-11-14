package gameui

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"sort"

	"github.com/n0remac/trick-evolution/domain"
)

type preparedObservation struct {
	observation BotObservation
	config      domain.Config
	field       []domain.RuleInstance
	rules       []domain.RuleID
	unknown     []domain.StandardCard
	captured    map[domain.SeatID][]domain.CardID
	voids       map[domain.SeatID]map[domain.Suit]bool
	opponents   []domain.PublicSeat
}

// prepareObservation uses only a public history and one private bot hand.
// It does not accept a live MatchState or any opponent hand.
func prepareObservation(observation BotObservation, catalog domain.Catalog) (preparedObservation, error) {
	view := observation.View
	public := view.Public
	config := domain.Config{HandSize: public.HandSize, RevealEvery: public.RevealEvery, MaxHands: public.MaxHands}
	if err := config.Validate(len(public.Seats)); err != nil {
		return preparedObservation{}, err
	}
	if public.Phase != domain.PhasePlayingTrick || public.Turn != view.Seat {
		return preparedObservation{}, fmt.Errorf("bot observation is not at its turn")
	}
	if err := catalog.Validate(); err != nil {
		return preparedObservation{}, err
	}
	prepared := preparedObservation{observation: observation, config: config, captured: make(map[domain.SeatID][]domain.CardID), voids: make(map[domain.SeatID]map[domain.Suit]bool)}
	seatIDs := make(map[domain.SeatID]bool, len(public.Seats))
	botFound := false
	handCount := 0
	for _, seat := range public.Seats {
		if seat.ID == "" || seatIDs[seat.ID] || seat.HandCount < 0 {
			return preparedObservation{}, fmt.Errorf("invalid public seats")
		}
		seatIDs[seat.ID] = true
		prepared.captured[seat.ID] = nil
		handCount += seat.HandCount
		if seat.ID == view.Seat {
			botFound = true
			if seat.HandCount != len(view.Hand) {
				return preparedObservation{}, fmt.Errorf("bot hand count mismatch")
			}
		} else {
			prepared.opponents = append(prepared.opponents, seat)
		}
	}
	if !botFound || handCount+len(observation.Played) != config.HandSize*len(public.Seats) {
		return preparedObservation{}, fmt.Errorf("hand conservation mismatch")
	}
	// Completed tricks followed by the visible current trick must exactly
	// explain the accepted public card-play history.
	var flattened []domain.Play
	for _, trick := range observation.Won {
		if !seatIDs[trick.Winner] || len(trick.Plays) != len(public.Seats) {
			return preparedObservation{}, fmt.Errorf("invalid public trick history")
		}
		for _, play := range trick.Plays {
			flattened = append(flattened, play)
			prepared.captured[trick.Winner] = append(prepared.captured[trick.Winner], play.Card)
		}
	}
	flattened = append(flattened, public.CurrentTrick...)
	if len(flattened) != len(observation.Played) {
		return preparedObservation{}, fmt.Errorf("public play history length mismatch")
	}
	for i, play := range flattened {
		if !seatIDs[play.Seat] || observation.Played[i].Seat != play.Seat || observation.Played[i].Card != play.Card {
			return preparedObservation{}, fmt.Errorf("public play history mismatch")
		}
	}
	for _, seat := range public.Seats {
		if len(prepared.captured[seat.ID]) != seat.CapturedCount {
			return preparedObservation{}, fmt.Errorf("captured count mismatch")
		}
	}
	seen := make(map[domain.CardID]bool, 52)
	for _, id := range view.Hand {
		if _, ok := domain.CardDetails(id); !ok || seen[id] {
			return preparedObservation{}, fmt.Errorf("invalid bot hand")
		}
		seen[id] = true
	}
	brokenSpades, brokenHearts := false, false
	for i, play := range observation.Played {
		card, ok := domain.CardDetails(play.Card)
		if !ok || seen[play.Card] {
			return preparedObservation{}, fmt.Errorf("duplicate or invalid public card")
		}
		seen[play.Card] = true
		if play.LeadSuit == "" {
			return preparedObservation{}, fmt.Errorf("missing public lead suit")
		}
		// The first card of each trick defines the lead. All later records
		// must retain that same suit, even after future rule reveals.
		leadIndex := (i / len(public.Seats)) * len(public.Seats)
		lead, ok := domain.CardDetails(observation.Played[leadIndex].Card)
		if !ok || play.LeadSuit != lead.Suit {
			return preparedObservation{}, fmt.Errorf("inconsistent public lead suit")
		}
		if play.PlayRestriction != domain.ValueFollow && play.PlayRestriction != domain.ValueAnySuit && play.PlayRestriction != domain.ValueFollowOrTrump {
			return preparedObservation{}, fmt.Errorf("unknown observed play restriction")
		}
		if play.TrumpSuit != "" && play.TrumpSuit != domain.Hearts && play.TrumpSuit != domain.Diamonds && play.TrumpSuit != domain.Clubs && play.TrumpSuit != domain.Spades {
			return preparedObservation{}, fmt.Errorf("unknown observed trump suit")
		}
		if card.Suit != play.LeadSuit && card.Suit == domain.Spades {
			brokenSpades = true
		}
		if card.Suit != play.LeadSuit && card.Suit == domain.Hearts {
			brokenHearts = true
		}
		provesVoid := card.Suit != play.LeadSuit && (play.PlayRestriction == domain.ValueFollow ||
			play.PlayRestriction == domain.ValueFollowOrTrump && play.TrumpSuit != "" && card.Suit != play.TrumpSuit)
		if provesVoid {
			if prepared.voids[play.Seat] == nil {
				prepared.voids[play.Seat] = make(map[domain.Suit]bool)
			}
			prepared.voids[play.Seat][play.LeadSuit] = true
		}
	}
	if brokenSpades != public.BrokenSpades || brokenHearts != public.BrokenHearts {
		return preparedObservation{}, fmt.Errorf("public broken-suit facts disagree with plays")
	}
	for _, id := range view.Hand {
		card, _ := domain.CardDetails(id)
		if prepared.voids[view.Seat][card.Suit] {
			return preparedObservation{}, fmt.Errorf("bot hand contradicts public void")
		}
	}
	for _, card := range domain.StandardDeck() {
		if !seen[card.ID] {
			prepared.unknown = append(prepared.unknown, card)
		}
	}
	needed := 0
	for _, seat := range prepared.opponents {
		needed += seat.HandCount
	}
	if len(prepared.unknown)-needed != 52-config.HandSize*len(public.Seats) {
		return preparedObservation{}, fmt.Errorf("unknown pool has wrong size")
	}
	prepared.field = make([]domain.RuleInstance, len(public.Rules))
	for i, rule := range public.Rules {
		prepared.field[i] = rule.Instance
	}
	resolved, err := domain.ResolveRules(prepared.field, catalog, config)
	if err != nil || !reflect.DeepEqual(resolved.History, public.Rules) || !reflect.DeepEqual(resolved.ActiveField, public.ActiveField) || !reflect.DeepEqual(resolved.Effective, public.Effective) {
		return preparedObservation{}, fmt.Errorf("public rule projection mismatch")
	}
	revealed := make(map[domain.RuleID]bool)
	for _, instance := range prepared.field {
		definition, _ := catalog.Definition(instance.DefinitionID)
		if definition.InDeck {
			revealed[instance.DefinitionID] = true
		}
	}
	for _, id := range catalog.RuleDeck() {
		if !revealed[id] {
			prepared.rules = append(prepared.rules, id)
		}
	}
	if len(prepared.rules) != public.RulesRemaining {
		return preparedObservation{}, fmt.Errorf("remaining rule count mismatch")
	}
	return prepared, nil
}

// sampleWorld uses randomized augmenting-path matching. Each hand slot can
// take only cards allowed by its seat's inferred voids; unmatched cards form
// the synthetic undealt remainder. Nodes and context bound the search.
func (prepared preparedObservation) sampleWorld(ctx context.Context, random *rand.Rand, maxNodes int, catalog domain.Catalog) (domain.MatchState, error) {
	if maxNodes <= 0 {
		return domain.MatchState{}, fmt.Errorf("sample node budget exhausted")
	}
	public := prepared.observation.View.Public
	unknown := append([]domain.StandardCard(nil), prepared.unknown...)
	random.Shuffle(len(unknown), func(i, j int) { unknown[i], unknown[j] = unknown[j], unknown[i] })
	opponents := append([]domain.PublicSeat(nil), prepared.opponents...)
	sort.SliceStable(opponents, func(i, j int) bool {
		return len(prepared.voids[opponents[i].ID]) > len(prepared.voids[opponents[j].ID])
	})
	var slots []domain.SeatID
	for _, seat := range opponents {
		for i := 0; i < seat.HandCount; i++ {
			slots = append(slots, seat.ID)
		}
	}
	owner := make([]int, len(unknown))
	slotCard := make([]int, len(slots))
	for i := range owner {
		owner[i] = -1
	}
	for i := range slotCard {
		slotCard[i] = -1
	}
	nodes := 0
	var augment func(int, []bool) bool
	augment = func(slot int, visited []bool) bool {
		if ctx.Err() != nil || nodes >= maxNodes {
			return false
		}
		for card := range unknown {
			if visited[card] || prepared.voids[slots[slot]][unknown[card].Suit] {
				continue
			}
			visited[card] = true
			nodes++
			if nodes > maxNodes || ctx.Err() != nil {
				return false
			}
			if owner[card] == -1 || augment(owner[card], visited) {
				owner[card] = slot
				slotCard[slot] = card
				return true
			}
		}
		return false
	}
	for slot := range slots {
		if !augment(slot, make([]bool, len(unknown))) {
			if ctx.Err() != nil {
				return domain.MatchState{}, ctx.Err()
			}
			return domain.MatchState{}, fmt.Errorf("void-constrained sample failed within %d nodes", maxNodes)
		}
	}
	state := domain.MatchState{Revision: public.Revision, CatalogVersion: catalog.Version, Config: prepared.config,
		Phase: public.Phase, Seats: make([]domain.Seat, len(public.Seats)), Hands: make(map[domain.SeatID][]domain.CardID), Captured: make(map[domain.SeatID][]domain.CardID),
		CurrentTrick: append([]domain.Play(nil), public.CurrentTrick...), TrumpSuit: public.TrumpSuit, BrokenSpades: public.BrokenSpades, BrokenHearts: public.BrokenHearts, Dealer: public.Dealer, Leader: public.Leader, Turn: public.Turn,
		HandNumber: public.HandNumber, TrickNumber: public.TrickNumber, CompletedHands: public.CompletedHands,
		Field: append([]domain.RuleInstance(nil), prepared.field...), RuleDraw: append([]domain.RuleID(nil), prepared.rules...)}
	for i, seat := range public.Seats {
		state.Seats[i] = domain.Seat{ID: seat.ID, Score: seat.Score, TricksWon: seat.TricksWon}
		state.Captured[seat.ID] = append([]domain.CardID(nil), prepared.captured[seat.ID]...)
	}
	state.Hands[prepared.observation.View.Seat] = append([]domain.CardID(nil), prepared.observation.View.Hand...)
	used := make([]bool, len(unknown))
	for slot, card := range slotCard {
		state.Hands[slots[slot]] = append(state.Hands[slots[slot]], unknown[card].ID)
		used[card] = true
	}
	for i, card := range unknown {
		if !used[i] {
			state.StandardDraw = append(state.StandardDraw, card.ID)
		}
	}
	random.Shuffle(len(state.RuleDraw), func(i, j int) { state.RuleDraw[i], state.RuleDraw[j] = state.RuleDraw[j], state.RuleDraw[i] })
	return state, nil
}
