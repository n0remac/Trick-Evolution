package gameui

import (
	"fmt"
	"sort"
	"time"

	"github.com/n0remac/trick-evolution/domain"
)

const defaultBotDelay = 60 * time.Millisecond

// chooseBotCard receives only a player projection: public facts and its own
// hand. It has no reference to MatchState or any opponent's private cards.
func chooseBotCard(view domain.PlayerView, catalog domain.Catalog) (domain.CardID, error) {
	legal, err := domain.LegalCards(view, catalog)
	if err != nil {
		return "", err
	}
	if len(legal) == 0 {
		return "", fmt.Errorf("bot has no legal card")
	}
	resolved, err := domain.EffectiveForView(view.Public, catalog)
	if err != nil {
		return "", err
	}
	rank, err := resolved.Effect(domain.MechanicRankComparison)
	if err != nil {
		return "", err
	}
	strength := func(card domain.StandardCard) int {
		if rank.Value == domain.ValueLow {
			return 16 - int(card.Rank)
		}
		return int(card.Rank)
	}
	cardFor := func(id domain.CardID) domain.StandardCard {
		card, _ := domain.CardDetails(id) // LegalCards already validates each ID.
		return card
	}
	// Stable ID order breaks every tie without randomness or deal-order bias.
	sort.Slice(legal, func(i, j int) bool { return legal[i] < legal[j] })
	trump, err := domain.TrumpForTrick(resolved, view.Public.TrumpSuit, view.Public.CurrentTrick)
	if err != nil {
		return "", err
	}
	if len(view.Public.CurrentTrick) == 0 {
		// Pitch chooses a suit for the hand: prefer a longer suit, then a
		// strong card within it. Fixed trump raises a legal spade lead.
		counts := map[domain.Suit]int{}
		for _, id := range view.Hand {
			counts[cardFor(id).Suit]++
		}
		trumpRule, _ := resolved.Effect(domain.MechanicTrump)
		leadValue := func(card domain.StandardCard) int {
			if trumpRule.Value == domain.ValueFirstLeadTrump && view.Public.TrumpSuit == "" {
				return counts[card.Suit]*20 + strength(card)
			}
			value := strength(card)*16 + counts[card.Suit]
			if trump != "" && card.Suit == trump {
				value += 256
			}
			return value
		}
		best := legal[0]
		for _, id := range legal[1:] {
			card, current := cardFor(id), cardFor(best)
			if leadValue(card) > leadValue(current) || leadValue(card) == leadValue(current) && counts[card.Suit] > counts[current.Suit] {
				best = id
			}
		}
		return best, nil
	}
	cost := func(card domain.StandardCard) int {
		value := strength(card)
		if trump != "" && card.Suit == trump {
			value += 16 // Save trump when an ordinary card can win or shed.
		}
		return value
	}
	// Win with the least valuable card that currently beats the table.
	var cheapestWin domain.CardID
	for _, id := range legal {
		candidate := append(append([]domain.Play(nil), view.Public.CurrentTrick...), domain.Play{Seat: view.Seat, Card: id})
		winner, err := domain.WinningPlay(candidate, resolved, view.Public.TrumpSuit)
		if err != nil {
			return "", err
		}
		if winner.Seat == view.Seat && (cheapestWin == "" || cost(cardFor(id)) < cost(cardFor(cheapestWin))) {
			cheapestWin = id
		}
	}
	if cheapestWin != "" {
		return cheapestWin, nil
	}
	// Otherwise shed the cheapest legal card, preserving trump if possible.
	weakest := legal[0]
	for _, id := range legal[1:] {
		if cost(cardFor(id)) < cost(cardFor(weakest)) {
			weakest = id
		}
	}
	return weakest, nil
}
