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
	var effect domain.EffectKind
	for _, rule := range view.Public.Rules {
		if !rule.Active {
			continue
		}
		definition, ok := catalog.Definition(rule.Instance.DefinitionID)
		if !ok {
			return "", fmt.Errorf("unknown active rule %s", rule.Instance.DefinitionID)
		}
		if definition.WinnerEffect != "" {
			if effect != "" {
				return "", fmt.Errorf("multiple winner effects")
			}
			effect = definition.WinnerEffect
		}
	}
	if effect != domain.EffectHighLead && effect != domain.EffectLowLead {
		return "", fmt.Errorf("unsupported winner effect %s", effect)
	}
	strength := func(card domain.StandardCard) int {
		if effect == domain.EffectLowLead {
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
	if len(view.Public.CurrentTrick) == 0 {
		// Lead a strong card in a longer suit to preserve later options.
		counts := map[domain.Suit]int{}
		for _, id := range view.Hand {
			counts[cardFor(id).Suit]++
		}
		best := legal[0]
		for _, id := range legal[1:] {
			card, current := cardFor(id), cardFor(best)
			if strength(card) > strength(current) ||
				(strength(card) == strength(current) && counts[card.Suit] > counts[current.Suit]) {
				best = id
			}
		}
		return best, nil
	}
	lead := cardFor(view.Public.CurrentTrick[0].Card).Suit
	winningStrength := -1
	for _, play := range view.Public.CurrentTrick {
		card := cardFor(play.Card)
		if card.Suit == lead && strength(card) > winningStrength {
			winningStrength = strength(card)
		}
	}
	// Win with the least valuable card that currently beats the table.
	var cheapestWin domain.CardID
	for _, id := range legal {
		card := cardFor(id)
		if card.Suit == lead && strength(card) > winningStrength &&
			(cheapestWin == "" || strength(card) < strength(cardFor(cheapestWin))) {
			cheapestWin = id
		}
	}
	if cheapestWin != "" {
		return cheapestWin, nil
	}
	// Otherwise shed the weakest legal card, including off-suit when allowed.
	weakest := legal[0]
	for _, id := range legal[1:] {
		if strength(cardFor(id)) < strength(cardFor(weakest)) {
			weakest = id
		}
	}
	return weakest, nil
}
