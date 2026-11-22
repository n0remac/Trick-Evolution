package gameui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	h "github.com/n0remac/GoDom/html"
	"github.com/n0remac/trick-evolution/domain"
)

func (s *Server) registerEndlessRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /game/join", s.join)
	mux.HandleFunc("POST /game/endless", s.startEndless)
	mux.HandleFunc("POST /game/endless/join", s.joinEndlessInvite)
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

func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var request struct {
		Room, Name, Token string
		Bots              int
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		http.Error(w, "invalid join request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		http.Error(w, "invalid join request", http.StatusBadRequest)
		return
	}
	if !roomPattern.MatchString(request.Room) {
		http.Error(w, "invalid room ID", http.StatusBadRequest)
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || utf8.RuneCountInString(request.Name) > 32 {
		http.Error(w, "name must be 1–32 characters", http.StatusBadRequest)
		return
	}
	if request.Bots < 0 || request.Bots > 3 {
		http.Error(w, "bots must be 0–3", http.StatusBadRequest)
		return
	}
	newID, err := randomToken(8)
	if err != nil {
		http.Error(w, "could not create seat", http.StatusInternalServerError)
		return
	}
	newToken, err := randomToken(32)
	if err != nil {
		http.Error(w, "could not create session", http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	s.pruneLocked()
	if target := s.redirects[request.Room]; request.Token != "" && target != "" {
		request.Room = target
	}
	gameRoom := s.rooms[request.Room]
	if gameRoom == nil {
		if request.Token != "" {
			s.mu.Unlock()
			http.Error(w, "room is closed", http.StatusGone)
			return
		}
		roomContext, cancel := context.WithCancel(context.Background())
		gameRoom = &room{id: request.Room, config: s.config, clients: make(map[domain.SeatID]*client), seen: make(map[domain.SeatID]map[string]acceptedCommand), lastActivity: s.now(), server: s, botWake: make(chan struct{}, 1), done: make(chan struct{}), ctx: roomContext, cancel: cancel}
		s.rooms[request.Room] = gameRoom
	}
	gameRoom.mu.Lock()
	if gameRoom.closed {
		gameRoom.mu.Unlock()
		s.mu.Unlock()
		http.Error(w, "room is closed", http.StatusGone)
		return
	}
	var seat participant
	if request.Token != "" {
		for _, existing := range gameRoom.seats {
			if !existing.Bot && existing.Token == request.Token {
				seat = existing
				break
			}
		}
		if seat.ID == "" {
			gameRoom.mu.Unlock()
			s.mu.Unlock()
			http.Error(w, "invalid session", http.StatusUnauthorized)
			return
		}
		if request.Bots > 0 && gameRoom.seats[0].ID != seat.ID {
			gameRoom.mu.Unlock()
			s.mu.Unlock()
			http.Error(w, "only the host can add bots", http.StatusForbidden)
			return
		}
	} else {
		if request.Bots > 0 && len(gameRoom.seats) != 0 {
			gameRoom.mu.Unlock()
			s.mu.Unlock()
			http.Error(w, "only the host can add bots", http.StatusForbidden)
			return
		}
		if gameRoom.state != nil || len(gameRoom.seats)+1+request.Bots > 4 {
			gameRoom.mu.Unlock()
			s.mu.Unlock()
			http.Error(w, "room is full or already started", http.StatusConflict)
			return
		}
		seat = participant{ID: domain.SeatID(newID), Name: request.Name, Token: newToken}
		gameRoom.seats = append(gameRoom.seats, seat)
	}
	if request.Bots > 0 {
		if gameRoom.state != nil || len(gameRoom.seats)+request.Bots > 4 {
			gameRoom.mu.Unlock()
			s.mu.Unlock()
			http.Error(w, "room is full or already started", http.StatusConflict)
			return
		}
		count := 0
		for _, existing := range gameRoom.seats {
			if existing.Bot {
				count++
			}
		}
		for i := 0; i < request.Bots; i++ {
			count++
			gameRoom.seats = append(gameRoom.seats, participant{ID: domain.SeatID(fmt.Sprintf("bot:%d", count)), Name: fmt.Sprintf("Bot %d", count), Bot: true})
		}
	}
	gameRoom.lastActivity = s.now()
	host := gameRoom.seats[0].ID == seat.ID
	invite := gameRoom.inviteToken
	if gameRoom.config.Mode == domain.ModeEndless && gameRoom.seats[0].Bot {
		host = false
		for _, p := range gameRoom.seats {
			if !p.Bot {
				host = p.ID == seat.ID
				break
			}
		}
	}
	gameRoom.mu.Unlock()
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Room   string        `json:"room"`
		Seat   domain.SeatID `json:"seat"`
		Name   string        `json:"name"`
		Token  string        `json:"token"`
		Host   bool          `json:"host"`
		RunID  string        `json:"runId,omitempty"`
		Invite string        `json:"invite,omitempty"`
	}{Room: request.Room, Seat: seat.ID, Name: seat.Name, Token: seat.Token, Host: host, RunID: seat.RunID, Invite: invite})
	gameRoom.publish()
}

// startEndless makes a private, immediately playable room. Refreshes use the
// ordinary authenticated join path with the returned room and token.
func (s *Server) startEndless(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var input struct {
		Name string `json:"name"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid start request", http.StatusBadRequest)
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 32 {
		http.Error(w, "name must be 1–32 characters", http.StatusBadRequest)
		return
	}
	id, err := randomToken(8)
	if err != nil {
		http.Error(w, "could not create room", 500)
		return
	}
	seatID, err := randomToken(8)
	if err != nil {
		http.Error(w, "could not create seat", 500)
		return
	}
	token, err := randomToken(32)
	if err != nil {
		http.Error(w, "could not create session", 500)
		return
	}
	invite, err := randomToken(32)
	if err != nil {
		http.Error(w, "could not create invitation", 500)
		return
	}
	roomID := "endless-" + id
	config := domain.DefaultConfig()
	config.Mode, config.HandSize = domain.ModeEndless, 3
	state, err := domain.NewMatch([]domain.SeatID{domain.SeatID(seatID), "bot:1"}, config, s.catalog, s.shuffle())
	if err != nil {
		http.Error(w, "could not start run", 500)
		return
	}
	state.Revision = 1
	roomContext, cancel := context.WithCancel(context.Background())
	gameRoom := &room{id: roomID, config: config, seats: []participant{{ID: domain.SeatID(seatID), Name: input.Name, Token: token, RunID: roomID}, {ID: "bot:1", Name: "Bot 1", Bot: true}},
		clients: make(map[domain.SeatID]*client), seen: make(map[domain.SeatID]map[string]acceptedCommand), state: &state,
		lastEvents:   []domain.Event{{Kind: domain.EventHandStarted, Revision: 1, Hand: 1, Seat: state.Leader}},
		lastActivity: s.now(), server: s, botWake: make(chan struct{}, 1), done: make(chan struct{}), ctx: roomContext, cancel: cancel, workerStarted: true}
	gameRoom.inviteToken = invite
	s.mu.Lock()
	s.pruneLocked()
	if _, exists := s.rooms[roomID]; exists {
		s.mu.Unlock()
		cancel()
		http.Error(w, "room ID collision", 500)
		return
	}
	s.rooms[roomID] = gameRoom
	s.mu.Unlock()
	go gameRoom.runBots()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Room   string        `json:"room"`
		Seat   domain.SeatID `json:"seat"`
		Name   string        `json:"name"`
		Token  string        `json:"token"`
		Host   bool          `json:"host"`
		RunID  string        `json:"runId"`
		Invite string        `json:"invite"`
	}{Room: roomID, Seat: domain.SeatID(seatID), Name: input.Name, Token: token, Host: true, RunID: roomID, Invite: invite})
}

// joinEndlessInvite claims an existing bot seat in the invited live table.
// The invite is separate from every player's private reconnect token.
func (s *Server) joinEndlessInvite(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var input struct {
		Room, Invite, Name string
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || !roomPattern.MatchString(input.Room) || len(input.Invite) != 64 {
		http.Error(w, "invalid invitation link", http.StatusBadRequest)
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 32 {
		http.Error(w, "name must be 1–32 characters", http.StatusBadRequest)
		return
	}
	seatID, err := randomToken(8)
	if err != nil {
		http.Error(w, "could not create seat", 500)
		return
	}
	token, err := randomToken(32)
	if err != nil {
		http.Error(w, "could not create session", 500)
		return
	}
	s.mu.Lock()
	gameRoom := s.rooms[input.Room]
	s.mu.Unlock()
	if gameRoom == nil {
		http.Error(w, "invitation expired", http.StatusGone)
		return
	}
	gameRoom.dispatchMu.Lock()
	defer gameRoom.dispatchMu.Unlock()
	s.mu.Lock()
	gameRoom.mu.Lock()
	if s.rooms[input.Room] != gameRoom || gameRoom.closed || gameRoom.state == nil || gameRoom.state.Phase == domain.PhaseGameComplete {
		gameRoom.mu.Unlock()
		s.mu.Unlock()
		http.Error(w, "invitation expired", http.StatusGone)
		return
	}
	if gameRoom.config.Mode != domain.ModeEndless || gameRoom.inviteToken != input.Invite {
		gameRoom.mu.Unlock()
		s.mu.Unlock()
		http.Error(w, "invalid invitation link", http.StatusForbidden)
		return
	}
	botIndex := -1
	for i, seat := range gameRoom.seats {
		if seat.Bot {
			botIndex = i
			break
		}
	}
	if botIndex < 0 {
		gameRoom.mu.Unlock()
		s.mu.Unlock()
		http.Error(w, "table is full", http.StatusConflict)
		return
	}
	oldID := gameRoom.seats[botIndex].ID
	next, err := domain.ReplaceEndlessBotSeat(*gameRoom.state, oldID, domain.SeatID(seatID))
	if err != nil {
		gameRoom.mu.Unlock()
		s.mu.Unlock()
		http.Error(w, "could not join table", http.StatusConflict)
		return
	}
	gameRoom.state = &next
	gameRoom.seats[botIndex] = participant{ID: domain.SeatID(seatID), Name: input.Name, Token: token, RunID: input.Room}
	for i := range gameRoom.lastEvents {
		if gameRoom.lastEvents[i].Seat == oldID {
			gameRoom.lastEvents[i].Seat = domain.SeatID(seatID)
		}
		for j := range gameRoom.lastEvents[i].Plays {
			if gameRoom.lastEvents[i].Plays[j].Seat == oldID {
				gameRoom.lastEvents[i].Plays[j].Seat = domain.SeatID(seatID)
			}
		}
	}
	for i := range gameRoom.publicPlays {
		if gameRoom.publicPlays[i].Seat == oldID {
			gameRoom.publicPlays[i].Seat = domain.SeatID(seatID)
		}
	}
	for i := range gameRoom.publicTricks {
		if gameRoom.publicTricks[i].Winner == oldID {
			gameRoom.publicTricks[i].Winner = domain.SeatID(seatID)
		}
		for j := range gameRoom.publicTricks[i].Plays {
			if gameRoom.publicTricks[i].Plays[j].Seat == oldID {
				gameRoom.publicTricks[i].Plays[j].Seat = domain.SeatID(seatID)
			}
		}
	}
	gameRoom.lastActivity = s.now()
	gameRoom.mu.Unlock()
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Room   string        `json:"room"`
		Seat   domain.SeatID `json:"seat"`
		Name   string        `json:"name"`
		Token  string        `json:"token"`
		Host   bool          `json:"host"`
		RunID  string        `json:"runId"`
		Invite string        `json:"invite"`
	}{input.Room, domain.SeatID(seatID), input.Name, token, botIndex == 0, input.Room, input.Invite})
	gameRoom.publishLocked()
}

func endlessShareControls() *h.Node {
	return h.Div(h.Class("share-game"),
		h.Button(h.Type("button"), h.Attr("data-action", "share-endless"), h.T("Share live game")),
		h.Input(h.Id("endless-share-link"), h.Attr("aria-label", "Live game invitation link"), h.Attr("readonly", ""), h.Attr("hidden", "")))
}
