package gameui

import (
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/n0remac/trick-evolution/domain"
)

func TestTargetHTTPWebSocketResultAndReconnect(t *testing.T) {
	cfg := domain.DefaultConfig()
	cfg.HandSize, cfg.MaxHands, cfg.RevealEvery = 1, 1, 100
	cfg.InitialRules = []domain.RuleID{domain.RuleFirstTo5}
	s, err := NewServerWithConfig(cfg, func() domain.ShuffleSource { return identityShuffle{} })
	if err != nil {
		t.Fatal(err)
	}
	s.dealDelay = 0
	web := httptest.NewServer(s.Handler())
	t.Cleanup(web.Close)
	t.Cleanup(s.Close)
	host := joinRoom(t, web.URL, "target-ws", "Host", "", 200)
	guest := joinRoom(t, web.URL, "target-ws", "Guest", "", 200)
	a, b := dialRoom(t, web.URL, host), dialRoom(t, web.URL, guest)
	_ = readMessage(t, a, "lobby")
	_ = readMessage(t, b, "lobby")
	sendCommand(t, a, command{Type: "start", ID: "start"})
	_ = readMessage(t, a, "ack")
	views := [2]serverMessage{readRevision(t, a, 1), readRevision(t, b, 1)}
	reconnected := false
	for moves := 0; views[0].View.Public.Phase != domain.PhaseGameComplete && moves < 50; moves++ {
		public := views[0].View.Public
		if !reconnected && public.CompletedHands >= 1 && public.Phase == domain.PhasePlayingTrick {
			_ = a.Close()
			rejoined := joinRoom(t, web.URL, host.Room, host.Name, host.Token, 200)
			a = dialRoom(t, web.URL, rejoined)
			reconnect := readMessage(t, a, "state")
			if reconnect.View == nil || reconnect.View.Public.GameTarget != 5 || reconnect.View.Public.GameNumber != public.GameNumber || !reflect.DeepEqual(reconnect.View.Public.GameScores, public.GameScores) {
				t.Fatal("reconnect lost target interval progress")
			}
			views[0] = reconnect
			reconnected = true
		}
		if public.Phase == domain.PhaseHandComplete {
			views = [2]serverMessage{readRevision(t, a, public.Revision+1), readRevision(t, b, public.Revision+1)}
			continue
		}
		if public.Phase != domain.PhasePlayingTrick {
			t.Fatalf("unexpected phase %s", public.Phase)
		}
		index := 0
		if public.Turn == guest.Seat {
			index = 1
		}
		conn := a
		if index == 1 {
			conn = b
		}
		id := "move-" + string(rune('a'+moves))
		sendCommand(t, conn, command{Type: "play", ID: id, Card: views[index].View.Hand[0]})
		_ = readMessage(t, conn, "ack")
		views = [2]serverMessage{readRevision(t, a, public.Revision+1), readRevision(t, b, public.Revision+1)}
	}
	final := views[0].View.Public
	if final.Phase != domain.PhaseGameComplete || final.CompletedHands <= cfg.MaxHands || final.GameResult == nil || final.GameResult.Target != 5 || len(final.GameResult.Winners) == 0 {
		t.Fatalf("target result missing: phase %s, hands %d, result %+v", final.Phase, final.CompletedHands, final.GameResult)
	}
	if html := renderScreen(views[0], host.Seat, s.catalog, s.config); !strings.Contains(html, "first to 5") || !strings.Contains(html, "Target game") {
		t.Fatal("target progress or scorecard result missing from GoDom")
	}
	if !reconnected {
		t.Fatal("active target game was never reconnected")
	}
}
