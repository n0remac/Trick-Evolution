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
	if c := card.Contract; c != nil {
		unit := "points"
		if c.Metric == domain.ValueBidTricks {
			unit = "tricks"
		}
		outcome := "made"
		if card.Partial {
			outcome = "unsettled"
		} else if !c.Made {
			outcome = "failed"
		}
		line := fmt.Sprintf("%s bid %d %s · actual %d · %s", name(c.Bidder), c.Bid, unit, c.Actual, outcome)
		if c.Fallback {
			line += " · all-pass dealer fallback"
		}
		if c.Team >= 0 {
			line = fmt.Sprintf("Team %d · %s", c.Team+1, line)
		}
		if c.Penalty > 0 {
			line += fmt.Sprintf(" · set penalty −%d", c.Penalty)
		}
		parts = append(parts, h.P(h.Class("scorecard-contract"), h.T(line)))
	}
	for _, team := range card.Teams {
		members := make([]string, 0, len(team.Seats))
		for _, id := range team.Seats {
			members = append(members, name(id))
		}
		line := fmt.Sprintf("Team %d · %s · hand total %+d", team.Team+1, strings.Join(members, " & "), team.Total)
		if team.GameTally > 0 {
			line += fmt.Sprintf(" · Game tally %d", team.GameTally)
		}
		parts = append(parts, h.P(h.Class("scorecard-team"), h.T(line)))
	}
	for _, player := range card.Players {
		items := []*h.Node{h.Class("scorecard-player"), h.H3(h.T(fmt.Sprintf("%s · hand total %+d", name(player.Seat), player.Total)))}
		if len(player.Awards) == 0 {
			items = append(items, h.P(h.Class("muted"), h.T("No points awarded.")))
		}
		for _, award := range player.Awards {
			detail := fmt.Sprintf("%s · %+d", award.Title, award.Points)
			if award.Rule == domain.RuleGamePoint {
				kind := "captured-card tally"
				if len(card.Teams) == 2 {
					kind = "team captured-card tally"
				}
				detail = fmt.Sprintf("Game Point · %+d match point (%s %d)", award.Points, kind, award.Tally)
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
