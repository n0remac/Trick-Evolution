package gameui

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/n0remac/trick-evolution/domain"
)

func startSoloForMerge(t *testing.T, base, name string) testSession {
	t.Helper()
	response, err := http.Post(base+"/game/endless", "application/json", strings.NewReader(`{"name":"`+name+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("start: %d", response.StatusCode)
	}
	var session testSession
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	return session
}

func parkAtOpenBoundary(s *Server, roomID string, score int, handSize int) {
	s.mu.Lock()
	r := s.rooms[roomID]
	r.mu.Lock()
	st := *r.state
	st.Round, st.RoundHand, st.Phase = 3, 0, domain.PhaseHandComplete
	st.CompletedHands, st.HandNumber = 4, 4
	st.Seats[0].Score = score
	st.RunScores[st.Seats[0].ID] = score
	st.Hands = map[domain.SeatID][]domain.CardID{}
	st.Retained = nil
	st.Config.HandSize = handSize
	r.config.HandSize = handSize
	st.Revision++
	r.state = &st
	r.mu.Unlock()
	s.mu.Unlock()
}

func TestEndlessConcurrentTransferAndReconnect(t *testing.T) {
	s, web := testServer(t)
	s.botDelay = time.Hour
	a := startSoloForMerge(t, web.URL, "A")
	b := startSoloForMerge(t, web.URL, "B")
	ca, cb := dialRoom(t, web.URL, a), dialRoom(t, web.URL, b)
	_ = readMessage(t, ca, "state")
	_ = readMessage(t, cb, "state")
	parkAtOpenBoundary(s, a.Room, 11, 3)
	parkAtOpenBoundary(s, b.Room, 17, 3)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.reconcileEndless() }()
	}
	wg.Wait()
	movedA, movedB := readMessage(t, ca, "room_moved"), readMessage(t, cb, "room_moved")
	if movedA.Room == "" || movedA.Room != movedB.Room || movedA.Token != a.Token || movedB.Token != b.Token {
		t.Fatalf("moves: %+v %+v", movedA, movedB)
	}
	ra := joinRoom(t, web.URL, a.Room, a.Name, a.Token, http.StatusOK)
	rb := joinRoom(t, web.URL, b.Room, b.Name, b.Token, http.StatusOK)
	if ra.Room != movedA.Room || rb.Room != movedA.Room || ra.Seat != a.Seat || rb.Seat != b.Seat || ra.RunID != a.Room || rb.RunID != b.Room {
		t.Fatalf("routing: %+v %+v", ra, rb)
	}
	na, nb := dialRoom(t, web.URL, ra), dialRoom(t, web.URL, rb)
	va, vb := readMessage(t, na, "state"), readMessage(t, nb, "state")
	if va.View.Public.Phase != domain.PhasePlayingTrick || va.View.Public.RoundLength != 3 || len(va.Seats) != 4 || va.View.Public.RunScores[a.Seat] != 11 || va.View.Public.RunScores[b.Seat] != 17 {
		t.Fatalf("shared state: %+v", va.View.Public)
	}
	if len(va.View.Hand) != 3 || len(vb.View.Hand) != 3 || va.View.Seat != a.Seat || vb.View.Seat != b.Seat {
		t.Fatal("private hands or seats lost")
	}
	for _, card := range va.View.Hand {
		for _, other := range vb.View.Hand {
			if card == other {
				t.Fatal("private hands overlap")
			}
		}
	}
	s.mu.Lock()
	count := len(s.rooms)
	s.mu.Unlock()
	if count != 1 {
		t.Fatalf("expected one shared room, got %d", count)
	}
	hostSession, survivor, survivorConn := a, b, nb
	if va.Seats[0].ID == b.Seat {
		hostSession, survivor, survivorConn = b, a, na
	}
	leaveBody := strings.NewReader(`{"room":"` + ra.Room + `","seat":"` + string(hostSession.Seat) + `","token":"` + hostSession.Token + `"}`)
	response, err := http.Post(web.URL+"/game/leave", "application/json", leaveBody)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("departure: %d", response.StatusCode)
	}
	s.mu.Lock()
	shared := s.rooms[ra.Room]
	shared.mu.Lock()
	departedOK := !shared.closed && shared.seats[0].Bot && shared.seats[1].ID == survivor.Seat
	departedSeats := append([]participant(nil), shared.seats...)
	shared.state.Phase = domain.PhaseHandComplete
	shared.state.Hands = map[domain.SeatID][]domain.CardID{}
	shared.state.Retained = nil
	shared.state.CurrentTrick = nil
	shared.state.Revision++
	before := shared.state.Revision
	shared.mu.Unlock()
	s.mu.Unlock()
	if !departedOK {
		t.Fatalf("departure did not return seat to a bot: %+v", departedSeats)
	}
	sendCommand(t, survivorConn, command{Type: "next", ID: "survivor-next"})
	if got := readRevision(t, survivorConn, before+1); got.View.Public.RoundHand != 2 {
		t.Fatalf("survivor could not continue: %+v", got.View.Public)
	}
	joinRoom(t, web.URL, hostSession.Room, hostSession.Name, hostSession.Token, http.StatusUnauthorized)
	rejoined := joinRoom(t, web.URL, survivor.Room, survivor.Name, survivor.Token, http.StatusOK)
	if !rejoined.Host || rejoined.RunID != survivor.Room {
		t.Fatalf("survivor reconnect: %+v", rejoined)
	}
}

func TestEndlessAutomaticDealAtSharedRoundBoundaryKeepsBothHumans(t *testing.T) {
	s, web := testServer(t)
	s.botDelay = time.Hour
	s.dealDelay = 80 * time.Millisecond
	a := startSoloForMerge(t, web.URL, "A")
	b := startSoloForMerge(t, web.URL, "B")
	ca, cb := dialRoom(t, web.URL, a), dialRoom(t, web.URL, b)
	_ = readMessage(t, ca, "state")
	_ = readMessage(t, cb, "state")
	parkAtOpenBoundary(s, a.Room, 11, 3)
	parkAtOpenBoundary(s, b.Room, 17, 3)
	s.reconcileEndless()
	_ = readMessage(t, ca, "room_moved")
	_ = readMessage(t, cb, "room_moved")
	ra := joinRoom(t, web.URL, a.Room, a.Name, a.Token, http.StatusOK)
	rb := joinRoom(t, web.URL, b.Room, b.Name, b.Token, http.StatusOK)
	na, nb := dialRoom(t, web.URL, ra), dialRoom(t, web.URL, rb)
	joined := readMessage(t, na, "state")
	_ = readMessage(t, nb, "state")
	if html := renderScreen(joined, a.Seat, s.catalog, *joined.Config); !strings.Contains(html, "Two runs joined") {
		t.Fatal("shared-round cue disappeared from the notification bar")
	}

	// Park the shared table at the boundary where the next Endless cycle
	// shrinks to two seats. The scheduled deal must retain both controllers.
	s.mu.Lock()
	shared := s.rooms[ra.Room]
	s.mu.Unlock()
	shared.mu.Lock()
	shared.state.Round, shared.state.RoundHand = 7, 0
	shared.state.Phase = domain.PhaseHandComplete
	shared.state.Hands = map[domain.SeatID][]domain.CardID{}
	shared.state.Retained = nil
	shared.state.CurrentTrick = nil
	shared.state.Revision++
	before := shared.state.Revision
	shared.markDealBoundaryLocked([]domain.Event{{Kind: domain.EventHandCompleted}})
	shared.mu.Unlock()
	shared.scheduleAutomaticDeal()
	va, vb := readRevision(t, na, before+1), readRevision(t, nb, before+1)
	if va.View.Public.Phase != domain.PhasePlayingTrick || va.View.Public.RoundHand != 1 || len(va.View.Public.Seats) != 2 {
		t.Fatalf("shared boundary did not automatically deal two seats: %+v", va.View.Public)
	}
	if va.View.Seat != a.Seat || vb.View.Seat != b.Seat || len(va.View.Hand) == 0 || len(vb.View.Hand) == 0 {
		t.Fatal("a shared player lost their private hand at the boundary")
	}
	shared.mu.Lock()
	first, second := shared.seats[0], shared.seats[1]
	shared.mu.Unlock()
	participants := map[domain.SeatID]participant{first.ID: first, second.ID: second}
	if first.Bot || second.Bot || len(participants) != 2 ||
		participants[a.Seat].Token != a.Token || participants[b.Seat].Token != b.Token ||
		participants[a.Seat].RunID != a.Room || participants[b.Seat].RunID != b.Room {
		t.Fatalf("shared participants were rebuilt as bots: %+v %+v", first, second)
	}
	if len(va.Seats) != 2 || !va.Seats[0].Host || va.Seats[0].Bot || va.Seats[1].Bot {
		t.Fatalf("shared seat identities changed: %+v", va.Seats)
	}
	html := renderScreen(va, a.Seat, s.catalog, *va.Config)
	if !strings.Contains(html, "Run totals") || !strings.Contains(html, "A · 11") || !strings.Contains(html, "B · 17") {
		t.Fatal("Scores control lost either shared player's run total")
	}
}

func TestEndlessIncompatibleDealNeverTransfers(t *testing.T) {
	s, web := testServer(t)
	s.botDelay = time.Hour
	a, b := startSoloForMerge(t, web.URL, "A"), startSoloForMerge(t, web.URL, "B")
	ca, cb := dialRoom(t, web.URL, a), dialRoom(t, web.URL, b)
	_ = readMessage(t, ca, "state")
	_ = readMessage(t, cb, "state")
	parkAtOpenBoundary(s, a.Room, 4, 3)
	parkAtOpenBoundary(s, b.Room, 6, 4)
	s.reconcileEndless()
	s.mu.Lock()
	count := len(s.rooms)
	s.mu.Unlock()
	if count != 2 {
		t.Fatalf("incompatible deals transferred: %d rooms", count)
	}
}

func TestEndlessDraftCueClearsOnDisconnect(t *testing.T) {
	s, web := testServer(t)
	s.botDelay = time.Hour
	a, b := startSoloForMerge(t, web.URL, "A"), startSoloForMerge(t, web.URL, "B")
	ca, cb := dialRoom(t, web.URL, a), dialRoom(t, web.URL, b)
	_ = readMessage(t, ca, "state")
	_ = readMessage(t, cb, "state")
	s.mu.Lock()
	ra, rb := s.rooms[a.Room], s.rooms[b.Room]
	ra.mu.Lock()
	ra.state.Round, ra.state.RoundHand, ra.state.Phase = 2, 0, domain.PhaseAwaitingChoice
	ra.state.DraftOptions = []domain.RuleID{domain.RuleLastTrickBonus, domain.RuleAvoidHearts}
	ra.state.PendingBoundaryID = "test-boundary"
	ra.state.Revision++
	ra.mu.Unlock()
	rb.mu.Lock()
	rb.state.Round = 2
	rb.state.Field = append(rb.state.Field, domain.RuleInstance{InstanceID: "rule:test", DefinitionID: domain.RuleLastTrickBonus, Sequence: len(rb.state.Field)})
	rb.state.Revision++
	rb.mu.Unlock()
	s.mu.Unlock()
	s.reconcileEndless()
	ra.mu.Lock()
	cue := ra.draftCues[domain.RuleLastTrickBonus]
	ra.mu.Unlock()
	if cue == "" {
		t.Fatal("helpful draft choice has no cue")
	}
	_ = cb.Close()
	// The socket's detach runs asynchronously; force the same state observed by
	// the coordinator so the recomputation is deterministic.
	rb.mu.Lock()
	delete(rb.clients, b.Seat)
	rb.mu.Unlock()
	s.reconcileEndless()
	ra.mu.Lock()
	remaining := len(ra.draftCues)
	ra.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("stale draft cue after disconnect: %d", remaining)
	}
	cb = dialRoom(t, web.URL, b)
	_ = readMessage(t, cb, "state")
	s.reconcileEndless()
	ra.mu.Lock()
	if ra.draftCues[domain.RuleLastTrickBonus] == "" {
		ra.mu.Unlock()
		t.Fatal("cue did not return on reconnect")
	}
	current := ra.state.Revision
	ra.mu.Unlock()
	sendCommand(t, ca, command{Type: "choose_rule", ID: "different-choice", Rule: domain.RuleAvoidHearts, BoundaryID: "test-boundary"})
	_ = readRevision(t, ca, current+1)
	s.reconcileEndless()
	ra.mu.Lock()
	remaining = len(ra.draftCues)
	ra.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("stale draft cue after choice: %d", remaining)
	}
}
