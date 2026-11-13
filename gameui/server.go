package gameui

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/n0remac/trick-evolution/domain"
)

const roomIdleTTL = 2 * time.Hour

var roomPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
var actionPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

type shuffleFactory func() domain.ShuffleSource

type Server struct {
	mu        sync.Mutex
	rooms     map[string]*room
	catalog   domain.Catalog
	config    domain.Config
	shuffle   shuffleFactory
	now       func() time.Time
	done      chan struct{}
	closeOnce sync.Once
}

type participant struct {
	ID    domain.SeatID
	Name  string
	Token string
}

type room struct {
	mu           sync.Mutex
	dispatchMu   sync.Mutex // Serializes accepted actions and snapshot delivery for this room.
	id           string
	seats        []participant
	clients      map[domain.SeatID]*client
	state        *domain.MatchState
	lastEvents   []domain.Event
	seen         map[domain.SeatID]map[string]acceptedCommand
	lastActivity time.Time
	server       *Server
}

type acceptedCommand struct {
	fingerprint string
	revision    uint64
}

type client struct {
	conn     *websocket.Conn
	send     chan []byte
	done     chan struct{}
	stopOnce sync.Once
	room     *room
	seat     domain.SeatID
}

type seatSummary struct {
	ID     domain.SeatID `json:"id"`
	Name   string        `json:"name"`
	Online bool          `json:"online"`
	Host   bool          `json:"host"`
}

type serverMessage struct {
	Type     string             `json:"type"`
	ID       string             `json:"id,omitempty"`
	Code     string             `json:"code,omitempty"`
	Message  string             `json:"message,omitempty"`
	Revision uint64             `json:"revision"`
	Room     string             `json:"room,omitempty"`
	Seats    []seatSummary      `json:"seats,omitempty"`
	View     *domain.PlayerView `json:"view,omitempty"`
	Events   []domain.Event     `json:"events,omitempty"`
}

type command struct {
	Type string        `json:"type"`
	ID   string        `json:"id"`
	Card domain.CardID `json:"card,omitempty"`
}

// NewServer uses independent cryptographic seeds for each deal. Deck orders are
// passed explicitly to the domain engine and retained in match state.
func NewServer() *Server {
	s, err := NewServerWithConfig(domain.DefaultConfig(), randomShuffle)
	if err != nil {
		panic(err)
	}
	return s
}

func NewServerWithConfig(config domain.Config, shuffle func() domain.ShuffleSource) (*Server, error) {
	if err := config.Validate(2); err != nil {
		return nil, err
	}
	catalog := domain.DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	if shuffle == nil {
		return nil, fmt.Errorf("shuffle factory is required")
	}
	s := &Server{rooms: make(map[string]*room), catalog: catalog, config: config, shuffle: shuffle, now: time.Now, done: make(chan struct{})}
	go s.cleanupLoop()
	return s, nil
}

// Close stops periodic room cleanup. Active connections are closed by their
// HTTP server or clients; production normally keeps Server for process life.
func (s *Server) Close() { s.closeOnce.Do(func() { close(s.done) }) }

func (s *Server) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.mu.Lock()
			s.pruneLocked()
			s.mu.Unlock()
		}
	}
}

func randomShuffle() domain.ShuffleSource {
	var seed [8]byte
	if _, err := rand.Read(seed[:]); err != nil {
		panic(err)
	}
	var value int64
	for _, b := range seed {
		value = value<<8 | int64(b)
	}
	return domain.SeededShuffle(value)
}

func randomToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /game", serveGamePage)
	mux.HandleFunc("GET /game/", serveGamePage)
	mux.HandleFunc("GET /game/game.js", serveGameJS)
	mux.HandleFunc("GET /game/style.css", serveGameCSS)
	mux.HandleFunc("POST /game/join", s.join)
	mux.HandleFunc("GET /ws/game", s.websocket)
	return mux
}

func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var request struct{ Room, Name, Token string }
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
	gameRoom := s.rooms[request.Room]
	if gameRoom == nil {
		gameRoom = &room{id: request.Room, clients: make(map[domain.SeatID]*client), seen: make(map[domain.SeatID]map[string]acceptedCommand), lastActivity: s.now(), server: s}
		s.rooms[request.Room] = gameRoom
	}
	gameRoom.mu.Lock()
	var seat participant
	if request.Token != "" {
		for _, existing := range gameRoom.seats {
			if existing.Token == request.Token {
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
	} else {
		if gameRoom.state != nil || len(gameRoom.seats) >= 4 {
			gameRoom.mu.Unlock()
			s.mu.Unlock()
			http.Error(w, "room is full or already started", http.StatusConflict)
			return
		}
		seat = participant{ID: domain.SeatID(newID), Name: request.Name, Token: newToken}
		gameRoom.seats = append(gameRoom.seats, seat)
	}
	gameRoom.lastActivity = s.now()
	host := gameRoom.seats[0].ID == seat.ID
	gameRoom.mu.Unlock()
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Room  string        `json:"room"`
		Seat  domain.SeatID `json:"seat"`
		Name  string        `json:"name"`
		Token string        `json:"token"`
		Host  bool          `json:"host"`
	}{Room: request.Room, Seat: seat.ID, Name: seat.Name, Token: seat.Token, Host: host})
	gameRoom.publish()
}

func (s *Server) pruneLocked() {
	for id, gameRoom := range s.rooms {
		gameRoom.mu.Lock()
		stale := len(gameRoom.clients) == 0 && s.now().Sub(gameRoom.lastActivity) > roomIdleTTL
		gameRoom.mu.Unlock()
		if stale {
			delete(s.rooms, id)
		}
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize: 1024, WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		} // Non-browser clients still need a valid session token.
		u, err := url.Parse(origin)
		if err != nil {
			return false
		}
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if u.Host != r.Host {
			return false
		}
		if u.Scheme == scheme {
			return true
		}
		// The reverse proxy terminates TLS. Only the exact configured public
		// origin may use HTTPS when the backend request arrives over HTTP.
		return origin == os.Getenv("TRICK_EVOLUTION_PUBLIC_ORIGIN") && origin != ""
	},
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	roomID, token := r.URL.Query().Get("room"), r.URL.Query().Get("token")
	if !roomPattern.MatchString(roomID) || token == "" {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}
	s.mu.Lock()
	gameRoom := s.rooms[roomID]
	s.mu.Unlock()
	if gameRoom == nil {
		http.Error(w, "room not found", http.StatusNotFound)
		return
	}
	gameRoom.mu.Lock()
	var seat domain.SeatID
	for _, p := range gameRoom.seats {
		if p.Token == token {
			seat = p.ID
			break
		}
	}
	gameRoom.mu.Unlock()
	if seat == "" {
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &client{conn: conn, send: make(chan []byte, 16), done: make(chan struct{}), room: gameRoom, seat: seat}
	gameRoom.mu.Lock()
	previous := gameRoom.clients[seat]
	gameRoom.clients[seat] = c
	gameRoom.lastActivity = s.now()
	gameRoom.mu.Unlock()
	if previous != nil {
		previous.stop()
	}
	go c.writePump()
	gameRoom.publish()
	c.readPump()
}

func (c *client) stop() { c.stopOnce.Do(func() { close(c.done); _ = c.conn.Close() }) }

func (c *client) enqueue(message serverMessage) {
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("encode game message: %v", err)
		return
	}
	select {
	case <-c.done:
		return
	case c.send <- data:
	default:
		c.stop() // A slow client can rejoin; it cannot stall the room.
	}
}

func (c *client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	defer c.stop()
	for {
		select {
		case <-c.done:
			return
		case data := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *client) readPump() {
	defer c.stop()
	defer c.room.detach(c)
	c.conn.SetReadLimit(2048)
	_ = c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	c.conn.SetPongHandler(func(string) error { return c.conn.SetReadDeadline(time.Now().Add(90 * time.Second)) })
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var cmd command
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&cmd); err != nil {
			c.enqueue(serverMessage{Type: "error", Code: "bad_command", Message: "invalid command"})
			continue
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			c.enqueue(serverMessage{Type: "error", Code: "bad_command", Message: "invalid command"})
			continue
		}
		if !actionPattern.MatchString(cmd.ID) {
			c.enqueue(serverMessage{Type: "error", Code: "bad_command", Message: "action ID is required"})
			continue
		}
		c.room.handleCommand(c, cmd)
	}
}

func (r *room) detach(c *client) {
	r.mu.Lock()
	if r.clients[c.seat] == c {
		delete(r.clients, c.seat)
	}
	r.lastActivity = r.server.now()
	r.mu.Unlock()
	r.publish()
}

func (r *room) publish() {
	r.dispatchMu.Lock()
	defer r.dispatchMu.Unlock()
	r.publishLocked()
}

// publishLocked snapshots room data under its state lock and performs all
// projection, JSON encoding, and network queueing after releasing that lock.
func (r *room) publishLocked() {
	r.mu.Lock()
	seats := make([]seatSummary, len(r.seats))
	clients := make(map[domain.SeatID]*client, len(r.clients))
	for id, c := range r.clients {
		clients[id] = c
	}
	for i, p := range r.seats {
		seats[i] = seatSummary{ID: p.ID, Name: p.Name, Online: clients[p.ID] != nil, Host: i == 0}
	}
	state := r.state
	events := append([]domain.Event(nil), r.lastEvents...)
	r.mu.Unlock()
	for seat, c := range clients {
		message := serverMessage{Room: r.id, Seats: seats}
		if state == nil {
			message.Type = "lobby"
		} else {
			view, err := domain.ProjectForSeat(*state, r.server.catalog, seat)
			if err != nil {
				log.Printf("project room %s: %v", r.id, err)
				continue
			}
			message.Type, message.Revision, message.View, message.Events = "state", state.Revision, &view, events
		}
		c.enqueue(message)
	}
}

func (r *room) handleCommand(c *client, cmd command) {
	r.dispatchMu.Lock()
	defer r.dispatchMu.Unlock()
	r.mu.Lock()
	if r.clients[c.seat] != c {
		r.mu.Unlock()
		c.enqueue(serverMessage{Type: "error", ID: cmd.ID, Code: "stale_session", Message: "reconnect to continue"})
		return
	}
	fingerprint := cmd.Type + ":" + string(cmd.Card)
	if existing, ok := r.seen[c.seat][cmd.ID]; ok {
		r.mu.Unlock()
		if existing.fingerprint != fingerprint {
			c.enqueue(serverMessage{Type: "error", ID: cmd.ID, Code: "action_id_conflict", Message: "action ID was already used"})
		} else {
			c.enqueue(serverMessage{Type: "ack", ID: cmd.ID, Revision: existing.revision})
		}
		return
	}
	var next domain.MatchState
	var events []domain.Event
	var err error
	switch cmd.Type {
	case "start":
		if r.state != nil {
			err = errors.New("game already started")
			break
		}
		if len(r.seats) < 2 {
			err = errors.New("at least two seats are required")
			break
		}
		if r.seats[0].ID != c.seat {
			err = errors.New("only the host can start")
			break
		}
		ids := make([]domain.SeatID, len(r.seats))
		for i, p := range r.seats {
			ids[i] = p.ID
		}
		next, err = domain.NewMatch(ids, r.server.config, r.server.catalog, r.server.shuffle())
		if err == nil {
			next.Revision = 1
			events = []domain.Event{{Kind: domain.EventHandStarted, Revision: 1, Hand: 1, Seat: next.Leader}}
		}
	case "play":
		if r.state == nil {
			err = errors.New("game has not started")
			break
		}
		next, events, err = domain.Step(*r.state, domain.Action{Kind: domain.ActionPlayCard, Seat: c.seat, Card: cmd.Card}, r.server.catalog)
	case "next":
		if r.state == nil {
			err = errors.New("game has not started")
			break
		}
		if r.seats[0].ID != c.seat {
			err = errors.New("only the host can start a hand")
			break
		}
		if r.state.Phase != domain.PhaseHandComplete {
			err = errors.New("hand is not complete")
			break
		}
		deck := domain.StandardDeck()
		r.server.shuffle().Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
		order := make([]domain.CardID, len(deck))
		for i, card := range deck {
			order[i] = card.ID
		}
		next, events, err = domain.Step(*r.state, domain.Action{Kind: domain.ActionStartHand, Seat: c.seat, DeckOrder: order}, r.server.catalog)
	default:
		err = errors.New("unknown command")
	}
	if err != nil {
		revision := uint64(0)
		if r.state != nil {
			revision = r.state.Revision
		}
		r.mu.Unlock()
		code := "invalid_action"
		var transition *domain.TransitionError
		if errors.As(err, &transition) {
			code = string(transition.Code)
		}
		c.enqueue(serverMessage{Type: "error", ID: cmd.ID, Code: code, Message: err.Error(), Revision: revision})
		return
	}
	r.state = &next
	r.lastEvents = events
	r.lastActivity = r.server.now()
	if r.seen[c.seat] == nil {
		r.seen[c.seat] = make(map[string]acceptedCommand)
	}
	r.seen[c.seat][cmd.ID] = acceptedCommand{fingerprint: fingerprint, revision: next.Revision}
	r.mu.Unlock()
	c.enqueue(serverMessage{Type: "ack", ID: cmd.ID, Revision: next.Revision})
	r.publishLocked()
}
