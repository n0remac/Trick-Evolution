package gameui

import (
	"bytes"
	"context"
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
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/n0remac/trick-evolution/browser3d"
	"github.com/n0remac/trick-evolution/domain"
)

const roomIdleTTL = 2 * time.Hour

var roomPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)
var actionPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

type shuffleFactory func() domain.ShuffleSource

type Server struct {
	mu         sync.Mutex
	rooms      map[string]*room
	catalog    domain.Catalog
	config     domain.Config
	shuffle    shuffleFactory
	botDelay   time.Duration
	botOptions BotOptions
	now        func() time.Time
	done       chan struct{}
	closeOnce  sync.Once
}

type participant struct {
	ID    domain.SeatID
	Name  string
	Token string
	Bot   bool
}

type room struct {
	mu             sync.Mutex
	dispatchMu     sync.Mutex // Serializes accepted actions and snapshot delivery for this room.
	id             string
	seats          []participant
	clients        map[domain.SeatID]*client
	state          *domain.MatchState
	lastEvents     []domain.Event
	publicPlays    []ObservedPlay
	publicTricks   []ObservedTrick
	seen           map[domain.SeatID]map[string]acceptedCommand
	lastActivity   time.Time
	server         *Server
	config         domain.Config
	settingsStatus string
	botWake        chan struct{}
	done           chan struct{}
	ctx            context.Context
	cancel         context.CancelFunc
	closed         bool
	workerStarted  bool
	sequence       uint64 // Monotonic for lobby changes and same-revision reconnects.
}

type acceptedCommand struct {
	fingerprint string
	revision    uint64
}

type client struct {
	conn     *websocket.Conn
	send     chan outbound
	done     chan struct{}
	stopOnce sync.Once
	room     *room
	seat     domain.SeatID
	html     bool
}

type outbound struct {
	data     []byte
	terminal bool
}

type seatSummary struct {
	ID     domain.SeatID `json:"id"`
	Name   string        `json:"name"`
	Online bool          `json:"online"`
	Host   bool          `json:"host"`
	Bot    bool          `json:"bot"`
}

type serverMessage struct {
	Type           string             `json:"type"`
	ID             string             `json:"id,omitempty"`
	Code           string             `json:"code,omitempty"`
	Message        string             `json:"message,omitempty"`
	Revision       uint64             `json:"revision"`
	Room           string             `json:"room,omitempty"`
	Seats          []seatSummary      `json:"seats,omitempty"`
	View           *domain.PlayerView `json:"view,omitempty"`
	Config         *domain.Config     `json:"config,omitempty"`
	SettingsStatus string             `json:"settingsStatus,omitempty"`
	Events         []domain.Event     `json:"events,omitempty"`
	Sequence       uint64             `json:"-"`
}

type command struct {
	Type             string        `json:"type"`
	ID               string        `json:"id"`
	Card             domain.CardID `json:"card,omitempty"`
	Rule             domain.RuleID `json:"rule,omitempty"`
	BoundaryID       string        `json:"boundaryId,omitempty"`
	Settings         string        `json:"settings,omitempty"`
	ExpectedRevision *uint64       `json:"-"`
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
	return NewServerWithBotOptions(config, shuffle, BotOptions{Mode: BotModeRollout, Budget: DefaultRolloutBudget()})
}

// NewServerWithBotOptions selects the bot policy and its bounded rollout
// budget. A seed factory is injectable for reproducible decisions in tests.
func NewServerWithBotOptions(config domain.Config, shuffle func() domain.ShuffleSource, options BotOptions) (*Server, error) {
	if err := config.Validate(2); err != nil {
		return nil, err
	}
	catalog := domain.DefaultCatalog()
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	if err := domain.ValidateInitialSetup(config, 2, catalog); err != nil {
		return nil, err
	}
	if shuffle == nil {
		return nil, fmt.Errorf("shuffle factory is required")
	}
	if options.Mode != BotModeHeuristic && options.Mode != BotModeRollout {
		return nil, fmt.Errorf("unknown bot mode %q", options.Mode)
	}
	if options.Mode == BotModeRollout {
		if err := options.Budget.validate(); err != nil {
			return nil, err
		}
	}
	if options.SeedFactory == nil {
		options.SeedFactory = randomSeed
	}
	s := &Server{rooms: make(map[string]*room), catalog: catalog, config: config, shuffle: shuffle, botDelay: defaultBotDelay, botOptions: options, now: time.Now, done: make(chan struct{})}
	go s.cleanupLoop()
	return s, nil
}

// Close stops periodic room cleanup. Active connections are closed by their
// HTTP server or clients; production normally keeps Server for process life.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		close(s.done)
		s.mu.Lock()
		for _, gameRoom := range s.rooms {
			gameRoom.mu.Lock()
			gameRoom.closeLocked()
			gameRoom.mu.Unlock()
		}
		s.mu.Unlock()
	})
}

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
	return domain.SeededShuffle(randomSeed())
}

func randomSeed() int64 {
	var seed [8]byte
	if _, err := rand.Read(seed[:]); err != nil {
		panic(err)
	}
	var value int64
	for _, b := range seed {
		value = value<<8 | int64(b)
	}
	return value
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
	mux.HandleFunc("GET /game/vendor/htmx-2.0.11.min.js", serveHTMX)
	mux.HandleFunc("GET /game/vendor/htmx-ext-ws-2.0.4.min.js", serveHTMXWS)
	browser3d.Register(mux)
	mux.HandleFunc("POST /game/join", s.join)
	mux.HandleFunc("POST /game/endless", s.startEndless)
	mux.HandleFunc("POST /game/leave", s.leave)
	mux.HandleFunc("GET /ws/game", s.websocket)
	return mux
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

// Leave is authenticated by the same room, seat and token returned by join.
// dispatchMu serializes it with plays, bot moves and snapshots.
func (s *Server) leave(w http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(w, request.Body, 1024)
	var input struct {
		Room  string        `json:"room"`
		Seat  domain.SeatID `json:"seat"`
		Token string        `json:"token"`
	}
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || !roomPattern.MatchString(input.Room) || input.Seat == "" || input.Token == "" {
		http.Error(w, "invalid leave request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	gameRoom := s.rooms[input.Room]
	s.mu.Unlock()
	if gameRoom == nil {
		http.Error(w, "room is closed", http.StatusGone)
		return
	}
	gameRoom.dispatchMu.Lock()
	defer gameRoom.dispatchMu.Unlock()
	s.mu.Lock()
	gameRoom.mu.Lock()
	if s.rooms[input.Room] != gameRoom || gameRoom.closed {
		gameRoom.mu.Unlock()
		s.mu.Unlock()
		http.Error(w, "room is closed", http.StatusGone)
		return
	}
	index := -1
	for i, seat := range gameRoom.seats {
		if !seat.Bot && seat.ID == input.Seat && seat.Token == input.Token {
			index = i
			break
		}
	}
	if index < 0 {
		gameRoom.mu.Unlock()
		s.mu.Unlock()
		http.Error(w, "invalid session", http.StatusUnauthorized)
		return
	}
	departing := gameRoom.seats[index]
	if gameRoom.state == nil && index > 0 {
		client := gameRoom.clients[departing.ID]
		delete(gameRoom.clients, departing.ID)
		delete(gameRoom.seen, departing.ID)
		gameRoom.seats = append(gameRoom.seats[:index], gameRoom.seats[index+1:]...)
		gameRoom.lastActivity = s.now()
		gameRoom.mu.Unlock()
		s.mu.Unlock()
		if client != nil {
			client.enqueue(serverMessage{Type: "room_closed", Code: "lobby", Message: "You left the room."})
		}
		gameRoom.publishLocked()
	} else {
		if gameRoom.state != nil {
			next, events, err := domain.Step(*gameRoom.state, domain.Action{Kind: domain.ActionLeaveGame, Seat: departing.ID}, s.catalog)
			if err != nil {
				gameRoom.mu.Unlock()
				s.mu.Unlock()
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			gameRoom.recordPublicEventsLocked(gameRoom.state, events)
			gameRoom.state, gameRoom.lastEvents = &next, events
		}
		gameRoom.closeLocked()
		gameRoom.mu.Unlock()
		s.mu.Unlock()
		gameRoom.finishClosedLocked(gameRoom.state != nil, departing.Name+" left the room.")
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, `{"accepted":true}`)
}

// The caller holds dispatchMu and has already marked the room closed under
// room.mu. Registry removal uses server.mu -> room.mu, like join and prune.
func (r *room) finishClosedLocked(final bool, reason string) {
	r.server.mu.Lock()
	r.mu.Lock()
	if r.server.rooms[r.id] == r {
		delete(r.server.rooms, r.id)
	}
	clients := make([]*client, 0, len(r.clients))
	for _, c := range r.clients {
		clients = append(clients, c)
	}
	r.mu.Unlock()
	r.server.mu.Unlock()
	if final {
		r.publishSnapshotLocked(true)
	}
	code := "final"
	if !final {
		code = "lobby"
	}
	for _, c := range clients {
		c.enqueue(serverMessage{Type: "room_closed", Room: r.id, Code: code, Message: reason})
	}
}

func (s *Server) pruneLocked() {
	for id, gameRoom := range s.rooms {
		gameRoom.mu.Lock()
		stale := len(gameRoom.clients) == 0 && s.now().Sub(gameRoom.lastActivity) > roomIdleTTL
		if stale {
			gameRoom.closeLocked()
			delete(s.rooms, id)
		}
		gameRoom.mu.Unlock()
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
	format := r.URL.Query().Get("format")
	if format != "" && format != "html" {
		http.Error(w, "invalid format", http.StatusBadRequest)
		return
	}
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
	if gameRoom.closed {
		gameRoom.mu.Unlock()
		http.Error(w, "room is closed", http.StatusGone)
		return
	}
	var seat domain.SeatID
	for _, p := range gameRoom.seats {
		if !p.Bot && p.Token == token {
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
	c := &client{conn: conn, send: make(chan outbound, 16), done: make(chan struct{}), room: gameRoom, seat: seat, html: format == "html"}
	gameRoom.mu.Lock()
	valid := false
	for _, p := range gameRoom.seats {
		if !p.Bot && p.ID == seat && p.Token == token {
			valid = true
			break
		}
	}
	if gameRoom.closed || !valid {
		gameRoom.mu.Unlock()
		c.stop()
		return
	}
	previous := gameRoom.clients[seat]
	gameRoom.clients[seat] = c
	gameRoom.lastActivity = s.now()
	gameRoom.mu.Unlock()
	if previous != nil {
		previous.stop()
	}
	go c.writePump()
	gameRoom.publish()
	gameRoom.signalBot()
	c.readPump()
}

func (c *client) stop() { c.stopOnce.Do(func() { close(c.done); _ = c.conn.Close() }) }

func (c *client) enqueue(message serverMessage) {
	var data []byte
	if c.html {
		switch message.Type {
		case "lobby", "state":
			config := c.room.server.config
			if message.Config != nil {
				config = *message.Config
			}
			data = []byte(renderScreen(message, c.seat, c.room.server.catalog, config))
		case "error":
			data = []byte(noticeNode(message.Message).Render())
		case "room_closed":
			data = []byte(renderClosure(message))
		default: // Acknowledgements have no visual update.
			return
		}
	} else {
		var err error
		data, err = json.Marshal(message)
		if err != nil {
			log.Printf("encode game message: %v", err)
			return
		}
	}
	item := outbound{data: data, terminal: message.Type == "room_closed"}
	final := message.Type == "state" && message.View != nil && message.View.Public.Phase == domain.PhaseGameComplete
	if item.terminal || final {
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-c.done:
		case c.send <- item:
		case <-timer.C:
			c.stop()
		}
		return
	}
	select {
	case <-c.done:
		return
	case c.send <- item:
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
		case item := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, item.data); err != nil {
				return
			}
			if item.terminal {
				_ = c.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "room closed"), time.Now().Add(5*time.Second))
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
		cmd, err := decodeCommand(data, c.html)
		if err != nil {
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

// HTMX's WS extension adds HEADERS to form JSON. Decode that envelope only
// for HTML clients; legacy JSON clients retain their strict command contract.
func decodeCommand(data []byte, html bool) (command, error) {
	var cmd command
	if html {
		var envelope struct {
			Type       string          `json:"type"`
			ID         string          `json:"id"`
			Card       domain.CardID   `json:"card"`
			Rule       domain.RuleID   `json:"rule"`
			BoundaryID string          `json:"boundaryId"`
			Settings   string          `json:"settings"`
			Revision   string          `json:"revision"`
			Headers    json.RawMessage `json:"HEADERS"`
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&envelope); err != nil {
			return cmd, err
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			return cmd, fmt.Errorf("trailing command data")
		}
		if len(envelope.Headers) != 0 && envelope.Headers[0] != '{' {
			return cmd, fmt.Errorf("invalid HTMX headers")
		}
		if envelope.Revision == "" {
			return cmd, fmt.Errorf("missing revision")
		}
		revision, err := strconv.ParseUint(envelope.Revision, 10, 64)
		if err != nil {
			return cmd, fmt.Errorf("invalid revision: %w", err)
		}
		cmd = command{Type: envelope.Type, ID: envelope.ID, Card: envelope.Card, Rule: envelope.Rule, BoundaryID: envelope.BoundaryID, Settings: envelope.Settings, ExpectedRevision: &revision}
		return cmd, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cmd); err != nil {
		return cmd, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return cmd, fmt.Errorf("trailing command data")
	}
	return cmd, nil
}

func (r *room) detach(c *client) {
	r.mu.Lock()
	changed := false
	if r.clients[c.seat] == c {
		delete(r.clients, c.seat)
		changed = !r.closed
	}
	r.lastActivity = r.server.now()
	r.mu.Unlock()
	if changed {
		r.publish()
	}
}

func (r *room) publish() {
	r.dispatchMu.Lock()
	defer r.dispatchMu.Unlock()
	r.publishLocked()
}

// publishLocked snapshots room data under its state lock and performs all
// projection, JSON encoding, and network queueing after releasing that lock.
func (r *room) publishLocked() {
	r.publishSnapshotLocked(false)
}

func (r *room) publishSnapshotLocked(allowClosed bool) {
	r.mu.Lock()
	if r.closed && !allowClosed {
		r.mu.Unlock()
		return
	}
	r.sequence++
	sequence := r.sequence
	seats := make([]seatSummary, len(r.seats))
	clients := make(map[domain.SeatID]*client, len(r.clients))
	for id, c := range r.clients {
		clients[id] = c
	}
	for i, p := range r.seats {
		seats[i] = seatSummary{ID: p.ID, Name: p.Name, Online: p.Bot || clients[p.ID] != nil, Host: i == 0, Bot: p.Bot}
	}
	state := r.state
	config := r.config
	settingsStatus := r.settingsStatus
	events := append([]domain.Event(nil), r.lastEvents...)
	r.mu.Unlock()
	for seat, c := range clients {
		message := serverMessage{Room: r.id, Seats: seats, Sequence: sequence, Config: &config, SettingsStatus: settingsStatus}
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
	if r.closed {
		r.mu.Unlock()
		c.enqueue(serverMessage{Type: "error", ID: cmd.ID, Code: "room_closed", Message: "room is closed"})
		return
	}
	if r.clients[c.seat] != c {
		r.mu.Unlock()
		c.enqueue(serverMessage{Type: "error", ID: cmd.ID, Code: "stale_session", Message: "reconnect to continue"})
		return
	}
	fingerprint := cmd.Type + ":" + string(cmd.Card) + ":" + string(cmd.Rule) + ":" + cmd.BoundaryID + ":" + cmd.Settings
	if existing, ok := r.seen[c.seat][cmd.ID]; ok {
		r.mu.Unlock()
		if existing.fingerprint != fingerprint {
			c.enqueue(serverMessage{Type: "error", ID: cmd.ID, Code: "action_id_conflict", Message: "action ID was already used"})
		} else {
			c.enqueue(serverMessage{Type: "ack", ID: cmd.ID, Revision: existing.revision})
		}
		return
	}
	if cmd.ExpectedRevision != nil {
		current := uint64(0)
		if r.state != nil {
			current = r.state.Revision
		}
		if *cmd.ExpectedRevision != current {
			r.mu.Unlock()
			c.enqueue(serverMessage{Type: "error", ID: cmd.ID, Code: "stale_revision", Message: "Game changed. Use the updated controls.", Revision: current})
			return
		}
	}
	if cmd.Type == "configure" || cmd.Type == "reset_rules" {
		var updated domain.Config
		var configErr error
		if r.state != nil {
			configErr = errors.New("rules are locked after the match starts")
		} else if r.seats[0].ID != c.seat {
			configErr = errors.New("only the host can edit game rules")
		} else if cmd.Type == "reset_rules" {
			updated = r.server.config
			configErr = domain.ValidateInitialSetup(updated, len(r.seats), r.server.catalog)
		} else {
			var settings struct {
				HandSize     int             `json:"handSize"`
				RevealEvery  int             `json:"revealEvery"`
				InitialRules []domain.RuleID `json:"initialRules"`
			}
			decoder := json.NewDecoder(strings.NewReader(cmd.Settings))
			decoder.DisallowUnknownFields()
			if configErr = decoder.Decode(&settings); configErr == nil {
				if decoder.Decode(new(any)) != io.EOF {
					configErr = errors.New("trailing settings data")
				}
			}
			if configErr == nil {
				updated = domain.Config{HandSize: settings.HandSize, RevealEvery: settings.RevealEvery, MaxHands: r.config.MaxHands, InitialRules: settings.InitialRules}
				configErr = domain.ValidateInitialSetup(updated, len(r.seats), r.server.catalog)
			}
		}
		if configErr != nil {
			r.mu.Unlock()
			c.enqueue(serverMessage{Type: "error", ID: cmd.ID, Code: "invalid_config", Message: configErr.Error()})
			return
		}
		r.config = updated
		if cmd.Type == "reset_rules" {
			r.settingsStatus = "Defaults restored for this room. They apply when the game starts."
		} else {
			r.settingsStatus = "Settings saved for this room. They apply when the game starts."
		}
		r.lastActivity = r.server.now()
		if r.seen[c.seat] == nil {
			r.seen[c.seat] = make(map[string]acceptedCommand)
		}
		r.seen[c.seat][cmd.ID] = acceptedCommand{fingerprint: fingerprint}
		r.mu.Unlock()
		c.enqueue(serverMessage{Type: "ack", ID: cmd.ID})
		r.publishLocked()
		return
	}
	if cmd.Type == "add_bot" {
		code, detail := "", ""
		switch {
		case r.state != nil:
			code, detail = "wrong_phase", "bots can only be added in the lobby"
		case r.seats[0].ID != c.seat:
			code, detail = "not_host", "only the host can add bots"
		case len(r.seats) >= 4:
			code, detail = "room_full", "room already has four seats"
		}
		if code != "" {
			r.mu.Unlock()
			c.enqueue(serverMessage{Type: "error", ID: cmd.ID, Code: code, Message: detail})
			return
		}
		botNumber := 1
		for _, seat := range r.seats {
			if seat.Bot {
				botNumber++
			}
		}
		r.seats = append(r.seats, participant{ID: domain.SeatID(fmt.Sprintf("bot:%d", botNumber)), Name: fmt.Sprintf("Bot %d", botNumber), Bot: true})
		r.lastActivity = r.server.now()
		if r.seen[c.seat] == nil {
			r.seen[c.seat] = make(map[string]acceptedCommand)
		}
		r.seen[c.seat][cmd.ID] = acceptedCommand{fingerprint: fingerprint}
		r.mu.Unlock()
		c.enqueue(serverMessage{Type: "ack", ID: cmd.ID})
		r.publishLocked()
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
		next, err = domain.NewMatch(ids, r.config, r.server.catalog, r.server.shuffle())
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
	case "choose_rule":
		if r.state == nil {
			err = errors.New("game has not started")
			break
		}
		next, events, err = domain.Step(*r.state, domain.Action{Kind: domain.ActionChooseRule, Seat: c.seat, Rule: cmd.Rule, BoundaryID: cmd.BoundaryID}, r.server.catalog)
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
	case "end":
		if r.state == nil {
			err = errors.New("game has not started")
			break
		}
		next, events, err = domain.Step(*r.state, domain.Action{Kind: domain.ActionEndGame, Seat: c.seat}, r.server.catalog)
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
	r.recordPublicEventsLocked(r.state, events)
	if r.config.Mode == domain.ModeEndless && cmd.Type == "next" && len(next.Seats) != len(r.seats) {
		participants := make([]participant, len(next.Seats))
		participants[0] = r.seats[0]
		for i := 1; i < len(next.Seats); i++ {
			participants[i] = participant{ID: next.Seats[i].ID, Name: fmt.Sprintf("Bot %d", i), Bot: true}
		}
		r.seats = participants
	}
	r.state = &next
	complete := next.Phase == domain.PhaseGameComplete
	if complete {
		r.closeLocked()
	}
	r.lastEvents = events
	r.lastActivity = r.server.now()
	launchWorker := false
	if !r.workerStarted && r.hasBotLocked() {
		r.workerStarted = true
		launchWorker = true
	}
	if r.seen[c.seat] == nil {
		r.seen[c.seat] = make(map[string]acceptedCommand)
	}
	r.seen[c.seat][cmd.ID] = acceptedCommand{fingerprint: fingerprint, revision: next.Revision}
	r.mu.Unlock()
	c.enqueue(serverMessage{Type: "ack", ID: cmd.ID, Revision: next.Revision})
	if complete {
		reason := "Game complete. Room closed."
		if r.config.Mode == domain.ModeEndless {
			reason = "Run ended. Room closed."
		}
		r.finishClosedLocked(true, reason)
		return
	}
	r.publishLocked()
	if launchWorker {
		go r.runBots()
	}
	r.signalBot()
}
