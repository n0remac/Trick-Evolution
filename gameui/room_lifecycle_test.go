package gameui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/n0remac/trick-evolution/domain"
)

func leaveStatus(t *testing.T, base string, session testSession) int {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"room": session.Room, "seat": session.Seat, "token": session.Token})
	response, err := http.Post(base+"/game/leave", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	return response.StatusCode
}

func rejectOldSocket(t *testing.T, base string, session testSession, status int) {
	t.Helper()
	u, _ := url.Parse(base)
	address := "ws://" + u.Host + "/ws/game?room=" + url.QueryEscape(session.Room) + "&token=" + url.QueryEscape(session.Token)
	conn, response, err := websocket.DefaultDialer.Dial(address, nil)
	if conn != nil {
		conn.Close()
	}
	if err == nil || response == nil || response.StatusCode != status {
		t.Fatalf("old socket: %v (%v), want %d", err, response, status)
	}
}

func expectTerminal(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	if got := readMessage(t, conn, "room_closed"); got.Type != "room_closed" {
		t.Fatalf("terminal message: %+v", got)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := conn.ReadMessage()
	if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		t.Fatalf("socket did not close normally after terminal: %v", err)
	}
}

func TestLobbyLeaveReleasesSeatAndHostClosesRoom(t *testing.T) {
	s, web := testServer(t)
	host := joinRoom(t, web.URL, "lobby-leave", "Host", "", http.StatusOK)
	guest := joinRoom(t, web.URL, "lobby-leave", "Guest", "", http.StatusOK)
	a, b := dialRoom(t, web.URL, host), dialRoom(t, web.URL, guest)
	_ = readMessage(t, a, "lobby")
	_ = readMessage(t, b, "lobby")
	if got := leaveStatus(t, web.URL, guest); got != http.StatusOK {
		t.Fatalf("guest leave: %d", got)
	}
	expectTerminal(t, b)
	for i := 0; i < 4; i++ {
		lobby := readMessage(t, a, "lobby")
		if len(lobby.Seats) == 1 {
			break
		}
		if i == 3 {
			t.Fatalf("seat was not freed: %+v", lobby.Seats)
		}
	}
	joinRoom(t, web.URL, "lobby-leave", "Old guest", guest.Token, http.StatusUnauthorized)
	rejectOldSocket(t, web.URL, guest, http.StatusUnauthorized)
	replacement := joinRoom(t, web.URL, "lobby-leave", "Replacement", "", http.StatusOK)
	if replacement.Seat == guest.Seat {
		t.Fatal("replacement reused guest identity")
	}
	c := dialRoom(t, web.URL, replacement)
	_ = readMessage(t, c, "lobby")
	if got := leaveStatus(t, web.URL, host); got != http.StatusOK {
		t.Fatalf("host leave: %d", got)
	}
	expectTerminal(t, a)
	expectTerminal(t, c)
	if got := leaveStatus(t, web.URL, host); got != http.StatusGone {
		t.Fatalf("repeated leave: %d", got)
	}
	s.mu.Lock()
	_, exists := s.rooms["lobby-leave"]
	s.mu.Unlock()
	if exists {
		t.Fatal("host lobby remained registered")
	}
	fresh := joinRoom(t, web.URL, "lobby-leave", "Fresh", "", http.StatusOK)
	if fresh.Seat == host.Seat {
		t.Fatal("new room reused host identity")
	}
	joinRoom(t, web.URL, "lobby-leave", "Old host", host.Token, http.StatusUnauthorized)
	rejectOldSocket(t, web.URL, host, http.StatusUnauthorized)
	if got := leaveStatus(t, web.URL, host); got != http.StatusUnauthorized {
		t.Fatalf("stale leave changed reused room: %d", got)
	}
	s.mu.Lock()
	room := s.rooms["lobby-leave"]
	s.mu.Unlock()
	room.mu.Lock()
	seats := len(room.seats)
	room.mu.Unlock()
	if seats != 1 {
		t.Fatalf("stale leave changed new room seats: %d", seats)
	}
}

func TestActiveLeaveEndsMatchForEachSeat(t *testing.T) {
	for _, departingHost := range []bool{false, true} {
		t.Run(fmt.Sprint("host_", departingHost), func(t *testing.T) {
			s, web := testServer(t)
			code := fmt.Sprint("active-leave-", departingHost)
			host := joinRoom(t, web.URL, code, "Host", "", http.StatusOK)
			guest := joinRoom(t, web.URL, code, "Guest", "", http.StatusOK)
			a, b := dialRoom(t, web.URL, host), dialRoom(t, web.URL, guest)
			_ = readMessage(t, a, "lobby")
			_ = readMessage(t, b, "lobby")
			sendCommand(t, a, command{Type: "start", ID: "start"})
			_ = readMessage(t, a, "ack")
			_ = readRevision(t, a, 1)
			_ = readRevision(t, b, 1)
			departing := guest
			if departingHost {
				departing = host
			}
			oldRoom := s.rooms[code]
			if got := leaveStatus(t, web.URL, departing); got != http.StatusOK {
				t.Fatalf("active leave: %d", got)
			}
			for _, conn := range []*websocket.Conn{a, b} {
				final := readRevision(t, conn, 2)
				if final.View == nil || !final.View.Public.EndedEarly || final.View.Public.EndedBy != departing.Seat || final.View.Public.Phase != domain.PhaseGameComplete {
					t.Fatalf("wrong final result: %+v", final)
				}
				expectTerminal(t, conn)
			}
			s.mu.Lock()
			_, exists := s.rooms[code]
			s.mu.Unlock()
			if exists {
				t.Fatal("active room remained registered")
			}
			oldRoom.mu.Lock()
			stopped := oldRoom.ctx.Err() != nil
			oldRoom.mu.Unlock()
			if !stopped {
				t.Fatal("bot work was not canceled")
			}
			fresh := joinRoom(t, web.URL, code, "Fresh", "", http.StatusOK)
			if got := leaveStatus(t, web.URL, departing); got != http.StatusUnauthorized {
				t.Fatalf("stale leave: %d", got)
			}
			joinRoom(t, web.URL, code, "Old", departing.Token, http.StatusUnauthorized)
			rejectOldSocket(t, web.URL, departing, http.StatusUnauthorized)
			if fresh.Seat == departing.Seat {
				t.Fatal("fresh room reused departed seat")
			}
		})
	}
}

func TestTransientReconnectKeepsSeat(t *testing.T) {
	_, web := testServer(t)
	host := joinRoom(t, web.URL, "reconnect-life", "Host", "", http.StatusOK)
	guest := joinRoom(t, web.URL, "reconnect-life", "Guest", "", http.StatusOK)
	a, b := dialRoom(t, web.URL, host), dialRoom(t, web.URL, guest)
	_ = readMessage(t, a, "lobby")
	_ = readMessage(t, b, "lobby")
	sendCommand(t, a, command{Type: "start", ID: "start"})
	_ = readMessage(t, a, "ack")
	_ = readRevision(t, a, 1)
	first := readRevision(t, b, 1)
	b.Close()
	rejoined := joinRoom(t, web.URL, "reconnect-life", "Changed", guest.Token, http.StatusOK)
	if rejoined.Seat != guest.Seat || rejoined.Name != guest.Name {
		t.Fatal("reconnect changed seat identity")
	}
	b = dialRoom(t, web.URL, rejoined)
	restored := readRevision(t, b, 1)
	if len(restored.View.Hand) != len(first.View.Hand) {
		t.Fatal("reconnect lost private hand")
	}
}
