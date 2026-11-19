package gameui

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	"github.com/n0remac/trick-evolution/domain"
)

// reconcileEndless is the sole coordinator for transfers. Room dispatch locks
// exclude plays, choices and departures while revisions are checked and moved.
func (s *Server) reconcileEndless() {
	s.mergeMu.Lock()
	defer s.mergeMu.Unlock()
	s.mu.Lock()
	rooms := make([]*room, 0, len(s.rooms))
	for _, r := range s.rooms {
		if r.config.Mode == domain.ModeEndless {
			rooms = append(rooms, r)
		}
	}
	s.mu.Unlock()
	sort.Slice(rooms, func(i, j int) bool { return rooms[i].id < rooms[j].id })
	for i := range rooms {
		for j := i + 1; j < len(rooms); j++ {
			if s.tryJoinEndless(rooms[i], rooms[j]) {
				return
			}
		}
	}
	s.refreshEndlessCues(rooms)
}

func (s *Server) refreshEndlessCues(rooms []*room) {
	type snapshot struct {
		room   *room
		state  domain.MatchState
		online bool
		solo   bool
	}
	items := make([]snapshot, 0, len(rooms))
	for _, r := range rooms {
		r.mu.Lock()
		if !r.closed && r.state != nil {
			items = append(items, snapshot{r, *r.state, len(r.seats) > 0 && r.clients[r.seats[0].ID] != nil, len(r.seats) == 2 && !r.seats[0].Bot && r.seats[1].Bot})
		}
		r.mu.Unlock()
	}
	for _, item := range items {
		cues := make(map[domain.RuleID]string)
		if item.online && item.solo && item.state.Phase == domain.PhaseAwaitingChoice {
			for _, other := range items {
				if other.room == item.room || !other.online || !other.solo {
					continue
				}
				base, ok := domain.EndlessForecastGap(item.state, other.state, s.catalog, "")
				if !ok {
					continue
				}
				baseNow, _ := domain.EndlessChoiceGap(item.state, other.state, s.catalog, "")
				for _, choice := range item.state.DraftOptions {
					gap, ok := domain.EndlessForecastGap(item.state, other.state, s.catalog, choice)
					gapNow, okNow := domain.EndlessChoiceGap(item.state, other.state, s.catalog, choice)
					if ok && okNow && (gap < base || gapNow < baseNow) {
						if gap == 0 || gapNow == 0 {
							cues[choice] = "Could match another run."
						} else {
							cues[choice] = "Moves closer to another run."
						}
					}
				}
			}
		}
		r := item.room
		if len(cues) == 0 {
			cues = nil
		}
		r.mu.Lock()
		changed := r.state != nil && r.state.Revision == item.state.Revision && !reflect.DeepEqual(r.draftCues, cues)
		if changed {
			r.draftCues = cues
		}
		r.mu.Unlock()
		if changed {
			r.publish()
		}
	}
}

func (s *Server) tryJoinEndless(a, b *room) bool {
	a.dispatchMu.Lock()
	defer a.dispatchMu.Unlock()
	b.dispatchMu.Lock()
	defer b.dispatchMu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	a.mu.Lock()
	defer a.mu.Unlock()
	b.mu.Lock()
	defer b.mu.Unlock()
	if s.rooms[a.id] != a || s.rooms[b.id] != b || a.closed || b.closed ||
		a.state == nil || b.state == nil || len(a.seats) != 2 || len(b.seats) != 2 ||
		a.seats[0].Bot || b.seats[0].Bot || a.clients[a.seats[0].ID] == nil || b.clients[b.seats[0].ID] == nil ||
		!domain.EndlessCompatible(*a.state, *b.state, s.catalog) {
		return false
	}
	revisionA, revisionB := a.state.Revision, b.state.Revision
	deck := domain.StandardDeck()
	s.shuffle().Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	order := make([]domain.CardID, len(deck))
	for i, card := range deck {
		order[i] = card.ID
	}
	next, err := domain.JoinEndless(*a.state, *b.state, s.catalog, order)
	if err != nil || a.state.Revision != revisionA || b.state.Revision != revisionB {
		return false
	}
	id, err := randomToken(8)
	if err != nil {
		return false
	}
	roomID := "endless-" + id
	if s.rooms[roomID] != nil {
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	seats := []participant{a.seats[0], b.seats[0]}
	for i := 1; len(seats) < len(next.Seats); i++ {
		seats = append(seats, participant{ID: next.Seats[len(seats)].ID, Name: fmt.Sprintf("Bot %d", i), Bot: true})
	}
	next.Revision = max(revisionA, revisionB) + 1
	joined := &room{id: roomID, config: next.Config, seats: seats, state: &next,
		clients: make(map[domain.SeatID]*client), seen: make(map[domain.SeatID]map[string]acceptedCommand),
		lastEvents:   []domain.Event{{Kind: domain.EventHandStarted, Revision: next.Revision, Hand: next.HandNumber, Seat: next.Leader}},
		lastActivity: s.now(), server: s, botWake: make(chan struct{}, 1), done: make(chan struct{}), ctx: ctx, cancel: cancel, workerStarted: true}
	s.rooms[roomID] = joined
	s.redirects[a.id], s.redirects[b.id] = roomID, roomID
	delete(s.rooms, a.id)
	delete(s.rooms, b.id)
	a.closeLocked()
	b.closeLocked()
	for _, old := range []*room{a, b} {
		for _, c := range old.clients {
			token := ""
			for _, p := range old.seats {
				if p.ID == c.seat {
					token = p.Token
					break
				}
			}
			c.enqueue(serverMessage{Type: "room_moved", Room: roomID, Token: token})
		}
	}
	go joined.runBots()
	return true
}
