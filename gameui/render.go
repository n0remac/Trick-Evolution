package gameui

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	h "github.com/n0remac/GoDom/html"
	"github.com/n0remac/trick-evolution/cardtable"
	"github.com/n0remac/trick-evolution/domain"
)

// The page and every live game fragment are composed with GoDom. The outer
// live node stays mounted so the WebSocket and a future canvas can survive
// screen swaps.
func pageNode(room string) *h.Node {
	return h.Html(h.Attr("lang", "en"),
		h.Head(
			h.Meta(h.Charset("utf-8")),
			h.Meta(h.Attrs(map[string]string{"name": "viewport", "content": "width=device-width, initial-scale=1, viewport-fit=cover"})),
			h.Title(h.T("Trick Evolution")),
			h.Link(h.Rel("stylesheet"), h.Href("/game/style.css")),
			h.Script(h.Src("/game/vendor/htmx-2.0.11.min.js"), h.Attr("defer", "")),
			h.Script(h.Src("/game/vendor/htmx-ext-ws-2.0.4.min.js"), h.Attr("defer", "")),
			h.Script(h.Src("/game/game.js"), h.Type("module")),
		),
		h.Body(h.Main(h.Class("shell"),
			h.Header(h.Class("masthead"),
				h.Div(h.Span(h.Class("eyebrow"), h.T("Trick taking, one rule at a time")), h.H1(h.T("Trick Evolution"))),
			),
			h.Div(h.Class("top-controls"),
				h.Button(h.Id("rules-open"), h.Type("button"), h.Attr("hidden", ""), h.Attr("aria-haspopup", "dialog"), h.Attr("aria-controls", "rules-review"), h.T("Rules")),
				h.Span(h.Id("connection"), h.Class("connection"), h.T("Offline")),
			),
			h.Div(h.Id("notice"), h.Class("notice"), h.Role("status"), h.Attr("aria-live", "polite")),
			h.Div(h.Id("room-closed"), h.Attr("hidden", "")),
			h.Section(h.Id("table3d"), h.Class("table3d"), h.Attr("hidden", ""), h.Attr("aria-label", "3D card table"),
				h.NewNode("canvas", []*h.Node{h.Id("table3d-canvas"), h.Attr("aria-label", "Your cards on a 3D table")}),
				h.Div(h.Id("table3d-drop-hint"), h.Class("table3d-drop-hint"), h.T("Release here to play")),
				h.Div(h.Id("table3d-seats"), h.Class("table3d-seats")),
			),
			h.NewNode("dialog", []*h.Node{h.Id("rule-details"), h.Attr("aria-labelledby", "rule-title"),
				h.H2(h.Id("rule-title")),
				h.P(h.Id("rule-text")),
				h.Form(h.Attr("method", "dialog"), h.Button(h.Type("submit"), h.T("Close"))),
			}),
			h.Div(h.Id("live"), h.Attr("hx-ext", "ws"),
				joinScreen(room),
				h.NewNode("dialog", []*h.Node{h.Id("rules-review"), h.Attr("aria-labelledby", "rules-review-title"), h.Attr("aria-modal", "true"),
					h.Div(h.Class("review-frame"),
						h.Header(h.Class("review-header"), h.H2(h.Id("rules-review-title"), h.T("Game Rules")),
							h.Button(h.Type("button"), h.Attr("data-action", "review-close"), h.Attr("aria-label", "Close Game Rules"), h.T("Close"))),
						h.Div(h.Id("rules-review-content"), h.Class("review-content"), h.P(h.T("Loading rules…"))),
					),
				}),
			),
		)),
	)
}

func joinScreen(room string) *h.Node {
	options := make([]*h.Node, 4)
	for i := range options {
		options[i] = h.Option(h.Value(fmt.Sprint(i)), h.T(fmt.Sprint(i)))
	}
	return h.Section(h.Id("screen"), h.Attr("aria-live", "polite"),
		h.Section(h.Class("panel join"),
			h.H2(h.T("Endless Mode")),
			h.P(h.Class("muted"), h.T("Start a private run with one bot, three cards per player, and rules that change between rounds.")),
			h.Form(h.Id("endless-form"), h.Class("form-grid"),
				h.Label(h.T("Your name"), h.Input(h.Name("name"), h.Attr("required", ""), h.Attr("maxlength", "32"))),
				h.Button(h.Type("submit"), h.T("Start Endless Mode")))),
		h.Section(h.Class("panel join"),
			h.H2(h.T("Join a room")),
			h.P(h.Class("muted"), h.T("Share the room code with friends, or add computer opponents when creating a room. The first player hosts.")),
			h.Form(h.Id("join-form"), h.Class("form-grid"),
				h.Label(h.T("Room code"), h.Input(h.Name("room"), h.Attr("required", ""), h.Attr("maxlength", "40"), h.Attr("pattern", "[A-Za-z0-9_-]+"), h.Value(room))),
				h.Label(h.T("Your name"), h.Input(h.Name("name"), h.Attr("required", ""), h.Attr("maxlength", "32"))),
				h.Label(h.T("Computer opponents (new room)"), h.Select(append([]*h.Node{h.Name("bots")}, options...)...)),
				h.Button(h.Type("submit"), h.T("Join room")),
			),
		),
	)
}

func commandForm(kind, label string, card domain.CardID, disabled bool, class string, revision uint64) *h.Node {
	children := []*h.Node{
		h.Attr("ws-send", ""),
		h.Input(h.Type("hidden"), h.Name("type"), h.Value(kind)),
		h.Input(h.Type("hidden"), h.Name("revision"), h.Value(fmt.Sprint(revision))),
	}
	if card != "" {
		children = append(children, h.Input(h.Type("hidden"), h.Name("card"), h.Value(string(card))))
	}
	button := []*h.Node{h.Type("submit"), h.T(label)}
	if class != "" {
		button = append(button, h.Class(class))
	}
	if disabled {
		button = append(button, h.Attr("disabled", ""))
	}
	children = append(children, h.Button(button...))
	return h.Form(children...)
}

func draftForm(rule domain.RuleID, label, boundary string, revision uint64) *h.Node {
	return h.Form(h.Attr("ws-send", ""), h.Input(h.Type("hidden"), h.Name("type"), h.Value("choose_rule")),
		h.Input(h.Type("hidden"), h.Name("revision"), h.Value(fmt.Sprint(revision))),
		h.Input(h.Type("hidden"), h.Name("boundaryId"), h.Value(boundary)),
		h.Input(h.Type("hidden"), h.Name("rule"), h.Value(string(rule))),
		h.Button(h.Type("submit"), h.T(label)))
}

func renderScreen(message serverMessage, seat domain.SeatID, catalog domain.Catalog, config domain.Config) string {
	if message.View != nil && message.View.Public.Mode == domain.ModeEndless {
		catalog.Definitions = append([]domain.RuleDefinition(nil), catalog.Definitions...)
		for i := range catalog.Definitions {
			switch catalog.Definitions[i].ID {
			case domain.RuleHandLimit:
				catalog.Definitions[i].Title = "Open Run"
				catalog.Definitions[i].Text = "The run continues across hands and rounds until you end it or the room expires. Completed hand points remain in your run score."
			case domain.RuleReveal:
				catalog.Definitions[i].Title = "Round Pacing"
				catalog.Definitions[i].Text = "Rounds contain complete hands. A round boundary may offer a draft, change a rule automatically, or keep the current field."
			case domain.RuleVictory:
				catalog.Definitions[i].Title = "Run Score"
				catalog.Definitions[i].Text = "Bank points from completed tricks and hands. Scores continue across rounds until the run ends."
			}
		}
	}
	var content *h.Node
	if message.Type == "lobby" {
		content = lobbyPanel(message, seat)
	} else {
		content = gamePanel(message, seat, catalog)
	}
	screen := h.Section(h.Id("screen"), h.Attr("hx-swap-oob", "outerHTML"),
		h.Attr("data-update", fmt.Sprint(message.Sequence)),
		h.Attr("data-revision", fmt.Sprint(message.Revision)),
		h.Attr("data-kind", message.Type),
		h.Attr("aria-live", "polite"), content)
	if message.View != nil {
		if payload, err := cardtable.Build(*message.View, catalog, message.Events...); err == nil {
			for i := range payload.Seats {
				for _, summary := range message.Seats {
					if payload.Seats[i].ID == summary.ID {
						payload.Seats[i].Name = summary.Name
						break
					}
				}
			}
			if data, err := json.Marshal(payload); err == nil {
				screen = h.Section(h.Id("screen"), h.Attr("hx-swap-oob", "outerHTML"),
					h.Attr("data-update", fmt.Sprint(message.Sequence)),
					h.Attr("data-revision", fmt.Sprint(message.Revision)),
					h.Attr("data-kind", message.Type),
					h.Attr("data-scene", base64.StdEncoding.EncodeToString(data)),
					h.Attr("aria-live", "polite"), content)
			}
		}
	}
	review, err := rulesReviewContent(message, seat, catalog, config)
	if err != nil {
		return screen.Render() + noticeNode("Rules unavailable: "+err.Error()).Render()
	}
	return screen.Render() + review.Render() + noticeNode("").Render()
}

func renderClosure(message serverMessage) string {
	marker := h.Div(h.Id("room-closed"), h.Attr("hidden", ""), h.Attr("data-terminal", "true"), h.Attr("data-message", message.Message), h.Attr("hx-swap-oob", "outerHTML"))
	if message.Code != "lobby" {
		return marker.Render()
	}
	screen := h.Section(h.Id("screen"), h.Attr("hx-swap-oob", "outerHTML"), h.Attr("data-kind", "closed"),
		h.Section(h.Class("panel"), h.H2(h.T("Room closed")), h.P(h.T(message.Message)),
			h.Button(h.Type("button"), h.Attr("data-action", "return-to-join"), h.T("Return to join"))))
	return marker.Render() + screen.Render()
}

func lobbyPanel(message serverMessage, seat domain.SeatID) *h.Node {
	tiles := make([]*h.Node, 0, len(message.Seats))
	for _, p := range message.Seats {
		status := "Offline"
		if p.Bot {
			status = "Bot"
		} else if p.Online {
			status = "Online"
		}
		if p.Host {
			status = "Host · " + status
		}
		tiles = append(tiles, h.Div(h.Class("seat"), h.NewNode("strong", []*h.Node{h.T(p.Name)}), h.Span(h.Class("tag"), h.T(status))))
	}
	controls := []*h.Node{}
	if len(message.Seats) > 0 && message.Seats[0].ID == seat {
		if len(message.Seats) < 4 {
			controls = append(controls, commandForm("add_bot", "Add bot", "", false, "", 0))
		}
		start := []*h.Node{h.Type("button"), h.Attr("data-action", "review-start"), h.Attr("aria-haspopup", "dialog"), h.Attr("aria-controls", "rules-review"), h.T("Start game")}
		if len(message.Seats) < 2 {
			start = append(start, h.Attr("disabled", ""))
		}
		controls = append(controls, h.Button(start...))
	} else {
		controls = append(controls, h.P(h.Class("muted"), h.T("Waiting for the host to start.")))
	}
	controls = append(controls, h.Button(h.Type("button"), h.Attr("data-action", "leave-room"), h.T("Leave room")))
	return h.Section(h.Class("panel"),
		h.H2(h.T("Room "+message.Room)),
		h.P(h.Class("muted"), h.T("Invite friends with this page URL or add bots. Two to four seats can play.")),
		h.Div(append([]*h.Node{h.Class("scoreboard")}, tiles...)...),
		h.Div(append([]*h.Node{h.Class("actions")}, controls...)...),
	)
}

func gamePanel(message serverMessage, seat domain.SeatID, catalog domain.Catalog) *h.Node {
	view := message.View
	public := view.Public
	host := false
	for _, p := range message.Seats {
		if p.ID == seat {
			host = p.Host
		}
	}
	names := make(map[domain.SeatID]string, len(message.Seats))
	for _, p := range message.Seats {
		names[p.ID] = p.Name
	}
	name := func(id domain.SeatID) string {
		if names[id] != "" {
			return names[id]
		}
		return string(id)
	}
	rules := renderRuleField(public, catalog)
	next := ""
	turnText := ""
	localTurn, botTurn := false, false
	switch public.Phase {
	case domain.PhasePlayingTrick:
		next = name(public.Turn) + " to play"
		if public.Turn == seat {
			next = "Your turn"
			turnText, localTurn = "Your turn · "+name(seat), true
		} else {
			turnText = name(public.Turn) + "'s turn"
			for _, player := range message.Seats {
				if player.ID == public.Turn && player.Bot {
					botTurn = true
					turnText = name(public.Turn) + " (bot) is playing"
					break
				}
			}
		}
	case domain.PhaseHandComplete:
		next = "Waiting for the host to deal"
		turnText = "Hand complete · waiting for " + name(public.Seats[0].ID) + " to deal"
		if host {
			next = "Deal the next hand"
			turnText = "Hand complete · deal the next hand"
		}
	case domain.PhaseAwaitingChoice:
		next, turnText = "Choose a rule", "Round complete · your rule choice"
	case domain.PhaseGameComplete:
		next = "Game complete"
		turnText = "Game complete"
		if public.EndedEarly {
			turnText = "Game ended"
		}
	}
	hudDetails := ""
	if public.DealSize > 0 && (public.DealSize != public.HandSize || public.TricksPerHand != public.DealSize) {
		hudDetails = fmt.Sprintf("%d cards · %d played tricks", public.DealSize, public.TricksPerHand)
	}
	if public.Mode == domain.ModeEndless && public.Phase == domain.PhasePlayingTrick {
		hudDetails = fmt.Sprintf("Hand countdown: %d trick(s) left · Trick %d/%d", public.TricksPerHand-public.TrickNumber+1, public.TrickNumber, public.TricksPerHand)
	}
	if public.Phase == domain.PhasePlayingTrick && view.Carried != "" {
		for _, id := range view.Hand {
			if id == view.Carried {
				if hudDetails != "" {
					hudDetails += " · "
				}
				hudDetails += "Your carried card: " + cardLabel(id)
				break
			}
		}
	}
	hud := []*h.Node{h.Class("game-hud"),
		h.Div(h.Class("turn-indicator"), h.Attr("data-turn", string(public.Turn)), h.Attr("data-phase", string(public.Phase)), h.Attr("data-local", fmt.Sprint(localTurn)), h.Attr("data-bot", fmt.Sprint(botTurn)), h.Role("status"), h.T(turnText)),
		h.Div(h.Class("match-cue"), h.Role("status"), h.Attr("aria-atomic", "true"),
			h.Span(h.Class("match-event"), h.T(eventNotice(message.Events, catalog, name))),
			h.NewNode("strong", []*h.Node{h.Class("match-next"), h.T(next)})),
		h.P(h.Class("hand-plan"), h.T(hudDetails))}
	if public.Mode == domain.ModeEndless {
		countdown := public.RoundLength - public.RoundHand
		if countdown < 0 {
			countdown = 0
		}
		cue := public.TableCue
		if public.Phase == domain.PhasePlayingTrick {
			_, event := domain.EndlessRound(public.Round + 1)
			action := map[string]string{"draft": "Your rule draft", "table": "The table changes a rule", "hold": "The rules hold"}[event]
			cue += fmt.Sprintf(" %s in %d hand(s).", action, countdown+1)
		}
		handInRound := public.RoundHand
		if handInRound == 0 {
			handInRound = 1
		}
		scores := []*h.Node{h.Class("run-scores"), h.NewNode("strong", []*h.Node{h.T(fmt.Sprintf("Endless Mode · Round %d · Hand %d/%d · Run Score", public.Round, handInRound, public.RoundLength))})}
		ids := make([]domain.SeatID, 0, len(public.Seats)+3)
		seen := make(map[domain.SeatID]bool)
		for _, player := range public.Seats {
			ids = append(ids, player.ID)
			seen[player.ID] = true
		}
		for _, id := range []domain.SeatID{"bot:1", "bot:2", "bot:3"} {
			if !seen[id] {
				ids = append(ids, id)
			}
		}
		for _, id := range ids {
			if score, ok := public.RunScores[id]; ok {
				label := name(id)
				if strings.HasPrefix(string(id), "bot:") && label == string(id) {
					label = "Bot " + strings.TrimPrefix(string(id), "bot:")
				}
				scores = append(scores, h.Span(h.T(fmt.Sprintf("%s: %d", label, score))))
			}
		}
		hud = append(hud, h.Div(scores...))
		if public.ShortGameRound > 0 {
			gameScores := []*h.Node{h.Class("run-scores short-game-scores"), h.NewNode("strong", []*h.Node{h.T(fmt.Sprintf("Three-round game %d · Round %d/3 · Game Score", public.ShortGameNumber, public.ShortGameRound))})}
			for _, player := range public.Seats {
				gameScores = append(gameScores, h.Span(h.T(fmt.Sprintf("%s: %d", name(player.ID), public.ShortGameScores[player.ID]))))
			}
			hud = append(hud, h.Div(gameScores...))
		}
		if result := public.ShortGameResult; result != nil {
			winners := make([]string, 0, len(result.Winners))
			for _, id := range result.Winners {
				winners = append(winners, name(id))
			}
			label := "Winner: "
			if len(winners) > 1 {
				label = "Tie: "
			}
			resultScores := make([]string, 0, len(result.Scores))
			for _, id := range []domain.SeatID{public.Seats[0].ID, "bot:1", "bot:2", "bot:3"} {
				if score, ok := result.Scores[id]; ok {
					playerName := name(id)
					if strings.HasPrefix(string(id), "bot:") && playerName == string(id) {
						playerName = "Bot " + strings.TrimPrefix(string(id), "bot:")
					}
					resultScores = append(resultScores, fmt.Sprintf("%s %d", playerName, score))
				}
			}
			hud = append(hud, h.Div(h.Class("short-game-result"), h.Role("status"), h.NewNode("strong", []*h.Node{h.T(fmt.Sprintf("Three-round game %d complete · %s%s", result.Number, label, strings.Join(winners, " and ")))}), h.P(h.T(strings.Join(resultScores, " · ")))))
		}
		hud = append(hud, h.P(h.Class("round-cue"), h.T(cue)))
	}
	hud = append(hud, rules)
	parts := []*h.Node{h.Class("game-panel"), h.Attr("data-hand", fmt.Sprint(public.HandNumber)), h.Div(hud...)}
	if public.Phase == domain.PhasePlayingTrick {
		legal, _ := domain.LegalCards(*view, catalog)
		allowed := make(map[domain.CardID]bool, len(legal))
		for _, id := range legal {
			allowed[id] = true
		}
		cards := make([]*h.Node, 0, len(view.Hand))
		for _, id := range view.Hand {
			cards = append(cards, commandForm("play", cardLabel(id), id, !allowed[id], cardClass(id), public.Revision))
		}
		parts = append(parts, h.Div(append([]*h.Node{h.Class("play-commands"), h.Attr("hidden", "")}, cards...)...))
	} else if public.Phase == domain.PhaseHandComplete {
		handStatus := []*h.Node{h.Class("status hand-status"), h.P(h.T("Hand complete"))}
		if view.Retained != "" && public.NextRosterSize == 0 {
			handStatus = append(handStatus, h.P(h.T("Your unplayed "+cardLabel(view.Retained)+" stays in your next hand. Each player receives one fewer new card.")))
		}
		if public.NextRosterSize > 0 {
			handStatus = append(handStatus, h.P(h.T(fmt.Sprintf("Next round: %d players. Everyone gets a fresh deal; carried cards reset.", public.NextRosterSize))))
		}
		if resolved, err := domain.EffectiveForView(public, catalog); err == nil {
			cfg := domain.Config{HandSize: public.HandSize, RevealEvery: public.RevealEvery, MaxHands: public.MaxHands}
			if plan, err := domain.HandPlanForRules(cfg, len(public.Seats), resolved); err == nil {
				handStatus = append(handStatus, h.P(h.T(fmt.Sprintf("Next hand: %d cards per player, %d played tricks.", plan.DealSize, plan.Tricks))))
			}
		}
		parts = append(parts, h.Div(handStatus...))
		if host {
			parts = append(parts, h.Div(h.Class("next-hand"), commandForm("next", "Deal next hand", "", false, "", public.Revision)))
		}
	} else if public.Phase == domain.PhaseAwaitingChoice {
		choices := []*h.Node{h.Class("draft-choices"), h.H2(h.T("Choose a rule for the next round"))}
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
		parts = append(parts, h.Div(choices...))
	} else if public.Phase == domain.PhaseGameComplete {
		status := "Game complete"
		if public.EndedEarly {
			status = "Game ended by " + name(public.EndedBy)
		}
		if public.Mode == domain.ModeEndless {
			status = "Run ended"
		}
		winners := make([]string, 0, len(public.Winners))
		for _, id := range public.Winners {
			winners = append(winners, name(id))
		}
		label := "Winner: "
		if len(winners) > 1 {
			label = "Winners: "
		}
		if public.Mode == domain.ModeEndless {
			label = "Top score: "
		}
		results := []*h.Node{
			h.Class("panel game-results"),
			h.P(h.Class("status"), h.T(status)),
			h.H2(h.T(label + strings.Join(winners, " and "))),
		}
		if view.Retained != "" {
			results = append(results, h.P(h.T("Your final unplayed "+cardLabel(view.Retained)+" was not captured or scored as a capture.")))
		}
		if public.EndedEarly {
			results = append(results, h.P(h.Class("muted"), h.T("Scores include completed tricks and settled hands. An unfinished trick and its hand bonus do not score.")))
		}
		results = append(results, h.Button(h.Type("button"), h.Attr("data-action", "return-to-join"), h.T("Return to join")))
		parts = append(parts, h.Div(results...))
	}
	if host && public.Phase != domain.PhaseGameComplete {
		confirmText, confirmLabel := "End this game for everyone? Completed-trick scores remain; an unfinished trick and hand bonus will not score.", "Confirm end game"
		if public.Mode == domain.ModeEndless {
			confirmText, confirmLabel = "End this run? Completed scores remain; an unfinished trick and hand bonus will not score.", "Confirm end run"
		}
		parts = append(parts, h.Div(h.Class("end-game"),
			h.Button(h.Type("button"), h.Attr("data-action", "end-confirm"), h.T(map[bool]string{true: "End run", false: "End game"}[public.Mode == domain.ModeEndless])),
			h.Div(h.Id("end-confirm"), h.Attr("hidden", ""), h.Class("end-confirm"),
				h.Span(h.Class("muted"), h.T(confirmText)),
				commandForm("end", confirmLabel, "", false, "", public.Revision),
				h.Button(h.Type("button"), h.Attr("data-action", "end-cancel"), h.T("Cancel")),
			),
		))
	}
	if public.Phase != domain.PhaseGameComplete && public.Mode != domain.ModeEndless {
		parts = append(parts, h.Div(h.Class("leave-room"),
			h.Button(h.Type("button"), h.Attr("data-action", "leave-prompt"), h.T("Leave room")),
			h.Div(h.Id("leave-confirm"), h.Attr("hidden", ""),
				h.P(h.T("Leaving ends this match for everyone. Completed scores will be final.")),
				h.Button(h.Type("button"), h.Attr("data-action", "leave-room"), h.T("End match and leave")),
				h.Button(h.Type("button"), h.Attr("data-action", "leave-cancel"), h.T("Cancel")))))
	}
	if public.Phase == domain.PhaseHandComplete || public.Phase == domain.PhaseAwaitingChoice || public.Phase == domain.PhaseGameComplete {
		title := "Hand score breakdown"
		if public.EndedEarly && public.CompletedHands < public.HandNumber {
			title = "Completed trick score breakdown"
		}
		items := []*h.Node{h.Class("hand-awards"), h.NewNode("summary", []*h.Node{h.T(title)})}
		totals := map[domain.SeatID]int{}
		for _, award := range public.HandAwards {
			totals[award.Seat] += award.Points
			detail := fmt.Sprintf("%s · %s %+d", award.Title, name(award.Seat), award.Points)
			if award.Card != "" {
				detail += " · " + cardLabel(award.Card)
			} else if award.HiddenCard {
				detail += " · retained card hidden"
			}
			if award.Trick > 0 {
				detail += fmt.Sprintf(" · trick %d", award.Trick)
			}
			if award.Rule == domain.RuleGamePoint {
				detail += fmt.Sprintf(" · tally %d", award.Tally)
			}
			items = append(items, h.P(h.T(detail)))
		}
		for _, player := range public.Seats {
			items = append(items, h.P(h.T(fmt.Sprintf("%s · hand total %+d", name(player.ID), totals[player.ID]))))
		}
		parts = append(parts, h.NewNode("details", items))
	}
	return h.Section(parts...)
}

func cardLabel(id domain.CardID) string {
	card, ok := domain.CardDetails(id)
	if !ok {
		return "?"
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
	suit := map[domain.Suit]string{domain.Hearts: "♥", domain.Diamonds: "♦", domain.Clubs: "♣", domain.Spades: "♠"}[card.Suit]
	return rank + suit
}

func cardClass(id domain.CardID) string {
	class := "card"
	if card, ok := domain.CardDetails(id); ok && (card.Suit == domain.Hearts || card.Suit == domain.Diamonds) {
		class += " red"
	}
	return class
}

func eventNotice(events []domain.Event, catalog domain.Catalog, name func(domain.SeatID) string) string {
	var outcomes []string
	var play string
	for _, event := range events {
		switch event.Kind {
		case domain.EventHandStarted:
			outcomes = append(outcomes, fmt.Sprintf("Hand %d dealt.", event.Hand))
		case domain.EventCardPlayed:
			play = fmt.Sprintf("%s played %s.", name(event.Seat), cardLabel(event.Card))
			if event.TrumpSuit != "" {
				play += " Trump: " + suitName(event.TrumpSuit) + " chosen for this hand."
			}
			if event.BrokeSpades {
				play += " Spades are broken."
			}
			if event.BrokeHearts {
				play += " Hearts are broken."
			}
		case domain.EventTrickWon:
			outcomes = append(outcomes, fmt.Sprintf("%s won trick %d.", name(event.Seat), event.Trick))
		case domain.EventRuleRevealed:
			if definition, ok := catalog.Definition(event.Rule); ok {
				outcomes = append(outcomes, fmt.Sprintf("%s revealed after hand %d; it applies from hand %d.", definition.Title, event.Hand, event.Hand+1))
			}
		}
	}
	if len(outcomes) == 0 {
		return play
	}
	if play != "" {
		outcomes = append([]string{play}, outcomes...)
	}
	return strings.Join(outcomes, " ")
}

func noticeNode(message string) *h.Node {
	return h.Div(h.Id("notice"), h.Class("notice"), h.Role("status"), h.Attr("aria-live", "polite"), h.Attr("hx-swap-oob", "outerHTML"), h.T(message))
}

func serveGamePage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	room := r.URL.Query().Get("room")
	if room == "" {
		room = "default"
	}
	_, _ = w.Write([]byte("<!doctype html>" + pageNode(room).Render()))
}
