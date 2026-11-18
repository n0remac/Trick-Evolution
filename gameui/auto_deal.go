package gameui

import (
	"fmt"
	"log"
	"time"

	"github.com/n0remac/trick-evolution/domain"
)

// The room owns hand progression. A revision guard makes timer, reconnect,
// and a legacy manual next command converge on at most one new deal.
func (r *room) markDealBoundaryLocked(events []domain.Event) {
	for _, event := range events {
		if event.Kind == domain.EventHandCompleted {
			r.dealReadyAt = time.Now().Add(r.server.dealDelay)
			r.dealScheduled = 0
			return
		}
	}
}

func (r *room) hasConnectedHumanLocked() bool {
	for _, seat := range r.seats {
		if !seat.Bot && r.clients[seat.ID] != nil {
			return true
		}
	}
	return false
}

func (r *room) scheduleAutomaticDeal() {
	r.mu.Lock()
	if r.closed || r.state == nil || r.state.Phase != domain.PhaseHandComplete || r.pendingReview != "" || !r.hasConnectedHumanLocked() {
		r.mu.Unlock()
		return
	}
	revision := r.state.Revision
	if r.dealScheduled == revision {
		r.mu.Unlock()
		return
	}
	r.dealScheduled = revision
	delay := time.Until(r.dealReadyAt)
	r.mu.Unlock()
	if delay < 0 {
		delay = 0
	}
	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-r.done:
			return
		case <-timer.C:
			r.automaticDeal(revision)
		}
	}()
}

func (r *room) syncEndlessRosterLocked(next domain.MatchState) {
	if r.config.Mode != domain.ModeEndless || len(next.Seats) == len(r.seats) {
		return
	}
	participants := make([]participant, len(next.Seats))
	participants[0] = r.seats[0]
	for i := 1; i < len(next.Seats); i++ {
		participants[i] = participant{ID: next.Seats[i].ID, Name: fmt.Sprintf("Bot %d", i), Bot: true}
	}
	r.seats = participants
}

func (r *room) automaticDeal(revision uint64) {
	r.dispatchMu.Lock()
	defer r.dispatchMu.Unlock()
	r.mu.Lock()
	if r.dealScheduled == revision {
		r.dealScheduled = 0
	}
	if r.closed || r.state == nil || r.state.Revision != revision || r.state.Phase != domain.PhaseHandComplete ||
		r.pendingReview != "" || !r.hasConnectedHumanLocked() || time.Now().Before(r.dealReadyAt) {
		r.mu.Unlock()
		return
	}
	deck := domain.StandardDeck()
	r.server.shuffle().Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	order := make([]domain.CardID, len(deck))
	for i, card := range deck {
		order[i] = card.ID
	}
	next, events, err := domain.Step(*r.state, domain.Action{Kind: domain.ActionStartHand, Seat: r.seats[0].ID, DeckOrder: order}, r.server.catalog)
	if err != nil {
		r.mu.Unlock()
		log.Printf("automatic deal room=%s revision=%d: %v", r.id, revision, err)
		return
	}
	r.recordPublicEventsLocked(r.state, events)
	r.syncEndlessRosterLocked(next)
	r.state = &next
	r.lastEvents = events
	r.lastActivity = r.server.now()
	r.mu.Unlock()
	r.publishLocked()
	r.signalBot()
}
