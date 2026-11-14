package gameui

import (
	"net/http"
	"strings"
	"testing"

	"github.com/n0remac/trick-evolution/domain"
)

func TestRulesReviewLiveHostStartWhileGuestReads(t *testing.T) {
	server, web := testServer(t)
	host := joinRoom(t, web.URL, "rules-live", "Alice", "", http.StatusOK)
	guest := joinRoom(t, web.URL, "rules-live", "Bob", "", http.StatusOK)
	hostConn := dialHTMLRoom(t, web.URL, host)
	guestConn := dialHTMLRoom(t, web.URL, guest)
	hostLobby := readHTMLContaining(t, hostConn, `data-kind="lobby"`)
	guestLobby := readHTMLContaining(t, guestConn, `data-kind="lobby"`)
	if !strings.Contains(hostLobby, `data-action="review-start"`) || !strings.Contains(hostLobby, `id="review-start-form"`) || strings.Contains(guestLobby, `id="review-start-form"`) || !strings.Contains(guestLobby, "Alice is the host") {
		t.Fatal("host/guest review controls disagree")
	}
	gameRoom := server.rooms["rules-live"]
	gameRoom.mu.Lock()
	initial := gameRoom.state
	gameRoom.mu.Unlock()
	if initial != nil {
		t.Fatal("lobby review started a game")
	}
	sendCommand(t, guestConn, map[string]any{"type": "start", "id": "guest-start", "revision": "0"})
	_ = readHTMLContaining(t, guestConn, "only the host can start")
	gameRoom.mu.Lock()
	afterGuest := gameRoom.state
	gameRoom.mu.Unlock()
	if afterGuest != nil {
		t.Fatal("guest started match")
	}
	sendCommand(t, hostConn, map[string]any{"type": "start", "id": "host-start", "revision": "0"})
	_ = readHTMLContaining(t, hostConn, `data-kind="state"`)
	guestState := readHTMLContaining(t, guestConn, `data-kind="state"`)
	if !strings.Contains(guestState, "Back to game") || strings.Contains(guestState, `id="review-start-form"`) || !strings.Contains(guestState, `id="review-panel-current"`) || !strings.Contains(guestState, "Follow Suit") || !strings.Contains(guestState, "High Card Wins") {
		t.Fatal("guest's open review cannot switch to current game rules")
	}
	gameRoom.mu.Lock()
	state := *gameRoom.state
	gameRoom.mu.Unlock()
	if state.Revision != 1 {
		t.Fatalf("start revision %d", state.Revision)
	}
	for _, card := range state.Hands[domain.SeatID(host.Seat)] {
		if strings.Contains(guestState, string(card)) {
			t.Fatalf("host private card %s leaked in guest review", card)
		}
	}
	_ = guestConn.Close()
	reconnected := dialHTMLRoom(t, web.URL, guest)
	restored := readHTMLContaining(t, reconnected, `data-kind="state"`)
	if !strings.Contains(restored, "Back to game") || !strings.Contains(restored, `id="review-panel-current"`) || strings.Contains(restored, `id="review-start-form"`) {
		t.Fatal("reconnect restored stale lobby review")
	}
}
