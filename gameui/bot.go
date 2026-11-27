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
	_, queenPenaltyRule := resolved.Effect(domain.MechanicAvoidQueenHearts)
	_, heartsPenaltyRule := resolved.Effect(domain.MechanicAvoidHearts)
	heartPenalty := func(card domain.StandardCard) int {
		if card.Suit != domain.Hearts {
			return 0
		}
		points := 0
		if heartsPenaltyRule == nil {
			points++
		}
		if card.Rank == 12 && queenPenaltyRule == nil {
			points += 5
		}
		return points
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
		_, lastBonusRule := resolved.Effect(domain.MechanicLastTrickBonus)
		lastTrick := view.Public.TricksPerHand
		if lastTrick == 0 {
			lastTrick = view.Public.HandSize
		}
		saveLastWinner := lastBonusRule == nil && view.Public.TrickNumber == lastTrick-1 && len(view.Hand) > 1
		leadValue := func(card domain.StandardCard) int {
			if trumpRule.Value == domain.ValueFirstLeadTrump && view.Public.TrumpSuit == "" {
				return counts[card.Suit]*20 + strength(card) - heartPenalty(card)*100
			}
			value := strength(card)*16 + counts[card.Suit]
			if saveLastWinner {
				value = (16-strength(card))*16 + counts[card.Suit]
			}
			if trump != "" && card.Suit == trump {
				value += 256
			}
			return value - heartPenalty(card)*100
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
			if _, err := resolved.Effect(domain.MechanicLowTrump); err == nil && card.Rank <= 5 {
				value += 24
			}
		}
		if _, err := resolved.Effect(domain.MechanicGamePoint); err == nil {
			switch card.Rank {
			case 10:
				value += 10
			case 14:
				value += 4
			case 13:
				value += 3
			case 12:
				value += 2
			case 11:
				value++
			}
		}
		return value
	}
	// Win with the least valuable card that currently beats the table.
	var cheapestWin domain.CardID
	var cheapestLoss domain.CardID
	bestWinNet := -1000
	trickPoints, _ := resolved.Effect(domain.MechanicTrickPoints)
	baseWin := 0
	if trickPoints.Value == domain.ValueOnePoint {
		baseWin++
	}
	lastTrick := view.Public.TricksPerHand
	if lastTrick == 0 {
		lastTrick = view.Public.HandSize
	}
	if _, err := resolved.Effect(domain.MechanicLastTrickBonus); err == nil && view.Public.TrickNumber == lastTrick {
		baseWin += 2
	}
	for _, play := range view.Public.CurrentTrick {
		baseWin -= heartPenalty(cardFor(play.Card))
	}
	for _, id := range legal {
		card := cardFor(id)
		candidate := append(append([]domain.Play(nil), view.Public.CurrentTrick...), domain.Play{Seat: view.Seat, Card: id})
		winner, err := domain.WinningPlay(candidate, resolved, view.Public.TrumpSuit)
		if err != nil {
			return "", err
		}
		if winner.Seat == view.Seat {
			net := baseWin - heartPenalty(card)
			if cheapestWin == "" || net > bestWinNet || net == bestWinNet && cost(card) < cost(cardFor(cheapestWin)) {
				cheapestWin, bestWinNet = id, net
			}
		} else if cheapestLoss == "" || cost(card)-heartPenalty(card)*20 < cost(cardFor(cheapestLoss))-heartPenalty(cardFor(cheapestLoss))*20 {
			cheapestLoss = id
		}
	}
	if team, ok := view.Public.HandTeams[view.Seat]; ok && cheapestLoss != "" {
		leader, err := domain.WinningPlay(view.Public.CurrentTrick, resolved, view.Public.TrumpSuit)
		if err == nil && leader.Seat != view.Seat && view.Public.HandTeams[leader.Seat] == team {
			// Keep a partner's winning trick, and avoid giving that partner penalties.
			best := cheapestLoss
			for _, id := range legal {
				candidate := append(append([]domain.Play(nil), view.Public.CurrentTrick...), domain.Play{Seat: view.Seat, Card: id})
				winner, err := domain.WinningPlay(candidate, resolved, view.Public.TrumpSuit)
				if err == nil && winner.Seat != view.Seat && cost(cardFor(id))+heartPenalty(cardFor(id))*100 < cost(cardFor(best))+heartPenalty(cardFor(best))*100 {
					best = id
				}
			}
			return best, nil
		}
	}
	if cheapestWin != "" && (bestWinNet >= 0 || cheapestLoss == "") {
		return cheapestWin, nil
	}
	if cheapestLoss != "" {
		return cheapestLoss, nil
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
