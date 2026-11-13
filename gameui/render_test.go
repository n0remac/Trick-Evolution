package gameui

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/n0remac/trick-evolution/domain"
)

func TestGoDomScreensKeepHandsPrivateAndUseDomainLegality(t *testing.T) {
	catalog := domain.DefaultCatalog()
	state, err := domain.NewMatch([]domain.SeatID{"alice", "bob"}, domain.DefaultConfig(), catalog, identityShuffle{})
	if err != nil {
		t.Fatal(err)
	}
	alice, err := domain.ProjectForSeat(state, catalog, "alice")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := domain.ProjectForSeat(state, catalog, "bob")
	if err != nil {
		t.Fatal(err)
	}
	seats := []seatSummary{{ID: "alice", Name: "Alice", Host: true}, {ID: "bob", Name: "Bob"}}
	aliceHTML := renderScreen(serverMessage{Type: "state", Room: "test", View: &alice, Seats: seats, Sequence: 1}, "alice", catalog)
	bobHTML := renderScreen(serverMessage{Type: "state", Room: "test", View: &bob, Seats: seats, Sequence: 1}, "bob", catalog)
	for _, card := range bob.Hand {
		if strings.Contains(aliceHTML, string(card)) {
			t.Fatalf("Bob's private card %s in Alice HTML", card)
		}
	}
	for _, card := range alice.Hand {
		if strings.Contains(bobHTML, string(card)) {
			t.Fatalf("Alice's private card %s in Bob HTML", card)
		}
	}
	if !strings.Contains(aliceHTML, `hx-swap-oob="outerHTML"`) || !strings.Contains(aliceHTML, `data-update="1"`) {
		t.Fatal("HTML snapshot lacks swap and update markers")
	}

	rules, err := domain.ResolveField([]domain.RuleInstance{{InstanceID: "follow", DefinitionID: domain.RuleFollowSuit}}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	view := domain.PlayerView{Seat: "alice", Hand: []domain.CardID{"std:H:03", "std:S:02"}, Public: domain.PublicView{
		Phase: domain.PhasePlayingTrick, Turn: "alice", Revision: 7, HandNumber: 1, MaxHands: 10,
		CurrentTrick: []domain.Play{{Seat: "bob", Card: "std:H:09"}}, Rules: rules,
	}}
	html := renderScreen(serverMessage{Type: "state", Room: "test", View: &view, Seats: seats, Sequence: 9, Revision: 7}, "alice", catalog)
	if !regexp.MustCompile(`<button[^>]*disabled[^>]*>2♠</button>`).MatchString(html) {
		t.Fatal("off-suit card was enabled under Follow Suit")
	}
	if regexp.MustCompile(`<button[^>]*disabled[^>]*>3♥</button>`).MatchString(html) {
		t.Fatal("legal follow-suit card was disabled")
	}
	if !strings.Contains(html, "Follow suit: play a card matching the lead suit.") {
		t.Fatal("legal-action explanation missing")
	}
	if !strings.Contains(html, `name="revision"`) || !strings.Contains(html, `value="7"`) {
		t.Fatal("forms lack expected revision")
	}
}

func dialHTMLRoom(t *testing.T, base string, session testSession) *websocket.Conn {
	t.Helper()
	u, _ := url.Parse(base)
	address := "ws://" + u.Host + "/ws/game?room=" + url.QueryEscape(session.Room) + "&token=" + url.QueryEscape(session.Token) + "&format=html"
	conn, response, err := websocket.DefaultDialer.Dial(address, http.Header{"Origin": []string{base}})
	if err != nil {
		t.Fatalf("HTML WS dial: %v (%v)", err, response)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func readHTMLContaining(t *testing.T, conn *websocket.Conn, needle string) string {
	t.Helper()
	for i := 0; i < 12; i++ {
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), needle) {
			return string(data)
		}
	}
	t.Fatalf("HTML message containing %q not received", needle)
	return ""
}

func htmlUpdate(t *testing.T, body string) uint64 {
	t.Helper()
	found := regexp.MustCompile(`data-update="([0-9]+)"`).FindStringSubmatch(body)
	if found == nil {
		t.Fatal("missing update sequence")
	}
	value, err := strconv.ParseUint(found[1], 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestHTMLWebSocketEnvelopeRejectsStaleActionsAndResyncs(t *testing.T) {
	server, web := testServer(t)
	host := joinRoom(t, web.URL, "html-room", "Alice", "", http.StatusOK)
	guest := joinRoom(t, web.URL, "html-room", "Bob", "", http.StatusOK)
	hostConn := dialHTMLRoom(t, web.URL, host)
	guestConn := dialHTMLRoom(t, web.URL, guest)
	lobby := readHTMLContaining(t, hostConn, `data-kind="lobby"`)
	if !strings.Contains(lobby, "Start game") {
		t.Fatal("host controls missing")
	}
	_ = readHTMLContaining(t, guestConn, `data-kind="lobby"`)
	sendCommand(t, hostConn, map[string]any{"type": "start", "id": "html-start", "revision": "0", "HEADERS": map[string]string{"HX-Request": "true"}})
	first := readHTMLContaining(t, hostConn, `data-kind="state"`)
	_ = readHTMLContaining(t, guestConn, `data-kind="state"`)
	if !strings.Contains(first, "Your hand") || !strings.Contains(first, "Revision 1") {
		t.Fatal("start did not render first hand")
	}
	sendCommand(t, hostConn, map[string]any{"type": "play", "id": "stale-play", "card": "std:H:02", "revision": "0", "HEADERS": map[string]string{}})
	rejected := readHTMLContaining(t, hostConn, "Game changed. Use the updated controls.")
	if !strings.Contains(rejected, `id="notice"`) {
		t.Fatal("rejection lacks visible feedback")
	}
	gameRoom := server.rooms["html-room"]
	gameRoom.mu.Lock()
	revision := gameRoom.state.Revision
	gameRoom.mu.Unlock()
	if revision != 1 {
		t.Fatalf("stale action advanced revision to %d", revision)
	}
	sendCommand(t, hostConn, map[string]any{"type": "play", "id": "forged", "card": "std:H:02", "revision": "1", "seat": "bob"})
	if !strings.Contains(readHTMLContaining(t, hostConn, "invalid command"), `id="notice"`) {
		t.Fatal("forged envelope was not rejected")
	}
	_ = guestConn.Close()
	rejoined := dialHTMLRoom(t, web.URL, guest)
	restored := readHTMLContaining(t, rejoined, `data-kind="state"`)
	if !strings.Contains(restored, "Your hand") || !strings.Contains(restored, "Revision 1") || htmlUpdate(t, restored) <= htmlUpdate(t, first) {
		t.Fatal("reconnect did not receive a fresh private projection")
	}
	// Legacy JSON remains strict even though HTML messages accept HEADERS.
	raw := dialRoom(t, web.URL, host)
	_ = readMessage(t, raw, "state")
	data, _ := json.Marshal(map[string]any{"type": "next", "id": "raw-headers", "HEADERS": map[string]string{}})
	if err := raw.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatal(err)
	}
	if got := readMessage(t, raw, "error"); got.Code != "bad_command" {
		t.Fatalf("raw command accepted HTMX envelope: %+v", got)
	}
}
