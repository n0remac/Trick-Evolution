package gameui

import (
	"errors"
	"fmt"

	h "github.com/n0remac/GoDom/html"
	"github.com/n0remac/trick-evolution/domain"
)

func draftForm(rule domain.RuleID, label, boundary string, revision uint64) *h.Node {
	return h.Form(h.Attr("ws-send", ""), h.Input(h.Type("hidden"), h.Name("type"), h.Value("choose_rule")),
		h.Input(h.Type("hidden"), h.Name("revision"), h.Value(fmt.Sprint(revision))),
		h.Input(h.Type("hidden"), h.Name("boundaryId"), h.Value(boundary)),
		h.Input(h.Type("hidden"), h.Name("rule"), h.Value(string(rule))),
		h.Button(h.Type("submit"), h.T(label)))
}

func renderDraftChoices(public domain.PublicView, catalog domain.Catalog, scoreName func(domain.SeatID) string) *h.Node {
	choices := []*h.Node{h.Class("draft-choices"), h.H2(h.T("Choose a rule for the next round"))}
	if public.LastScorecard != nil && public.LastScorecard.HandNumber == public.HandNumber {
		choices = append(choices, renderScorecard(public.LastScorecard, scoreName, "scorecard-draft"))
	}
	for _, id := range public.DraftOptions {
		if definition, ok := catalog.Definition(id); ok {
			card := []*h.Node{h.Class("draft-card"), h.NewNode("strong", []*h.Node{h.T(definition.Title)}), h.P(h.T(definition.Text))}
			if cue := public.DraftCues[id]; cue != "" {
				card = append(card, h.P(h.Class("draft-cue"), h.T(cue)))
			}
			card = append(card, draftForm(id, "Take "+definition.Title, public.PendingBoundaryID, public.Revision))
			choices = append(choices, h.Div(card...))
		}
	}
	choices = append(choices, draftForm("", "Keep current rules", public.PendingBoundaryID, public.Revision))
	return h.Div(choices...)
}

func (r *room) dispatchDraftCommand(c *client, cmd command) (domain.MatchState, []domain.Event, error) {
	if r.state == nil {
		return domain.MatchState{}, nil, errors.New("game has not started")
	}
	actor := c.seat
	if r.config.Mode == domain.ModeEndless && r.seats[0].Bot {
		actor = r.seats[0].ID
	}
	return domain.Step(*r.state, domain.Action{Kind: domain.ActionChooseRule, Seat: actor, Rule: cmd.Rule, BoundaryID: cmd.BoundaryID}, r.server.catalog)
}
