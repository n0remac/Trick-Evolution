package gameui

import "github.com/n0remac/trick-evolution/domain"

// Uses only the projected public auction and this bot's own dealt cards.
func chooseBotBid(view domain.PlayerView) int {
	a := view.Public.Auction
	if a == nil || a.Contract != nil {
		return 0
	}
	want := a.HighBid + 1
	if want < a.Min {
		want = a.Min
	}
	if want > a.Max {
		return 0
	}
	strength := 0
	for _, id := range view.Hand {
		card, ok := domain.CardDetails(id)
		if !ok {
			continue
		}
		if card.Rank >= 13 {
			strength++
		}
		if card.Suit == view.Public.TrumpSuit && card.Rank >= 11 {
			strength++
		}
	}
	if a.Metric == domain.ValueBidTricks && want > 1+strength {
		return 0
	}
	if a.Metric == domain.ValueBidPoints && want > 2+strength {
		return 0
	}
	return want
}
