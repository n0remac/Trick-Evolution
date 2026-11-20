package gameui

import (
	"fmt"
	"strings"

	h "github.com/n0remac/GoDom/html"
	"github.com/n0remac/trick-evolution/domain"
)

func renderScorecard(card *domain.Scorecard, name func(domain.SeatID) string, class string) *h.Node {
	title := fmt.Sprintf("Hand %d scorecard", card.HandNumber)
	if card.Round > 0 {
		title = fmt.Sprintf("Round %d · %s", card.Round, title)
	}
	if card.Partial {
		title = fmt.Sprintf("Hand %d · completed tricks only", card.HandNumber)
	}
	parts := []*h.Node{h.Class("scorecard " + class), h.H2(h.T(title))}
	for _, player := range card.Players {
		items := []*h.Node{h.Class("scorecard-player"), h.H3(h.T(fmt.Sprintf("%s · hand total %+d", name(player.Seat), player.Total)))}
		if len(player.Awards) == 0 {
			items = append(items, h.P(h.Class("muted"), h.T("No points awarded.")))
		}
		for _, award := range player.Awards {
			detail := fmt.Sprintf("%s · %+d", award.Title, award.Points)
			if award.Rule == domain.RuleGamePoint {
				detail = fmt.Sprintf("Game Point · %+d match point (captured-card tally %d)", award.Points, award.Tally)
			}
			if award.Card != "" {
				detail += " · " + cardLabel(award.Card)
			}
			if award.CardLabel != "" {
				detail += " · " + award.CardLabel
			}
			if award.HiddenCard {
				detail += " · retained card hidden"
			}
			if award.Trick > 0 {
				detail += fmt.Sprintf(" · trick %d", award.Trick)
			}
			items = append(items, h.P(h.T(detail)))
		}
		if tally := player.GameTally; tally != nil {
			terms := make([]string, 0, len(tally.Cards))
			for _, item := range tally.Cards {
				terms = append(terms, fmt.Sprintf("%s=%d", item.Label, item.Value))
			}
			math := "no scoring cards"
			if len(terms) > 0 {
				math = strings.Join(terms, " + ")
			}
			items = append(items, h.P(h.Class("game-tally"), h.T(fmt.Sprintf("Game tally: %s = %d (10=10, A=4, K=3, Q=2, J=1)", math, tally.Total))))
		}
		parts = append(parts, h.Section(items...))
	}
	for _, note := range card.Notes {
		parts = append(parts, h.P(h.Class("scorecard-note"), h.T(note.Text)))
	}
	return h.Section(parts...)
}
