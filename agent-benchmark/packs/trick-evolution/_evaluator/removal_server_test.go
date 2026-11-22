package gameui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/n0remac/trick-evolution/domain"
)

func TestBenchmarkRemovalCommandAndControl(t *testing.T) {
	s, err := NewServerWithBotOptions(domain.DefaultConfig(), func() domain.ShuffleSource { return identityShuffle{} }, BotOptions{Mode: BotModeHeuristic})
	if err != nil {
		t.Fatal(err)
	}
	s.botDelay, s.dealDelay = time.Hour, time.Hour
	web := httptest.NewServer(s.Handler())
	t.Cleanup(web.Close)
	t.Cleanup(s.Close)
	response, err := http.Post(web.URL+"/game/endless", "application/json", bytes.NewBufferString(`{"name":"Alice"}`))
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
	conn := dialRoom(t, web.URL, session)
	_ = readMessage(t, conn, "state")
	s.mu.Lock()
	r := s.rooms[session.Room]
	s.mu.Unlock()
	r.mu.Lock()
	state := *r.state
	state.Field = append(state.Field, domain.RuleInstance{InstanceID: "benchmark:expanded", DefinitionID: domain.RuleExpandedHand, Sequence: len(state.Field), IntroducedAfterHand: 3})
	state.Round, state.Phase, state.PendingBoundaryID = 4, domain.PhaseAwaitingChoice, "round:4:after:4"
	state.DraftOptions = []domain.RuleID{domain.RuleLastTrickBonus}
	r.state = &state
	r.mu.Unlock()
	r.publish()
	pending := readMessage(t, conn, "state")
	if pending.View == nil || pending.View.Public.PendingBoundaryID != state.PendingBoundaryID {
		t.Fatalf("draft not visible: %+v", pending)
	}
	html := renderScreen(pending, session.Seat, s.catalog, state.Config)
	if !strings.Contains(html, "Remove Expanded Hand") || !strings.Contains(html, "remove_rule") {
		t.Fatal("draft has no focused removal control")
	}
	sendCommand(t, conn, command{Type: "remove_rule", ID: "bad-removal", Rule: domain.RuleFollowSuit, BoundaryID: state.PendingBoundaryID})
	if got := readMessage(t, conn, "error"); got.Revision != state.Revision {
		t.Fatalf("rejected command changed revision: %+v", got)
	}
	r.mu.Lock()
	if r.state.Revision != state.Revision || r.state.Phase != domain.PhaseAwaitingChoice || r.state.Field[len(r.state.Field)-1].Removed {
		r.mu.Unlock()
		t.Fatal("ineligible server command changed the draft")
	}
	r.mu.Unlock()
	sendCommand(t, conn, command{Type: "remove_rule", ID: "good-removal", Rule: domain.RuleExpandedHand, BoundaryID: state.PendingBoundaryID})
	ack := readMessage(t, conn, "ack")
	if ack.Revision != state.Revision+1 {
		t.Fatalf("valid removal acknowledgement: %+v", ack)
	}
	settled := readMessage(t, conn, "state")
	if settled.Revision != state.Revision+1 || settled.View.Public.Phase != domain.PhaseHandComplete || settled.View.Public.PendingBoundaryID != "" {
		t.Fatalf("valid removal did not settle draft once: %+v", settled)
	}
	sendCommand(t, conn, command{Type: "remove_rule", ID: "repeat-removal", Rule: domain.RuleExpandedHand, BoundaryID: state.PendingBoundaryID})
	if got := readMessage(t, conn, "error"); got.Revision != settled.Revision {
		t.Fatalf("repeated removal changed revision: %+v", got)
	}
}
