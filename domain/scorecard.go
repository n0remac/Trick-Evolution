package domain

import "fmt"

// Scorecard is a public snapshot of one hand, frozen before the next reveal.
// It keeps the previous roster when Endless changes seats at a round boundary.
type Scorecard struct {
	HandNumber int
	Round      int
	Partial    bool
	Players    []ScorecardPlayer
	Notes      []ScorecardNote
}

type ScorecardPlayer struct {
	Seat      SeatID
	Total     int
	Awards    []HandAward
	GameTally *GameTally
}

type GameTally struct {
	Cards []GameTallyCard
	Total int
}

type GameTallyCard struct {
	Label string
	Value int
}

type ScorecardNote struct {
	Rule RuleID
	Text string
}

func makeScorecard(state MatchState, rules RuleResolution, partial bool) *Scorecard {
	card := &Scorecard{HandNumber: state.HandNumber, Round: state.Round, Partial: partial}
	retained := make(map[CardID]bool, len(state.Retained))
	for _, id := range state.Retained {
		retained[id] = true
	}
	for _, seat := range state.Seats {
		entry := ScorecardPlayer{Seat: seat.ID}
		for _, award := range state.HandAwards {
			if award.Seat != seat.ID {
				continue
			}
			if retained[award.Card] {
				award.Card, award.HiddenCard = "", true
			} else if award.Card != "" {
				award.CardLabel = scorecardCardLabel(award.Card)
				award.Card = ""
			}
			entry.Total += award.Points
			entry.Awards = append(entry.Awards, award)
		}
		card.Players = append(card.Players, entry)
	}
	if partial {
		card.Notes = append(card.Notes, ScorecardNote{Text: "Only completed tricks count; the unfinished hand has no hand-end awards."})
		return card
	}
	active := func(mechanic Mechanic) bool { _, err := rules.Effect(mechanic); return err == nil }
	if active(MechanicGamePoint) {
		best, leaders := -1, 0
		for i, seat := range state.Seats {
			tally := &GameTally{}
			for _, id := range state.Captured[seat.ID] {
				standard, _ := cardByID(id)
				if value := gameCardValue(standard.Rank); value > 0 {
					tally.Cards = append(tally.Cards, GameTallyCard{Label: scorecardCardLabel(id), Value: value})
					tally.Total += value
				}
			}
			card.Players[i].GameTally = tally
			if tally.Total > best {
				best, leaders = tally.Total, 1
			} else if tally.Total == best {
				leaders++
			}
		}
		if leaders > 1 {
			card.Notes = append(card.Notes, ScorecardNote{Rule: RuleGamePoint, Text: fmt.Sprintf("Game Point tied at %d tally; no match point awarded.", best)})
		}
	}
	if state.TrumpSuit == "" {
		return card
	}
	var high, low, jack CardID
	for id := range state.DealOwner {
		standard, _ := cardByID(id)
		if standard.Suit != state.TrumpSuit {
			continue
		}
		if jack == "" && standard.Rank == 11 {
			jack = id
		}
		if high == "" || cardRank(id) > cardRank(high) {
			high = id
		}
		if low == "" || cardRank(id) < cardRank(low) {
			low = id
		}
	}
	if high == "" {
		for _, item := range []struct {
			active bool
			rule   RuleID
			title  string
		}{
			{active(MechanicHighTrump), RuleHighTrump, "High Trump"},
			{active(MechanicLowTrump), RuleLowTrump, "Low Trump"},
			{active(MechanicJackTrump), RuleJackTrump, "Trump Jack"},
		} {
			if item.active {
				card.Notes = append(card.Notes, ScorecardNote{item.rule, item.title + " had no dealt trump card to award."})
			}
		}
		return card
	}
	if active(MechanicLowTrump) && !scorecardHasAward(card, RuleLowTrump) {
		card.Notes = append(card.Notes, ScorecardNote{RuleLowTrump, "Lowest dealt trump was not captured; no Low Trump point awarded."})
	}
	if active(MechanicJackTrump) && !scorecardHasAward(card, RuleJackTrump) {
		message := "Trump Jack was not captured; no point awarded."
		if jack == "" {
			message = "Trump Jack was not dealt; no point awarded."
		}
		card.Notes = append(card.Notes, ScorecardNote{RuleJackTrump, message})
	}
	return card
}

func cardRank(id CardID) uint8 {
	card, _ := cardByID(id)
	return card.Rank
}

func scorecardCardLabel(id CardID) string {
	card, ok := cardByID(id)
	if !ok {
		return "unknown card"
	}
	rank := fmt.Sprint(card.Rank)
	switch card.Rank {
	case 11:
		rank = "J"
	case 12:
		rank = "Q"
	case 13:
		rank = "K"
	case 14:
		rank = "A"
	}
	suit := "?"
	switch card.Suit {
	case Spades:
		suit = "♠"
	case Hearts:
		suit = "♥"
	case Diamonds:
		suit = "♦"
	case Clubs:
		suit = "♣"
	}
	return rank + suit
}

func scorecardHasAward(card *Scorecard, rule RuleID) bool {
	for _, player := range card.Players {
		for _, award := range player.Awards {
			if award.Rule == rule {
				return true
			}
		}
	}
	return false
}

func cloneScorecard(card *Scorecard) *Scorecard {
	if card == nil {
		return nil
	}
	next := *card
	next.Notes = append([]ScorecardNote(nil), card.Notes...)
	next.Players = make([]ScorecardPlayer, len(card.Players))
	for i, player := range card.Players {
		next.Players[i] = player
		next.Players[i].Awards = append([]HandAward(nil), player.Awards...)
		if player.GameTally != nil {
			tally := *player.GameTally
			tally.Cards = append([]GameTallyCard(nil), player.GameTally.Cards...)
			next.Players[i].GameTally = &tally
		}
	}
	return &next
}
