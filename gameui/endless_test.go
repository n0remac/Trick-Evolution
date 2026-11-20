package gameui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/n0remac/trick-evolution/domain"
)

func TestEndlessHomeToRunReconnectAndClosure(t *testing.T) {
	s, err := NewServerWithBotOptions(domain.DefaultConfig(), func() domain.ShuffleSource { return identityShuffle{} }, BotOptions{Mode: BotModeHeuristic})
	if err != nil {
		t.Fatal(err)
	}
	s.botDelay = 0
	s.dealDelay = 120 * time.Millisecond
	web := httptest.NewServer(s.Handler())
	t.Cleanup(web.Close)
	t.Cleanup(s.Close)
	page, err := http.Get(web.URL + "/game")
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	_, _ = body.ReadFrom(page.Body)
	_ = page.Body.Close()
	if !strings.Contains(body.String(), "Start Endless Mode") {
		t.Fatal("home screen has no Endless Mode option")
	}
	response, err := http.Post(web.URL+"/game/endless", "application/json", strings.NewReader(`{"name":"Runner"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("start: HTTP %d", response.StatusCode)
	}
	var session testSession
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if session.Room == "" || session.Token == "" {
		t.Fatalf("missing session: %+v", session)
	}
	conn := dialRoom(t, web.URL, session)
	message := readMessage(t, conn, "state")
	if message.View.Public.Mode != domain.ModeEndless || message.View.Public.DealSize != 3 || len(message.Seats) != 2 || !message.Seats[1].Bot {
		t.Fatalf("opening: %+v", message)
	}
	openingHTML := renderScreen(message, session.Seat, s.catalog, domain.DefaultConfig())
	for _, text := range []string{"Run totals", "Round Pacing", "End run", "Share live game"} {
		if !strings.Contains(openingHTML, text) {
			t.Fatalf("opening screen missing %q", text)
		}
	}
	serial, drafts, tables, holds := 0, 0, 0, 0
	soloPaused, reviewRestored, keepSkipped := false, false, false
	expectKeepSkip := false
	noticeReconnect, gameReconnect, resultReconnect := false, false, false
	for message.View.Public.CompletedHands < 12 {
		public := message.View.Public
		if public.Round == 2 && public.RoundHand == 1 && public.DealSize != 5 {
			t.Fatalf("round two dealt %d cards, want five", public.DealSize)
		}
		switch public.Phase {
		case domain.PhasePlayingTrick:
			if public.Turn == session.Seat {
				legal, err := domain.LegalCards(*message.View, s.catalog)
				if err != nil || len(legal) == 0 {
					t.Fatalf("legal: %v %v", legal, err)
				}
				serial++
				sendCommand(t, conn, command{Type: "play", ID: fmt.Sprintf("play-%d", serial), Card: legal[0]})
			}
		case domain.PhaseAwaitingChoice:
			drafts++
			time.Sleep(2 * s.dealDelay)
			s.mu.Lock()
			pendingRoom := s.rooms[session.Room]
			s.mu.Unlock()
			pendingRoom.mu.Lock()
			stillDraft := pendingRoom.state.Revision == public.Revision && pendingRoom.state.Phase == domain.PhaseAwaitingChoice
			pendingRoom.mu.Unlock()
			if !stillDraft {
				t.Fatal("automatic deal crossed a pending draft")
			}
			draftHTML := renderScreen(message, session.Seat, s.catalog, domain.DefaultConfig())
			for _, text := range []string{"draft-choices", public.PendingBoundaryID, "Keep current rules", "Run totals"} {
				if !strings.Contains(draftHTML, text) {
					t.Fatalf("draft screen missing %q", text)
				}
			}
			if drafts == 1 {
				_ = conn.Close()
				conn = dialRoom(t, web.URL, session)
				message = readMessage(t, conn, "state")
				if message.View.Public.PendingBoundaryID != public.PendingBoundaryID || message.View.Public.CompletedHands != public.CompletedHands {
					t.Fatal("reconnect lost pending draft")
				}
				s.mu.Lock()
				count := len(s.rooms[session.Room].seats)
				s.mu.Unlock()
				if count != len(public.Seats) {
					t.Fatalf("reconnect created seats: %d", count)
				}
			}
			choice := domain.RuleID("")
			if drafts == 1 {
				choice = public.DraftOptions[0]
			} else {
				expectKeepSkip = true
			}
			serial++
			sendCommand(t, conn, command{Type: "choose_rule", ID: fmt.Sprintf("draft-%d", serial), Rule: choice, BoundaryID: public.PendingBoundaryID})
		case domain.PhaseHandComplete:
			if expectKeepSkip {
				if message.ReviewRule != "" || strings.Contains(renderScreen(message, session.Seat, s.catalog, domain.DefaultConfig()), "new-rule-overlay") {
					t.Fatal("Keep current rules opened a rule review")
				}
				keepSkipped, expectKeepSkip = true, false
			}
			if !soloPaused && public.CompletedHands == 1 {
				soloPaused = true
				_ = conn.Close()
				for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
					s.mu.Lock()
					pendingRoom := s.rooms[session.Room]
					s.mu.Unlock()
					pendingRoom.mu.Lock()
					offline := pendingRoom.clients[session.Seat] == nil
					pendingRoom.mu.Unlock()
					if offline {
						break
					}
				}
				time.Sleep(2 * s.dealDelay)
				s.mu.Lock()
				pendingRoom := s.rooms[session.Room]
				s.mu.Unlock()
				pendingRoom.mu.Lock()
				paused := pendingRoom.state.Revision == public.Revision && pendingRoom.state.Phase == domain.PhaseHandComplete
				pendingRoom.mu.Unlock()
				if !paused {
					t.Fatal("solo run dealt while disconnected")
				}
				conn = dialRoom(t, web.URL, session)
				message = readMessage(t, conn, "state")
			}
			if message.ReviewRule != "" {
				if reviewRestored {
					t.Fatal("rule review repeated")
				}
				reviewRestored = true
				html := renderScreen(message, session.Seat, s.catalog, domain.DefaultConfig())
				definition, _ := s.catalog.Definition(message.ReviewRule)
				for _, text := range []string{"A new rule is being added", definition.Title, definition.Text, "Continue"} {
					if !strings.Contains(html, text) {
						t.Fatalf("pending rule review missing %q", text)
					}
				}
				_ = conn.Close()
				conn = dialRoom(t, web.URL, session)
				message = readMessage(t, conn, "state")
				if message.ReviewRule != definition.ID {
					t.Fatal("reconnect lost pending rule review")
				}
				time.Sleep(2 * s.dealDelay)
				s.mu.Lock()
				pendingRoom := s.rooms[session.Room]
				s.mu.Unlock()
				pendingRoom.mu.Lock()
				paused := pendingRoom.state.Revision == public.Revision && pendingRoom.state.Phase == domain.PhaseHandComplete
				pendingRoom.mu.Unlock()
				if !paused {
					t.Fatal("automatic deal crossed pending rule review")
				}
				serial++
				continueCommand := command{Type: "continue_rule", ID: fmt.Sprintf("continue-%d", serial)}
				sendCommand(t, conn, continueCommand)
				sendCommand(t, conn, continueCommand)
			}
		default:
			t.Fatalf("unexpected phase %s", public.Phase)
		}
		previous := public.Revision
		for {
			message = readMessage(t, conn, "state")
			if message.Revision > previous {
				break
			}
		}
		if strings.Contains(message.View.Public.TableCue, "The table added") || strings.Contains(message.View.Public.TableCue, "The table retired") {
			tables++
		}
		if strings.Contains(message.View.Public.TableCue, "rules stay the same") {
			holds++
		}
		public = message.View.Public
		var reconnect bool
		switch {
		case public.Round == 3 && !noticeReconnect && strings.Contains(public.TableCue, "A three-round game starts after this round"):
			noticeReconnect, reconnect = true, true
		case public.ShortGameRound > 0 && !gameReconnect:
			gameReconnect, reconnect = true, true
		case public.ShortGameResult != nil && !resultReconnect:
			resultReconnect, reconnect = true, true
		}
		if reconnect {
			before := public
			_ = conn.Close()
			conn = dialRoom(t, web.URL, session)
			message = readMessage(t, conn, "state")
			after := message.View.Public
			if after.Round != before.Round || after.ShortGameRound != before.ShortGameRound || after.TableCue != before.TableCue || (after.ShortGameResult == nil) != (before.ShortGameResult == nil) {
				t.Fatal("reconnect lost Endless event")
			}
			if resultReconnect && after.ShortGameResult != nil {
				html := renderScreen(message, session.Seat, s.catalog, domain.DefaultConfig())
				if !strings.Contains(html, `data-result-number="1"`) || !strings.Contains(html, "Three-round game 1 complete") || !strings.Contains(html, "Latest game 1") || !strings.Contains(html, "Winner:") && !strings.Contains(html, "Tie:") {
					t.Fatal("result is not readable alongside open play")
				}
			}
		}
	}
	if message.View.Public.Phase == domain.PhaseGameComplete || drafts < 2 || tables == 0 || holds == 0 || !noticeReconnect || !gameReconnect || !resultReconnect || !soloPaused || !reviewRestored || !keepSkipped {
		t.Fatalf("progression: hands=%d drafts=%d tables=%d holds=%d", message.View.Public.CompletedHands, drafts, tables, holds)
	}
	total := 0
	for _, score := range message.View.Public.RunScores {
		total += score
	}
	if total <= 0 || len(message.View.Public.RunScores) != 4 {
		t.Fatalf("run score ledger = %+v", message.View.Public.RunScores)
	}
	sendCommand(t, conn, command{Type: "end", ID: "end-run"})
	final := readMessage(t, conn, "state")
	if final.View.Public.Phase != domain.PhaseGameComplete || !final.View.Public.EndedEarly {
		t.Fatalf("end run result: %+v", final.View.Public)
	}
	_ = readMessage(t, conn, "room_closed")
	joinRoom(t, web.URL, session.Room, session.Name, session.Token, http.StatusGone)
}

func TestEndlessShareLinkJoinsExactLiveHand(t *testing.T) {
	s, web := testServer(t)
	s.botDelay = time.Hour
	response, err := http.Post(web.URL+"/game/endless", "application/json", strings.NewReader(`{"name":"Runner"}`))
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("start: %d", response.StatusCode)
	}
	var host testSession
	if err := json.NewDecoder(response.Body).Decode(&host); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if host.Invite == "" || host.Invite == host.Token {
		t.Fatal("missing separate invitation token")
	}
	hostConn := dialRoom(t, web.URL, host)
	opening := readMessage(t, hostConn, "state")
	if opening.View.Public.DealSize != 3 {
		t.Fatal("opening hand was not three cards")
	}
	if !strings.Contains(renderScreen(opening, host.Seat, s.catalog, s.config), "Share live game") {
		t.Fatal("share control missing")
	}
	postInvite := func(room, invite, name string) (*http.Response, error) {
		body, _ := json.Marshal(map[string]string{"room": room, "invite": invite, "name": name})
		return http.Post(web.URL+"/game/endless/join", "application/json", bytes.NewReader(body))
	}
	for _, tc := range []struct {
		room, invite string
		status       int
	}{
		{host.Room, "bad", http.StatusBadRequest},
		{host.Room, strings.Repeat("0", 64), http.StatusForbidden},
		{"endless-expired", host.Invite, http.StatusGone},
	} {
		res, err := postInvite(tc.room, tc.invite, "Friend")
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != tc.status {
			t.Fatalf("invite status: got %d want %d", res.StatusCode, tc.status)
		}
	}
	joinRoom(t, web.URL, host.Room, "Uninvited", "", http.StatusConflict)
	res, err := postInvite(host.Room, host.Invite, "Friend")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("invited join: %d", res.StatusCode)
	}
	var guest testSession
	if err := json.NewDecoder(res.Body).Decode(&guest); err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if guest.Room != host.Room || guest.Seat == host.Seat || guest.Token == host.Token || guest.Invite != host.Invite {
		t.Fatalf("guest session: %+v", guest)
	}
	guestConn := dialRoom(t, web.URL, guest)
	view := readMessage(t, guestConn, "state")
	if len(view.View.Hand) != 3 || view.View.Public.Turn != guest.Seat || view.Seats[1].Bot || view.Seats[1].Name != "Friend" {
		t.Fatalf("guest did not take bot's live seat: %+v", view)
	}
	legal, err := domain.LegalCards(*view.View, s.catalog)
	if err != nil || len(legal) == 0 {
		t.Fatalf("guest legal cards: %v %v", legal, err)
	}
	sendCommand(t, guestConn, command{Type: "play", ID: "friend-play", Card: legal[0]})
	if next := readRevision(t, guestConn, view.Revision+1); next.View.Public.Revision != view.Revision+1 {
		t.Fatal("guest could not play")
	}
	res, err = postInvite(host.Room, host.Invite, "Another")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusConflict {
		t.Fatalf("full table: %d", res.StatusCode)
	}
	reconnected := joinRoom(t, web.URL, guest.Room, guest.Name, guest.Token, http.StatusOK)
	if reconnected.Seat != guest.Seat || reconnected.Invite != guest.Invite {
		t.Fatal("guest reconnect changed seat or invitation")
	}
	first := joinRoom(t, web.URL, "ordinary-room", "Alice", "", http.StatusOK)
	second := joinRoom(t, web.URL, "ordinary-room", "Bob", "", http.StatusOK)
	if first.Room != second.Room || first.Invite != "" || second.Invite != "" {
		t.Fatal("ordinary room links changed")
	}
}
