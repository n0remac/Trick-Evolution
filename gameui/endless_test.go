package gameui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/n0remac/trick-evolution/domain"
)

func TestEndlessHomeToRunReconnectAndClosure(t *testing.T) {
	s, err := NewServerWithBotOptions(domain.DefaultConfig(), func() domain.ShuffleSource { return identityShuffle{} }, BotOptions{Mode: BotModeHeuristic})
	if err != nil {
		t.Fatal(err)
	}
	s.botDelay = 0
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
	for _, text := range []string{"Hand countdown:", "Run Score", "Round Pacing", "End run"} {
		if !strings.Contains(openingHTML, text) {
			t.Fatalf("opening screen missing %q", text)
		}
	}
	serial, drafts, tables, holds := 0, 0, 0, 0
	for message.View.Public.CompletedHands < 12 {
		public := message.View.Public
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
			draftHTML := renderScreen(message, session.Seat, s.catalog, domain.DefaultConfig())
			for _, text := range []string{"draft-choices", public.PendingBoundaryID, "Keep current rules", "Run Score"} {
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
				if count != 2 {
					t.Fatalf("reconnect created seats: %d", count)
				}
			}
			choice := domain.RuleID("")
			if drafts == 1 {
				choice = domain.RuleLastTrickBonus
			}
			serial++
			sendCommand(t, conn, command{Type: "choose_rule", ID: fmt.Sprintf("draft-%d", serial), Rule: choice, BoundaryID: public.PendingBoundaryID})
		case domain.PhaseHandComplete:
			serial++
			sendCommand(t, conn, command{Type: "next", ID: fmt.Sprintf("next-%d", serial)})
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
	}
	if message.View.Public.Phase == domain.PhaseGameComplete || drafts < 2 || tables == 0 || holds == 0 {
		t.Fatalf("progression: hands=%d drafts=%d tables=%d holds=%d", message.View.Public.CompletedHands, drafts, tables, holds)
	}
	total := 0
	for _, seat := range message.View.Public.Seats {
		total += seat.Score
	}
	if total != 67 {
		t.Fatalf("score = %d, want 67", total)
	}
	sendCommand(t, conn, command{Type: "end", ID: "end-run"})
	final := readMessage(t, conn, "state")
	if final.View.Public.Phase != domain.PhaseGameComplete || !final.View.Public.EndedEarly {
		t.Fatalf("end run result: %+v", final.View.Public)
	}
	_ = readMessage(t, conn, "room_closed")
	joinRoom(t, web.URL, session.Room, session.Name, session.Token, http.StatusGone)
}
