package gameui

import (
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/n0remac/trick-evolution/domain"
)

func TestAutomaticDealRegularAndIdempotentBoundary(t *testing.T) {
	config := domain.DefaultConfig()
	config.HandSize = 1
	s, err := NewServerWithConfig(config, func() domain.ShuffleSource { return identityShuffle{} })
	if err != nil {
		t.Fatal(err)
	}
	s.dealDelay = 300 * time.Millisecond
	web := httptest.NewServer(s.Handler())
	t.Cleanup(web.Close)
	t.Cleanup(s.Close)
	host := joinRoom(t, web.URL, "auto-deal", "Alice", "", 200)
	guest := joinRoom(t, web.URL, "auto-deal", "Bob", "", 200)
	a, b := dialRoom(t, web.URL, host), dialRoom(t, web.URL, guest)
	_ = readMessage(t, a, "lobby")
	_ = readMessage(t, b, "lobby")
	sendCommand(t, a, command{Type: "start", ID: "start"})
	aState, bState := readRevision(t, a, 1), readRevision(t, b, 1)
	playHand := func() {
		t.Helper()
		for i := 0; i < 2; i++ {
			turn := aState.View.Public.Turn
			conn, view := a, aState.View
			if turn == guest.Seat {
				conn, view = b, bState.View
			}
			if len(view.Hand) != 1 {
				t.Fatalf("hand count for %s: %d", turn, len(view.Hand))
			}
			sendCommand(t, conn, command{Type: "play", ID: fmt.Sprintf("play-%d", aState.Revision), Card: view.Hand[0]})
			nextRevision := aState.Revision + 1
			aState, bState = readRevision(t, a, nextRevision), readRevision(t, b, nextRevision)
		}
		if aState.View.Public.Phase != domain.PhaseHandComplete {
			t.Fatalf("phase after trick: %s", aState.View.Public.Phase)
		}
	}
	playHand()
	time.Sleep(s.dealDelay / 3)
	s.mu.Lock()
	room := s.rooms[host.Room]
	s.mu.Unlock()
	room.mu.Lock()
	held := room.state.Revision == 3 && room.state.Phase == domain.PhaseHandComplete
	room.mu.Unlock()
	if !held {
		t.Fatal("next hand appeared before the collection interval")
	}
	aState, bState = readRevision(t, a, 4), readRevision(t, b, 4)
	if aState.View.Public.HandNumber != 2 || bState.View.Public.HandNumber != 2 {
		t.Fatal("automatic next hand missing")
	}
	playHand()
	// A legacy manual command racing the timer consumes the boundary once.
	sendCommand(t, a, command{Type: "next", ID: "legacy-next"})
	aState, bState = readRevision(t, a, 7), readRevision(t, b, 7)
	time.Sleep(2 * s.dealDelay)
	room.mu.Lock()
	once := room.state.Revision == 7 && room.state.HandNumber == 3
	room.mu.Unlock()
	if !once {
		t.Fatal("timer dealt a second hand after manual progression")
	}
	playHand()
	sendCommand(t, a, command{Type: "end", ID: "end-at-boundary"})
	aState = readRevision(t, a, 10)
	if aState.View.Public.Phase != domain.PhaseGameComplete {
		t.Fatal("end at boundary did not complete game")
	}
	time.Sleep(2 * s.dealDelay)
	room.mu.Lock()
	closed := room.closed && room.state.Revision == 10 && room.state.Phase == domain.PhaseGameComplete
	room.mu.Unlock()
	if !closed {
		t.Fatal("closed room automatically dealt")
	}
}
