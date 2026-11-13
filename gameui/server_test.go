package gameui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/n0remac/trick-evolution/domain"
)

type identityShuffle struct{}

func (identityShuffle) Shuffle(_ int, _ func(i, j int)) {}

type testSession struct {
	Room  string        `json:"room"`
	Seat  domain.SeatID `json:"seat"`
	Name  string        `json:"name"`
	Token string        `json:"token"`
	Host  bool          `json:"host"`
}

func testServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	s, err := NewServerWithConfig(domain.DefaultConfig(), func() domain.ShuffleSource { return identityShuffle{} })
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(s.Handler())
	t.Cleanup(httpServer.Close)
	t.Cleanup(s.Close)
	return s, httpServer
}

func joinRoom(t *testing.T, baseURL, room, name, token string, expectedStatus int) testSession {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"room": room, "name": name, "token": token})
	response, err := http.Post(baseURL+"/game/join", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		t.Fatalf("join %s: got %d, want %d", room, response.StatusCode, expectedStatus)
	}
	if expectedStatus != http.StatusOK {
		return testSession{}
	}
	var session testSession
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if session.Token == "" || session.Seat == "" {
		t.Fatal("join omitted session authority")
	}
	return session
}

func dialRoom(t *testing.T, baseURL string, session testSession) *websocket.Conn {
	t.Helper()
	u, _ := url.Parse(baseURL)
	wsURL := "ws://" + u.Host + "/ws/game?room=" + url.QueryEscape(session.Room) + "&token=" + url.QueryEscape(session.Token)
	conn, response, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Origin": []string{baseURL}})
	if err != nil {
		t.Fatalf("dial room: %v (%v)", err, response)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func sendCommand(t *testing.T, conn *websocket.Conn, command any) {
	t.Helper()
	if err := conn.WriteJSON(command); err != nil {
		t.Fatal(err)
	}
}

func readMessage(t *testing.T, conn *websocket.Conn, wanted string) serverMessage {
	t.Helper()
	for i := 0; i < 20; i++ {
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var message serverMessage
		if err := conn.ReadJSON(&message); err != nil {
			t.Fatal(err)
		}
		if message.Type == wanted {
			return message
		}
	}
	t.Fatalf("did not receive %s", wanted)
	return serverMessage{}
}

func readRevision(t *testing.T, conn *websocket.Conn, revision uint64) serverMessage {
	t.Helper()
	for i := 0; i < 20; i++ {
		message := readMessage(t, conn, "state")
		if message.Revision == revision {
			return message
		}
		if message.Revision > revision {
			t.Fatalf("skipped revision %d, saw %d", revision, message.Revision)
		}
	}
	t.Fatalf("did not receive revision %d", revision)
	return serverMessage{}
}

func TestSessionBindingPrivacyDedupAndReconnect(t *testing.T) {
	s, server := testServer(t)
	host := joinRoom(t, server.URL, "table", "Host", "", http.StatusOK)
	guest := joinRoom(t, server.URL, "table", "Guest", "", http.StatusOK)
	other := joinRoom(t, server.URL, "other", "Other", "", http.StatusOK)
	if !host.Host || guest.Host {
		t.Fatal("incorrect host selection")
	}
	a := dialRoom(t, server.URL, host)
	b := dialRoom(t, server.URL, guest)
	_ = readMessage(t, a, "lobby")
	_ = readMessage(t, b, "lobby")
	sendCommand(t, b, command{Type: "start", ID: "guest-start"})
	if got := readMessage(t, b, "error"); got.Code != "invalid_action" {
		t.Fatalf("guest started room: %+v", got)
	}
	sendCommand(t, a, command{Type: "start", ID: "start-1"})
	if got := readMessage(t, a, "ack"); got.Revision != 1 {
		t.Fatalf("start revision: %+v", got)
	}
	firstA := readRevision(t, a, 1)
	firstB := readRevision(t, b, 1)
	if firstA.View.Seat != host.Seat || firstB.View.Seat != guest.Seat || len(firstA.View.Hand) != 5 || len(firstB.View.Hand) != 5 {
		t.Fatal("wrong private projection")
	}
	for _, private := range firstB.View.Hand {
		encoded, _ := json.Marshal(firstA)
		if bytes.Contains(encoded, []byte(private)) {
			t.Fatalf("guest private card %s leaked to host", private)
		}
	}
	for _, private := range firstA.View.Hand {
		encoded, _ := json.Marshal(firstB)
		if bytes.Contains(encoded, []byte(private)) {
			t.Fatalf("host private card %s leaked to guest", private)
		}
	}
	sendCommand(t, a, command{Type: "start", ID: "start-1"})
	if got := readMessage(t, a, "ack"); got.Revision != 1 {
		t.Fatalf("duplicate changed revision: %+v", got)
	}
	sendCommand(t, a, command{Type: "play", ID: "start-1", Card: firstA.View.Hand[0]})
	if got := readMessage(t, a, "error"); got.Code != "action_id_conflict" {
		t.Fatalf("reused ID accepted: %+v", got)
	}
	// A forged room/player claim is rejected by the strict command schema.
	sendCommand(t, b, map[string]any{"type": "play", "id": "forged", "card": firstA.View.Hand[0], "room": "other", "playerId": host.Seat})
	if got := readMessage(t, b, "error"); got.Code != "bad_command" {
		t.Fatalf("forged identity accepted: %+v", got)
	}
	sendCommand(t, a, command{Type: "play", ID: "wrong-turn", Card: firstA.View.Hand[0]})
	if got := readMessage(t, a, "error"); got.Code != string(domain.ErrNotYourTurn) {
		t.Fatalf("wrong turn accepted: %+v", got)
	}
	s.rooms["table"].mu.Lock()
	if s.rooms["table"].state.Revision != 1 {
		t.Fatal("rejected commands changed revision")
	}
	s.rooms["table"].mu.Unlock()
	_ = b.Close()
	rejoined := joinRoom(t, server.URL, "table", "Renamed", guest.Token, http.StatusOK)
	if rejoined.Seat != guest.Seat || rejoined.Name != "Guest" {
		t.Fatal("reconnect created or renamed a seat")
	}
	b = dialRoom(t, server.URL, rejoined)
	if got := readRevision(t, b, 1); !reflect.DeepEqual(got.View.Hand, firstB.View.Hand) {
		t.Fatal("reconnect lost private hand")
	}
	joinRoom(t, server.URL, "table", "Intruder", "", http.StatusConflict)
	joinRoom(t, server.URL, "table", "Intruder", "bad-token", http.StatusUnauthorized)
	// A valid token from another room does not authorize this room.
	u, _ := url.Parse(server.URL)
	_, response, err := websocket.DefaultDialer.Dial("ws://"+u.Host+"/ws/game?room=table&token="+other.Token, nil)
	if err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("cross-room token accepted: %v, %v", err, response)
	}
	_, response, err = websocket.DefaultDialer.Dial("ws://"+u.Host+"/ws/game?room=table&token="+host.Token, http.Header{"Origin": []string{"http://evil.example"}})
	if err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin websocket accepted: %v, %v", err, response)
	}
}

func chooseLegal(state domain.MatchState) domain.CardID {
	hand := state.Hands[state.Turn]
	resolved, _ := domain.ResolveField(state.Field, domain.DefaultCatalog())
	follow := false
	for _, rule := range resolved {
		if rule.Active && rule.Instance.DefinitionID == domain.RuleFollowSuit {
			follow = true
		}
	}
	if follow && len(state.CurrentTrick) != 0 {
		suit := strings.Split(string(state.CurrentTrick[0].Card), ":")[1]
		for _, id := range hand {
			if strings.Split(string(id), ":")[1] == suit {
				return id
			}
		}
	}
	return hand[0]
}

func TestWebsocketJoinThroughRuleReveal(t *testing.T) {
	s, server := testServer(t)
	host := joinRoom(t, server.URL, "reveal", "A", "", http.StatusOK)
	guest := joinRoom(t, server.URL, "reveal", "B", "", http.StatusOK)
	a, b := dialRoom(t, server.URL, host), dialRoom(t, server.URL, guest)
	_ = readMessage(t, a, "lobby")
	_ = readMessage(t, b, "lobby")
	sendCommand(t, a, command{Type: "start", ID: "start"})
	_ = readMessage(t, a, "ack")
	_ = readRevision(t, a, 1)
	_ = readRevision(t, b, 1)
	connections := map[domain.SeatID]*websocket.Conn{host.Seat: a, guest.Seat: b}
	r := s.rooms["reveal"]
	serial := 0
	for hand := 1; hand <= 3; hand++ {
		for {
			r.mu.Lock()
			state := *r.state
			r.mu.Unlock()
			if state.Phase == domain.PhaseHandComplete {
				break
			}
			card := chooseLegal(state)
			serial++
			conn := connections[state.Turn]
			sendCommand(t, conn, command{Type: "play", ID: fmt.Sprintf("play-%d", serial), Card: card})
			ack := readMessage(t, conn, "ack")
			if ack.Revision != state.Revision+1 {
				t.Fatalf("bad revision: %+v", ack)
			}
			aState := readRevision(t, a, ack.Revision)
			bState := readRevision(t, b, ack.Revision)
			if aState.View.Public.Revision != bState.View.Public.Revision {
				t.Fatal("clients saw different revisions")
			}
		}
		if hand < 3 {
			sendCommand(t, a, command{Type: "next", ID: fmt.Sprintf("next-%d", hand)})
			ack := readMessage(t, a, "ack")
			_ = readRevision(t, a, ack.Revision)
			_ = readRevision(t, b, ack.Revision)
		}
	}
	r.mu.Lock()
	state := *r.state
	events := append([]domain.Event(nil), r.lastEvents...)
	r.mu.Unlock()
	if state.CompletedHands != 3 || len(state.Field) != 2 || events[len(events)-1].Kind != domain.EventRuleRevealed {
		t.Fatalf("reveal did not occur: %+v, %+v", state.Field, events)
	}
	if state.Seats[0].Score+state.Seats[1].Score != 15 {
		t.Fatalf("expected 15 awarded tricks, got %+v", state.Seats)
	}
}

func TestConcurrentRooms(t *testing.T) {
	s, server := testServer(t)
	var sessions [2][2]testSession
	for i, name := range []string{"alpha", "beta"} {
		sessions[i][0] = joinRoom(t, server.URL, name, "A", "", http.StatusOK)
		sessions[i][1] = joinRoom(t, server.URL, name, "B", "", http.StatusOK)
	}
	var wg sync.WaitGroup
	for i := range sessions {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			host, guest := sessions[i][0], sessions[i][1]
			a, b := dialRoom(t, server.URL, host), dialRoom(t, server.URL, guest)
			_ = readMessage(t, a, "lobby")
			_ = readMessage(t, b, "lobby")
			sendCommand(t, a, command{Type: "start", ID: "start"})
			_ = readMessage(t, a, "ack")
			_ = readRevision(t, a, 1)
			_ = readRevision(t, b, 1)
		}(i)
	}
	wg.Wait()
	for _, name := range []string{"alpha", "beta"} {
		r := s.rooms[name]
		r.mu.Lock()
		if r.state == nil || r.state.Revision != 1 {
			t.Fatalf("room %s not started", name)
		}
		r.mu.Unlock()
	}
}

func TestLobbyCapacityAndIdleCleanup(t *testing.T) {
	s, server := testServer(t)
	base := s.now()
	s.now = func() time.Time { return base }
	first := joinRoom(t, server.URL, "capacity", "A", "", http.StatusOK)
	for _, name := range []string{"B", "C", "D"} {
		joinRoom(t, server.URL, "capacity", name, "", http.StatusOK)
	}
	joinRoom(t, server.URL, "capacity", "E", "", http.StatusConflict)
	duplicate := joinRoom(t, server.URL, "capacity", "Changed", first.Token, http.StatusOK)
	if duplicate.Seat != first.Seat {
		t.Fatal("duplicate join created a new seat")
	}
	s.now = func() time.Time { return base.Add(roomIdleTTL + time.Minute) }
	joinRoom(t, server.URL, "fresh", "F", "", http.StatusOK)
	s.mu.Lock()
	_, exists := s.rooms["capacity"]
	s.mu.Unlock()
	if exists {
		t.Fatal("idle room was not removed")
	}
}

func TestBlockedRoomDoesNotBlockAnother(t *testing.T) {
	s, server := testServer(t)
	for _, id := range []string{"slow", "fast"} {
		joinRoom(t, server.URL, id, "A", "", http.StatusOK)
		joinRoom(t, server.URL, id, "B", "", http.StatusOK)
	}
	s.mu.Lock()
	slow := s.rooms["slow"]
	s.mu.Unlock()
	slow.dispatchMu.Lock()
	defer slow.dispatchMu.Unlock()
	fastHost := joinRoom(t, server.URL, "fast", "A", s.rooms["fast"].seats[0].Token, http.StatusOK)
	fastClient := dialRoom(t, server.URL, fastHost)
	_ = readMessage(t, fastClient, "lobby")
	sendCommand(t, fastClient, command{Type: "start", ID: "start"})
	if got := readMessage(t, fastClient, "ack"); got.Revision != 1 {
		t.Fatalf("other room stalled: %+v", got)
	}
}

func TestOversizedCommandDisconnects(t *testing.T) {
	_, server := testServer(t)
	seat := joinRoom(t, server.URL, "limit", "A", "", http.StatusOK)
	conn := dialRoom(t, server.URL, seat)
	_ = readMessage(t, conn, "lobby")
	if err := conn.WriteMessage(websocket.TextMessage, bytes.Repeat([]byte("x"), 3000)); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("oversized command left connection open")
	}
}
