package gameui

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/n0remac/trick-evolution/domain"
)

func auctionServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	cfg := domain.DefaultConfig()
	cfg.HandSize, cfg.MaxHands = 1, 1
	cfg.InitialRules = []domain.RuleID{domain.RuleAuction, domain.RuleSetBidder}
	s, err := NewServerWithConfig(cfg, func() domain.ShuffleSource { return identityShuffle{} })
	if err != nil {
		t.Fatal(err)
	}
	s.botDelay = 10 * time.Millisecond
	web := httptest.NewServer(s.Handler())
	t.Cleanup(web.Close)
	t.Cleanup(s.Close)
	return s, web
}

func TestAuctionHTMXRejectsStaleRevision(t *testing.T) {
	_, web := auctionServer(t)
	host := joinRoom(t, web.URL, "auction-html", "Host", "", 200)
	guest := joinRoom(t, web.URL, "auction-html", "Guest", "", 200)
	a := dialRoom(t, web.URL, host)
	_ = readMessage(t, a, "lobby")
	b := dialRoom(t, web.URL, guest)
	_ = readMessage(t, b, "lobby")
	sendCommand(t, a, command{Type: "start", ID: "start"})
	_ = readMessage(t, a, "ack")
	_ = readRevision(t, a, 1)
	_ = readRevision(t, b, 1)
	_ = b.Close()
	u, _ := url.Parse(web.URL)
	address := "ws://" + u.Host + "/ws/game?room=" + url.QueryEscape(guest.Room) + "&token=" + url.QueryEscape(guest.Token) + "&format=html"
	html, _, err := websocket.DefaultDialer.Dial(address, http.Header{"Origin": []string{web.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer html.Close()
	_ = html.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := html.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	if err := html.WriteMessage(websocket.TextMessage, []byte(`{"type":"bid","id":"stale-bid","bid":"1","revision":"0"}`)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		_ = html.SetReadDeadline(time.Now().Add(3 * time.Second))
		_, data, err := html.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "Game changed. Use the updated controls.") {
			return
		}
	}
	t.Fatal("stale HTMX bid was not rejected")
}

func TestAuctionHTTPWebSocketReconnectAndScorecard(t *testing.T) {
	s, web := auctionServer(t)
	host := joinRoom(t, web.URL, "auction-ws", "Host", "", 200)
	guest := joinRoom(t, web.URL, "auction-ws", "Guest", "", 200)
	a, b := dialRoom(t, web.URL, host), dialRoom(t, web.URL, guest)
	_ = readMessage(t, a, "lobby")
	_ = readMessage(t, b, "lobby")
	sendCommand(t, a, command{Type: "start", ID: "start"})
	_ = readMessage(t, a, "ack")
	firstA, firstB := readRevision(t, a, 1), readRevision(t, b, 1)
	if firstA.View.Public.Phase != domain.PhaseAuction || firstA.View.Public.Turn != guest.Seat || firstA.View.Public.Auction.Min != 1 {
		t.Fatalf("auction projection: %+v", firstA.View.Public.Auction)
	}
	if strings.Contains(renderScreen(firstB, guest.Seat, s.catalog, s.config), "Your bid") == false {
		t.Fatal("bid controls missing")
	}
	sendCommand(t, a, command{Type: "bid", ID: "wrong-seat", Bid: 1})
	if got := readMessage(t, a, "error"); got.Code != string(domain.ErrNotYourTurn) {
		t.Fatalf("out of turn bid: %+v", got)
	}
	sendCommand(t, b, command{Type: "bid", ID: "too-high", Bid: 2})
	if got := readMessage(t, b, "error"); got.Code != string(domain.ErrInvalidBid) {
		t.Fatalf("illegal bid: %+v", got)
	}
	sendCommand(t, b, command{Type: "bid", ID: "bid-one", Bid: 1})
	_ = readMessage(t, b, "ack")
	secondA, secondB := readRevision(t, a, 2), readRevision(t, b, 2)
	if secondA.View.Public.Phase != domain.PhasePlayingTrick || secondA.View.Public.Auction.Contract.Bidder != guest.Seat {
		t.Fatal("contract missing from public state")
	}
	_ = a.Close()
	rejoined := joinRoom(t, web.URL, host.Room, host.Name, host.Token, 200)
	a = dialRoom(t, web.URL, rejoined)
	secondA = readMessage(t, a, "state")
	if secondA.View.Public.Auction.Contract.Bid != 1 || len(secondA.View.Hand) != 1 {
		t.Fatal("reconnect lost contract or private hand")
	}
	sendCommand(t, b, command{Type: "play", ID: "lead", Card: secondB.View.Hand[0]})
	thirdA := readRevision(t, a, 3)
	_ = readRevision(t, b, 3)
	sendCommand(t, a, command{Type: "play", ID: "follow", Card: thirdA.View.Hand[0]})
	_ = readMessage(t, a, "ack")
	final := readRevision(t, a, 4)
	if final.View.Public.Phase != domain.PhaseGameComplete || final.View.Public.LastScorecard.Contract.Penalty != 1 || final.View.Public.Seats[1].Score != -1 {
		t.Fatalf("contract settlement: %+v", final.View.Public.LastScorecard)
	}
	if html := renderScreen(final, host.Seat, s.catalog, s.config); !strings.Contains(html, "set penalty") {
		t.Fatal("scorecard omitted contract penalty")
	}
}

func TestAuctionBotProgressesFromPublicObservation(t *testing.T) {
	_, web := auctionServer(t)
	host := joinRoom(t, web.URL, "auction-bot", "Host", "", 200)
	a := dialRoom(t, web.URL, host)
	_ = readMessage(t, a, "lobby")
	sendCommand(t, a, command{Type: "add_bot", ID: "bot"})
	_ = readMessage(t, a, "ack")
	_ = readMessage(t, a, "lobby")
	sendCommand(t, a, command{Type: "start", ID: "start"})
	_ = readMessage(t, a, "ack")
	first := readRevision(t, a, 1)
	if first.View.Public.Phase != domain.PhaseAuction {
		t.Fatal("bot auction did not open")
	}
	second := readRevision(t, a, 2)
	if second.View.Public.Phase != domain.PhasePlayingTrick || second.View.Public.Auction.Contract.Bidder != "bot:1" {
		t.Fatalf("bot did not make legal bid: %+v", second.View.Public.Auction)
	}
	third := readRevision(t, a, 3)
	if third.View.Public.Turn != host.Seat {
		t.Fatal("bot did not lead after winning auction")
	}
	sendCommand(t, a, command{Type: "play", ID: "follow", Card: third.View.Hand[0]})
	_ = readMessage(t, a, "ack")
	final := readRevision(t, a, 4)
	if final.View.Public.Phase != domain.PhaseGameComplete {
		t.Fatal("bot auction game did not complete")
	}
}
