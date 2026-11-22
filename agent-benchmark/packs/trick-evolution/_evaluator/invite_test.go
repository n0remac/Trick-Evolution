package gameui

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/n0remac/trick-evolution/domain"
)

// These checks exercise the public HTTP and websocket contracts. The prompt
// permits an invite returned at start or one requested by the live player.
type benchmarkInvite struct {
	path, room, value string
	expiresAt         time.Time
}

// net.Pipe lets the real HTTP and websocket handlers run when the benchmark
// host disallows listening sockets. Each dial gets a separate connection.
type benchmarkPipeListener struct {
	connections chan net.Conn
	closed      chan struct{}
}

func (l *benchmarkPipeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connections:
		return conn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}
func (l *benchmarkPipeListener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return nil
}
func (l *benchmarkPipeListener) Addr() net.Addr { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }
func (l *benchmarkPipeListener) dial(ctx context.Context, _, _ string) (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case l.connections <- server:
		return client, nil
	case <-ctx.Done():
		client.Close()
		server.Close()
		return nil, ctx.Err()
	case <-l.closed:
		client.Close()
		server.Close()
		return nil, net.ErrClosed
	}
}

type benchmarkHarness struct {
	base   string
	client *http.Client
	dialer websocket.Dialer
}

func benchmarkServe(t *testing.T, handler http.Handler) benchmarkHarness {
	t.Helper()
	listener := &benchmarkPipeListener{connections: make(chan net.Conn), closed: make(chan struct{})}
	server := &http.Server{Handler: handler}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close(); _ = listener.Close() })
	transport := &http.Transport{DialContext: listener.dial, Proxy: nil}
	t.Cleanup(transport.CloseIdleConnections)
	return benchmarkHarness{base: "http://benchmark.test", client: &http.Client{Transport: transport, Timeout: 5 * time.Second}, dialer: websocket.Dialer{NetDialContext: listener.dial}}
}

func (h benchmarkHarness) dial(t *testing.T, session testSession) *websocket.Conn {
	t.Helper()
	wsURL := "ws://benchmark.test/ws/game?room=" + url.QueryEscape(session.Room) + "&token=" + url.QueryEscape(session.Token)
	conn, response, err := h.dialer.Dial(wsURL, http.Header{"Origin": []string{h.base}})
	if err != nil {
		t.Fatalf("dial room: %v (%v)", err, response)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func benchmarkPost(t *testing.T, h benchmarkHarness, path string, input any, output any) int {
	t.Helper()
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	response, err := h.client.Post(h.base+path, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if output != nil && response.StatusCode == http.StatusOK {
		if err := json.NewDecoder(response.Body).Decode(output); err != nil {
			t.Fatal(err)
		}
	}
	return response.StatusCode
}

func benchmarkStart(t *testing.T, h benchmarkHarness, name string) (testSession, string) {
	t.Helper()
	var response struct {
		Room          string
		Seat          domain.SeatID
		Token, Invite string
	}
	if status := benchmarkPost(t, h, "/game/endless", map[string]string{"name": name}, &response); status != http.StatusOK {
		t.Fatalf("start: HTTP %d", status)
	}
	if response.Room == "" || response.Seat == "" || response.Token == "" {
		t.Fatalf("start omitted session: %+v", response)
	}
	return testSession{Room: response.Room, Seat: response.Seat, Token: response.Token}, response.Invite
}

func benchmarkIssue(t *testing.T, h benchmarkHarness, host testSession, initial string) benchmarkInvite {
	t.Helper()
	if initial != "" {
		return benchmarkInvite{path: "/game/endless/join", room: host.Room, value: initial}
	}
	var issued struct {
		Invite    string
		ExpiresAt time.Time
	}
	status := benchmarkPost(t, h, "/game/endless/invite", map[string]string{"room": host.Room, "token": host.Token}, &issued)
	if status != http.StatusOK || issued.Invite == "" {
		t.Fatalf("live player cannot issue invitation: HTTP %d, %+v", status, issued)
	}
	return benchmarkInvite{path: "/game/endless/accept", room: host.Room, value: issued.Invite, expiresAt: issued.ExpiresAt}
}

func benchmarkAccept(t *testing.T, h benchmarkHarness, invite benchmarkInvite, credential, name string, output any) int {
	t.Helper()
	return benchmarkPost(t, h, invite.path, map[string]string{"room": invite.room, "invite": credential, "name": name}, output)
}

func benchmarkRejected(t *testing.T, status int, label string) {
	t.Helper()
	if status < 400 || status >= 500 {
		t.Fatalf("%s was accepted or failed internally: HTTP %d", label, status)
	}
}

func benchmarkState(t *testing.T, s *Server, roomID string) []byte {
	t.Helper()
	s.mu.Lock()
	r := s.rooms[roomID]
	s.mu.Unlock()
	if r == nil {
		t.Fatal("live room missing")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	state, err := json.Marshal(r.state)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func benchmarkSeenGuest(t *testing.T, conn *websocket.Conn, guest testSession) serverMessage {
	t.Helper()
	for i := 0; i < 20; i++ {
		message := readMessage(t, conn, "state")
		for _, seat := range message.Seats {
			if seat.ID == guest.Seat && seat.Name == "Bob" && !seat.Bot {
				return message
			}
		}
	}
	t.Fatal("host websocket never observed named guest")
	return serverMessage{}
}

func benchmarkMapSeat(id, old, replacement domain.SeatID) domain.SeatID {
	if id == old {
		return replacement
	}
	return id
}

func TestBenchmarkInviteJoinsExactLiveHand(t *testing.T) {
	s, err := NewServerWithBotOptions(domain.DefaultConfig(), func() domain.ShuffleSource { return identityShuffle{} }, BotOptions{Mode: BotModeHeuristic})
	if err != nil {
		t.Fatal(err)
	}
	s.botDelay = time.Hour
	web := benchmarkServe(t, s.Handler())
	t.Cleanup(s.Close)

	host, initial := benchmarkStart(t, web, "Alice")
	hostConn := web.dial(t, host)
	before := readMessage(t, hostConn, "state")
	if before.View == nil || before.View.Public.HandNumber != 1 || len(before.View.Hand) != 3 || len(before.Seats) != 2 {
		t.Fatalf("opening hand: %+v", before)
	}
	var bot domain.SeatID
	for _, seat := range before.Seats {
		if seat.Bot {
			bot = seat.ID
		}
	}
	if bot == "" {
		t.Fatal("Endless table has no vacant bot seat")
	}
	s.mu.Lock()
	r := s.rooms[host.Room]
	s.mu.Unlock()
	r.mu.Lock()
	botView, err := domain.ProjectForSeat(*r.state, s.catalog, bot)
	r.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	invite := benchmarkIssue(t, web, host, initial)
	if invite.value == host.Token {
		t.Fatal("reconnect token is the invitation")
	}
	stable := benchmarkState(t, s, host.Room)
	if initial == "" {
		benchmarkRejected(t, benchmarkPost(t, web, "/game/endless/invite", map[string]string{"room": host.Room, "token": "invalid"}, nil), "unauthorized invite creation")
		if !bytes.Equal(stable, benchmarkState(t, s, host.Room)) {
			t.Fatal("unauthorized invite creation changed the game")
		}
	}

	for _, bad := range []struct{ label, value string }{{"invalid invite", "invalid"}, {"reconnect token", host.Token}} {
		benchmarkRejected(t, benchmarkAccept(t, web, invite, bad.value, "Other", nil), bad.label)
		if !bytes.Equal(stable, benchmarkState(t, s, host.Room)) {
			t.Fatalf("%s changed the game", bad.label)
		}
	}
	other, _ := benchmarkStart(t, web, "Other host")
	wrongRoom := invite
	wrongRoom.room = other.Room
	otherStable := benchmarkState(t, s, other.Room)
	benchmarkRejected(t, benchmarkAccept(t, web, wrongRoom, invite.value, "Other", nil), "invite for another room")
	if !bytes.Equal(stable, benchmarkState(t, s, host.Room)) || !bytes.Equal(otherStable, benchmarkState(t, s, other.Room)) {
		t.Fatal("wrong-room invite changed a game")
	}
	// A share credential must not authenticate a private websocket session.
	wsURL := "ws://benchmark.test/ws/game?room=" + url.QueryEscape(host.Room) + "&token=" + url.QueryEscape(invite.value)
	unauthorized, response, err := web.dialer.Dial(wsURL, http.Header{"Origin": []string{web.base}})
	if unauthorized != nil {
		unauthorized.Close()
	}
	if err == nil || response == nil || response.StatusCode == http.StatusSwitchingProtocols {
		t.Fatal("invite authenticated a private websocket")
	}

	var guest testSession
	if status := benchmarkAccept(t, web, invite, invite.value, "Bob", &guest); status != http.StatusOK {
		t.Fatalf("valid invite: HTTP %d", status)
	}
	if guest.Room != host.Room || guest.Seat == "" || guest.Seat == host.Seat || guest.Token == "" || guest.Token == host.Token || guest.Token == invite.value {
		t.Fatalf("guest authority: %+v", guest)
	}
	guestConn := web.dial(t, guest)
	afterHost := benchmarkSeenGuest(t, hostConn, guest)
	afterGuest := readMessage(t, guestConn, "state")
	for _, message := range []serverMessage{afterHost, afterGuest} {
		if message.View == nil || message.Room != host.Room || len(message.Seats) != 2 || message.View.Public.HandNumber != before.View.Public.HandNumber || message.View.Public.Phase != before.View.Public.Phase || message.View.Public.TrickNumber != before.View.Public.TrickNumber {
			t.Fatalf("guest missed live hand: %+v", message)
		}
		if message.Seats[0].ID != host.Seat || message.Seats[1].ID != guest.Seat || message.Seats[0].Name != "Alice" || message.Seats[1].Name != "Bob" || message.Seats[1].Bot {
			t.Fatalf("roster not shared: %+v", message.Seats)
		}
	}
	a, b, c := before.View.Public, afterHost.View.Public, afterGuest.View.Public
	if !reflect.DeepEqual(afterHost.View.Hand, before.View.Hand) || !reflect.DeepEqual(afterGuest.View.Hand, botView.Hand) || !reflect.DeepEqual(b, c) {
		t.Fatal("websocket views disagree or private cards changed")
	}
	if a.Mode != b.Mode || a.Round != b.Round || a.RoundHand != b.RoundHand || a.Dealer != benchmarkMapSeat(b.Dealer, guest.Seat, bot) || a.Leader != benchmarkMapSeat(b.Leader, guest.Seat, bot) || a.Turn != benchmarkMapSeat(b.Turn, guest.Seat, bot) || !reflect.DeepEqual(a.Rules, b.Rules) || !reflect.DeepEqual(a.ActiveField, b.ActiveField) || !reflect.DeepEqual(a.Effective, b.Effective) || len(a.CurrentTrick) != len(b.CurrentTrick) {
		t.Fatalf("join changed turn, trick, or rules: before=%+v after=%+v", a, b)
	}
	for i := range a.CurrentTrick {
		if a.CurrentTrick[i].Card != b.CurrentTrick[i].Card || a.CurrentTrick[i].Seat != benchmarkMapSeat(b.CurrentTrick[i].Seat, guest.Seat, bot) {
			t.Fatal("join changed current trick")
		}
	}
	for i := range a.Seats {
		if a.Seats[i].ID != benchmarkMapSeat(b.Seats[i].ID, guest.Seat, bot) || a.Seats[i].Score != b.Seats[i].Score || a.Seats[i].TricksWon != b.Seats[i].TricksWon {
			t.Fatal("join changed seat scores")
		}
	}
	if a.RunScores[host.Seat] != b.RunScores[host.Seat] || a.RunScores[bot] != b.RunScores[guest.Seat] {
		t.Fatal("join changed run scores")
	}

	joined := benchmarkState(t, s, host.Room)
	benchmarkRejected(t, benchmarkAccept(t, web, invite, invite.value, "Third", nil), "full or used invitation")
	if !bytes.Equal(joined, benchmarkState(t, s, host.Room)) {
		t.Fatal("rejected full table changed the game")
	}
	// A fresh invite, when the workflow supports one, must also fail at a full table.
	if initial == "" {
		status := benchmarkPost(t, web, "/game/endless/invite", map[string]string{"room": host.Room, "token": host.Token}, nil)
		benchmarkRejected(t, status, "full-table invite request")
	}
}

func TestBenchmarkInvitePreservesOrdinaryJoin(t *testing.T) {
	s, err := NewServerWithBotOptions(domain.DefaultConfig(), func() domain.ShuffleSource { return identityShuffle{} }, BotOptions{Mode: BotModeHeuristic})
	if err != nil {
		t.Fatal(err)
	}
	web := benchmarkServe(t, s.Handler())
	t.Cleanup(s.Close)
	var alice, bob testSession
	for _, player := range []struct {
		name   string
		output *testSession
	}{{"Alice", &alice}, {"Bob", &bob}} {
		if status := benchmarkPost(t, web, "/game/join", map[string]string{"room": "ordinary-room", "name": player.name}, player.output); status != http.StatusOK {
			t.Fatalf("ordinary join %s: HTTP %d", player.name, status)
		}
	}
	if alice.Room != bob.Room || alice.Seat == bob.Seat || alice.Token == bob.Token || alice.Token == "" || bob.Token == "" {
		t.Fatalf("ordinary joins lost distinct sessions: %+v %+v", alice, bob)
	}
	conn := web.dial(t, bob)
	if message := readMessage(t, conn, "lobby"); len(message.Seats) != 2 || message.Seats[0].Name != "Alice" || message.Seats[1].Name != "Bob" {
		t.Fatalf("ordinary lobby: %+v", message)
	}
}

func TestBenchmarkInviteExpiresWithoutChangingGame(t *testing.T) {
	s, err := NewServerWithBotOptions(domain.DefaultConfig(), func() domain.ShuffleSource { return identityShuffle{} }, BotOptions{Mode: BotModeHeuristic})
	if err != nil {
		t.Fatal(err)
	}
	s.botDelay = time.Hour
	web := benchmarkServe(t, s.Handler())
	t.Cleanup(s.Close)
	host, initial := benchmarkStart(t, web, "Alice")
	invite := benchmarkIssue(t, web, host, initial)
	before := benchmarkState(t, s, host.Room)
	if !invite.expiresAt.IsZero() {
		s.now = func() time.Time { return invite.expiresAt.Add(time.Second) }
		benchmarkRejected(t, benchmarkAccept(t, web, invite, invite.value, "Bob", nil), "expired invitation")
		if !bytes.Equal(before, benchmarkState(t, s, host.Room)) {
			t.Fatal("expired invitation changed the game")
		}
	} else {
		// A room closure also expires its invitation. This covers invite schemes
		// whose lifetime is the live run rather than a clock deadline.
		status := benchmarkPost(t, web, "/game/leave", map[string]string{"room": host.Room, "seat": string(host.Seat), "token": host.Token}, nil)
		if status < 200 || status >= 300 {
			t.Fatalf("close invited room: HTTP %d", status)
		}
		benchmarkRejected(t, benchmarkAccept(t, web, invite, invite.value, "Bob", nil), "closed-room invitation")
	}
}
