package gameui

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/n0remac/trick-evolution/domain"
)

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
		Room  string        `json:"room"`
		Seat  domain.SeatID `json:"seat"`
		Name  string        `json:"name"`
		Token string        `json:"token"`
		Host  bool          `json:"host"`
		RunID string        `json:"runId"`
	}{Room: roomID, Seat: domain.SeatID(seatID), Name: input.Name, Token: token, Host: true, RunID: roomID})
}
