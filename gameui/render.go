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
				h.Span(h.Id("connection"), h.Class("connection"), h.T("Offline")),
			),
			h.Div(h.Id("notice"), h.Class("notice"), h.Role("status"), h.Attr("aria-live", "polite")),
			h.Section(h.Id("table3d"), h.Class("table3d"), h.Attr("hidden", ""), h.Attr("aria-label", "3D card table"),
				h.NewNode("canvas", []*h.Node{h.Id("table3d-canvas"), h.Attr("aria-label", "Your cards on a 3D table")}),
				h.Div(h.Id("table3d-drop-hint"), h.Class("table3d-drop-hint"), h.T("Release here to play")),
				h.Div(h.Class("table3d-controls"),
					h.Span(h.Id("table3d-selection"), h.Role("status"), h.Attr("aria-live", "polite"), h.T("Select a card on the table.")),
					h.Button(h.Id("table3d-play"), h.Type("button"), h.Attr("disabled", ""), h.T("Play selected card")),
				),
			),
			h.Div(h.Id("live"), h.Attr("hx-ext", "ws"),
				joinScreen(room),
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

func renderScreen(message serverMessage, seat domain.SeatID, catalog domain.Catalog) string {
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
	return screen.Render() + noticeNode(eventNotice(message)).Render()
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
		controls = append(controls, commandForm("start", "Start game", "", len(message.Seats) < 2, "", 0))
	} else {
		controls = append(controls, h.P(h.Class("muted"), h.T("Waiting for the host to start.")))
	}
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
	status := "Game complete"
	switch public.Phase {
	case domain.PhasePlayingTrick:
		status = fmt.Sprintf("%s to play · Trick %d", name(public.Turn), public.TrickNumber)
	case domain.PhaseHandComplete:
		status = "Hand complete"
	case domain.PhaseGameComplete:
		if public.EndedEarly {
			status = "Game ended by " + name(public.EndedBy)
		}
	}
	seats := make([]*h.Node, 0, len(public.Seats))
	for _, p := range public.Seats {
		class := "seat"
		if p.ID == public.Turn {
			class += " current"
		}
		seats = append(seats, h.Div(h.Class(class),
			h.NewNode("strong", []*h.Node{h.T(name(p.ID))}),
			h.Span(h.T(fmt.Sprintf("%d points · %d tricks · %d cards", p.Score, p.TricksWon, p.HandCount))),
		))
	}
	rules := make([]*h.Node, 0, len(public.Rules))
	for _, resolved := range public.Rules {
		definition, ok := catalog.Definition(resolved.Instance.DefinitionID)
		title := string(resolved.Instance.DefinitionID)
		if ok {
			title = definition.Title
		}
		class := "rule"
		if !resolved.Active {
			class += " inactive"
			title += " · overridden"
		}
		label := title
		if ok {
			label += ": " + definition.Text
		}
		rules = append(rules, h.NewNode("details", []*h.Node{h.Class(class), h.NewNode("summary", []*h.Node{h.T(title)}), h.P(h.T(label))}))
	}
	plays := make([]*h.Node, 0, len(public.CurrentTrick))
	if len(public.CurrentTrick) == 0 {
		plays = append(plays, h.Span(h.Class("muted"), h.T("The table is clear.")))
	}
	for _, play := range public.CurrentTrick {
		plays = append(plays, h.Div(h.Class(cardClass(play.Card)), h.T(cardLabel(play.Card)), h.Small(h.T(name(play.Seat)))))
	}
	parts := []*h.Node{
		h.Class("panel game-panel"),
		h.Div(h.Class("row"), h.H2(h.T(fmt.Sprintf("Room %s · Hand %d of %d", message.Room, public.HandNumber, public.MaxHands))), h.Span(h.Class("tag"), h.T(fmt.Sprintf("Revision %d", public.Revision)))),
		h.P(h.Class("status"), h.T(status)),
		h.Div(append([]*h.Node{h.Class("scoreboard")}, seats...)...),
		h.Div(h.Class("panel"), h.H2(h.T("Rules in play")), h.Div(append([]*h.Node{h.Class("rules")}, rules...)...), h.P(h.Class("muted"), h.T(fmt.Sprintf("%d rule cards left · reveal every %d completed hands", public.RulesRemaining, public.RevealEvery)))),
		h.Div(append([]*h.Node{h.Class("table text-table")}, plays...)...),
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
		parts = append(parts, h.NewNode("details", []*h.Node{h.Class("hand-fallback"), h.NewNode("summary", []*h.Node{h.T("Text hand and keyboard controls")}), h.H2(h.T("Your hand")), h.Div(append([]*h.Node{h.Class("hand")}, cards...)...)}))
		if seat == public.Turn && len(legal) < len(view.Hand) {
			parts = append(parts, h.P(h.Class("muted"), h.T("Follow suit: play a card matching the lead suit.")))
		}
	} else if public.Phase == domain.PhaseHandComplete && len(message.Seats) > 0 && message.Seats[0].ID == seat {
		parts = append(parts, commandForm("next", "Deal next hand", "", false, "", public.Revision))
	} else if public.Phase == domain.PhaseGameComplete {
		winners := make([]string, 0, len(public.Winners))
		for _, id := range public.Winners {
			winners = append(winners, name(id))
		}
		label := "Winner: "
		if len(winners) > 1 {
			label = "Winners: "
		}
		parts = append(parts, h.H2(h.T(label+strings.Join(winners, " and "))))
		if public.EndedEarly {
			parts = append(parts, h.P(h.Class("muted"), h.T("Scores include completed tricks only.")))
		}
		parts = append(parts, h.Button(h.Type("button"), h.Attr("data-action", "leave"), h.T("Return to join")))
	}
	if len(message.Seats) > 0 && message.Seats[0].ID == seat && public.Phase != domain.PhaseGameComplete {
		parts = append(parts, h.Div(h.Class("row"),
			h.Button(h.Type("button"), h.Attr("data-action", "end-confirm"), h.T("End game")),
			h.Div(h.Id("end-confirm"), h.Attr("hidden", ""), h.Class("row"),
				h.Span(h.Class("muted"), h.T("End this game for everyone? An unfinished trick will not score.")),
				commandForm("end", "Confirm end game", "", false, "", public.Revision),
				h.Button(h.Type("button"), h.Attr("data-action", "end-cancel"), h.T("Cancel")),
			),
		))
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

func eventNotice(message serverMessage) string {
	names := map[domain.SeatID]string{}
	for _, p := range message.Seats {
		names[p.ID] = p.Name
	}
	for i := len(message.Events) - 1; i >= 0; i-- {
		event := message.Events[i]
		if event.Kind == domain.EventRuleRevealed {
			definition, ok := domain.DefaultCatalog().Definition(event.Rule)
			if ok {
				return fmt.Sprintf("%s revealed after hand %d.", definition.Title, event.Hand)
			}
		}
		if event.Kind == domain.EventTrickWon {
			return fmt.Sprintf("%s won trick %d.", names[event.Seat], event.Trick)
		}
	}
	return ""
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
