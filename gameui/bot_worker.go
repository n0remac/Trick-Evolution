package gameui

import (
	"log"
	"time"

	"github.com/n0remac/trick-evolution/domain"
)

// closeLocked cancels any pending bot decision. The caller holds room.mu.
func (r *room) closeLocked() {
	if !r.closed {
		r.closed = true
		close(r.done)
	}
}

func (r *room) hasBotLocked() bool {
	for _, seat := range r.seats {
		if seat.Bot {
			return true
		}
	}
	return false
}

func (r *room) botTurnLocked() bool {
	if r.state == nil || r.state.Phase != domain.PhasePlayingTrick {
		return false
	}
	for _, seat := range r.seats {
		if seat.ID == r.state.Turn {
			return seat.Bot
		}
	}
	return false
}

func (r *room) signalBot() {
	select {
	case <-r.done:
	case r.botWake <- struct{}{}:
	default:
	}
}

// One worker per started bot room prevents overlapping decisions. Projection
// is copied under room.mu, but delay and heuristic computation hold no room
// locks. The revision is checked again before Step commits a bot move.
func (r *room) runBots() {
	for {
		select {
		case <-r.done:
			return
		case <-r.botWake:
		}
		for {
			r.mu.Lock()
			if r.closed || r.state == nil || r.state.Phase == domain.PhaseGameComplete {
				r.mu.Unlock()
				return
			}
			if !r.botTurnLocked() {
				r.mu.Unlock()
				break
			}
			seat, revision := r.state.Turn, r.state.Revision
			view, err := domain.ProjectForSeat(*r.state, r.server.catalog, seat)
			r.mu.Unlock()
			if err != nil {
				log.Printf("bot projection in room %s: %v", r.id, err)
				return
			}
			timer := time.NewTimer(r.server.botDelay)
			select {
			case <-r.done:
				timer.Stop()
				return
			case <-timer.C:
			}
			card, err := chooseBotCard(view, r.server.catalog)
			if err != nil {
				log.Printf("bot choice in room %s: %v", r.id, err)
				return
			}
			r.dispatchMu.Lock()
			r.mu.Lock()
			if r.closed {
				r.mu.Unlock()
				r.dispatchMu.Unlock()
				return
			}
			if r.state == nil || r.state.Revision != revision || !r.botTurnLocked() || r.state.Turn != seat {
				r.mu.Unlock()
				r.dispatchMu.Unlock()
				continue
			}
			next, events, err := domain.Step(*r.state, domain.Action{Kind: domain.ActionPlayCard, Seat: seat, Card: card}, r.server.catalog)
			if err != nil {
				r.mu.Unlock()
				r.dispatchMu.Unlock()
				log.Printf("bot play in room %s: %v", r.id, err)
				return
			}
			r.state = &next
			r.lastEvents = events
			r.lastActivity = r.server.now()
			r.mu.Unlock()
			r.publishLocked()
			r.dispatchMu.Unlock()
			if next.Phase == domain.PhaseGameComplete {
				return
			}
		}
	}
}
