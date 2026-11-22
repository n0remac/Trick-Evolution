package gameui

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
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
	scoreName := func(id domain.SeatID) string {
		if label := message.SeatNames[id]; label != "" {
			return label
		}
		if strings.HasPrefix(string(id), "bot:") {
			return "Bot " + strings.TrimPrefix(string(id), "bot:")
		}
		return name(id)
	}
	rules := renderRuleField(public, catalog)
	next := ""
	switch public.Phase {
	case domain.PhasePlayingTrick:
		if public.Turn == seat {
			next = "Your turn"
		} else {
			next = name(public.Turn) + " to play"
		}
	case domain.PhaseHandComplete:
		next = "Dealing next hand soon"
		if message.ReviewRule != "" {
			next = "Waiting for the host to continue"
			if host {
				next = "Continue to the next hand"
			}
		}
	case domain.PhaseAwaitingChoice:
		next = "Choose a rule"
	case domain.PhaseGameComplete:
		next = "Game complete"
	}
	if public.Mode == domain.ModeEndless && public.Phase == domain.PhasePlayingTrick {
		countdown := public.RoundLength - public.RoundHand + 1
		if countdown < 1 {
			countdown = 1
		}
		_, event := domain.EndlessRound(public.Round + 1)
		action := map[string]string{"draft": "rule draft", "table": "table rule change", "hold": "rules hold"}[event]
		next += fmt.Sprintf(" · %s in %d hand(s)", action, countdown)
	}
	result := eventNotice(message.Events, catalog, name)
	if public.Mode == domain.ModeEndless && public.TableCue != "" {
		showTableCue := public.Phase == domain.PhaseAwaitingChoice ||
			(public.Phase == domain.PhaseHandComplete && public.RoundHand == 0)
		if public.Phase == domain.PhasePlayingTrick && public.RoundHand == 1 {
			for _, event := range message.Events {
				if event.Kind == domain.EventHandStarted {
					showTableCue = true
					break
				}
			}
		}
		if showTableCue {
			result = public.TableCue
		}
	}
	shortResult, shortResultNumber := "", ""
	if latest := public.ShortGameResult; latest != nil {
		winners := make([]string, 0, len(latest.Winners))
		for _, id := range latest.Winners {
			winners = append(winners, name(id))
		}
		outcome := "Winner: "
		if len(winners) > 1 {
			outcome = "Tie: "
		}
		shortResult = fmt.Sprintf("Three-round game %d complete · %s%s", latest.Number, outcome, strings.Join(winners, " and "))
		shortResultNumber = fmt.Sprint(latest.Number)
	}
	fallbackSeats := []*h.Node{h.Class("fallback-seats")}
	for _, player := range public.Seats {
		score := player.Score
		label := "score"
		if public.Mode == domain.ModeEndless && public.ShortGameRound > 0 {
			score = public.ShortGameScores[player.ID]
			label = "game score"
		}
		turn := ""
		if public.Phase == domain.PhasePlayingTrick && public.Turn == player.ID {
			turn = " · turn"
		}
		fallbackSeats = append(fallbackSeats, h.Span(h.T(fmt.Sprintf("%s · %s %d%s", name(player.ID), label, score, turn))))
	}
	hud := []*h.Node{h.Class("game-hud"), rules,
		h.Div(h.Class("match-cue"), h.Role("status"), h.Attr("aria-atomic", "true"),
			h.Attr("data-next", next), h.Attr("data-event", result), h.Attr("data-result", shortResult), h.Attr("data-result-number", shortResultNumber), h.T(next)),
		renderScoresControl(public, name, scoreName), h.Div(fallbackSeats...)}
	parts := []*h.Node{h.Class("game-panel"), h.Attr("data-hand", fmt.Sprint(public.HandNumber)), h.Div(hud...)}
	if public.Mode == domain.ModeEndless && public.Phase != domain.PhaseGameComplete {
		parts = append(parts, endlessShareControls())
	}
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
		if message.ReviewRule != "" {
			if definition, ok := catalog.Definition(message.ReviewRule); ok {
				card := []*h.Node{h.Class("new-rule-review"), h.NewNode("strong", []*h.Node{h.Class("new-rule-title"), h.T(definition.Title)}), h.P(h.Class("new-rule-text"), h.T(definition.Text))}
				contents := []*h.Node{h.Class("new-rule-overlay"), h.H2(h.T("A new rule is being added")), h.Div(card...)}
				if host {
					contents = append(contents, commandForm("continue_rule", "Continue", "", false, "", public.Revision))
				} else {
					contents = append(contents, h.P(h.T("Waiting for the host to continue.")))
				}
				parts = append(parts, h.Div(contents...))
			}
		}
	} else if public.Phase == domain.PhaseAwaitingChoice {
		parts = append(parts, renderDraftChoices(public, catalog, scoreName))
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
	if card := public.LastScorecard; card != nil && card.HandNumber == public.HandNumber && public.Phase != domain.PhaseAwaitingChoice {
		parts = append(parts, renderScorecard(card, scoreName, "scorecard-visible"))
	}
	return h.Section(parts...)
}

func renderScoresControl(public domain.PublicView, name, scoreName func(domain.SeatID) string) *h.Node {
	content := []*h.Node{h.Class("scores-control"), h.NewNode("summary", []*h.Node{h.T("Scores")})}
	standings := []*h.Node{h.Class("scores-standings"), h.NewNode("strong", []*h.Node{h.T("Standings")})}
	for _, seat := range public.Seats {
		standings = append(standings, h.Span(h.T(fmt.Sprintf("%s · %d", name(seat.ID), seat.Score))))
	}
	content = append(content, h.Div(standings...))
	if public.Mode == domain.ModeEndless {
		displayName := func(id domain.SeatID) string {
			label := name(id)
			if strings.HasPrefix(string(id), "bot:") && label == string(id) {
				return "Bot " + strings.TrimPrefix(string(id), "bot:")
			}
			return label
		}
		run := []*h.Node{h.Class("scores-standings"), h.NewNode("strong", []*h.Node{h.T("Run totals")})}
		ids := make([]domain.SeatID, 0, len(public.RunScores))
		seen := make(map[domain.SeatID]bool, len(public.Seats))
		for _, seat := range public.Seats {
			ids = append(ids, seat.ID)
			seen[seat.ID] = true
		}
		remaining := make([]string, 0, len(public.RunScores))
		for id := range public.RunScores {
			if !seen[id] {
				remaining = append(remaining, string(id))
			}
		}
		sort.Strings(remaining)
		for _, id := range remaining {
			ids = append(ids, domain.SeatID(id))
		}
		for _, id := range ids {
			if score, ok := public.RunScores[id]; ok {
				run = append(run, h.Span(h.T(fmt.Sprintf("%s · %d", displayName(id), score))))
			}
		}
		content = append(content, h.Div(run...))
		if public.ShortGameRound > 0 {
			game := []*h.Node{h.Class("scores-standings"), h.NewNode("strong", []*h.Node{h.T(fmt.Sprintf("Game %d · round %d/3", public.ShortGameNumber, public.ShortGameRound))})}
			for _, seat := range public.Seats {
				game = append(game, h.Span(h.T(fmt.Sprintf("%s · %d", name(seat.ID), public.ShortGameScores[seat.ID]))))
			}
			content = append(content, h.Div(game...))
		}
		if result := public.ShortGameResult; result != nil {
			winners := make([]string, 0, len(result.Winners))
			for _, id := range result.Winners {
				winners = append(winners, name(id))
			}
			outcome := "Winner: "
			if len(winners) > 1 {
				outcome = "Tie: "
			}
			latest := []*h.Node{h.Class("scores-standings"), h.NewNode("strong", []*h.Node{h.T(fmt.Sprintf("Latest game %d · %s%s", result.Number, outcome, strings.Join(winners, " and ")))})}
			for _, id := range ids {
				if score, ok := result.Scores[id]; ok {
					latest = append(latest, h.Span(h.T(fmt.Sprintf("%s · %d", displayName(id), score))))
				}
			}
			content = append(content, h.Div(latest...))
		}
	}
	if public.LastScorecard != nil {
		content = append(content, renderScorecard(public.LastScorecard, scoreName, "scorecard-saved"))
	}
	return h.NewNode("details", content)
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
