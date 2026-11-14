package gameui

import "github.com/n0remac/trick-evolution/domain"

// ObservedPlay contains only facts visible when a card was played. The
// play restriction and selected trump are stored at that time; later rule
// reveals cannot reinterpret whether an off-suit play proved a void suit.
type ObservedPlay struct {
	Seat            domain.SeatID
	Card            domain.CardID
	LeadSuit        domain.Suit
	PlayRestriction domain.EffectValue
	TrumpSuit       domain.Suit
}

type ObservedTrick struct {
	Winner domain.SeatID
	Plays  []domain.Play
}

type BotObservation struct {
	View   domain.PlayerView
	Played []ObservedPlay
	Won    []ObservedTrick
}

// recordPublicEventsLocked retains only accepted public events. Caller holds
// room.mu; previous is the state before the accepted transition.
func (r *room) recordPublicEventsLocked(previous *domain.MatchState, events []domain.Event) {
	for _, event := range events {
		switch event.Kind {
		case domain.EventHandStarted:
			r.publicPlays = nil
			r.publicTricks = nil
		case domain.EventCardPlayed:
			if previous == nil {
				continue
			}
			leadID := event.Card
			if len(previous.CurrentTrick) > 0 {
				leadID = previous.CurrentTrick[0].Card
			}
			lead, _ := domain.CardDetails(leadID) // Accepted Step validated the card.
			resolved, _ := domain.ResolveRules(previous.Field, r.server.catalog, previous.Config)
			play, _ := resolved.Effect(domain.MechanicPlayRestriction)
			r.publicPlays = append(r.publicPlays, ObservedPlay{Seat: event.Seat, Card: event.Card, LeadSuit: lead.Suit, PlayRestriction: play.Value, TrumpSuit: previous.TrumpSuit})
		case domain.EventTrickWon:
			r.publicTricks = append(r.publicTricks, ObservedTrick{Winner: event.Seat, Plays: append([]domain.Play(nil), event.Plays...)})
		}
	}
}

func (r *room) botObservationLocked(seat domain.SeatID) (BotObservation, error) {
	view, err := domain.ProjectForSeat(*r.state, r.server.catalog, seat)
	if err != nil {
		return BotObservation{}, err
	}
	observation := BotObservation{View: view, Played: append([]ObservedPlay(nil), r.publicPlays...), Won: make([]ObservedTrick, len(r.publicTricks))}
	for i, trick := range r.publicTricks {
		observation.Won[i] = ObservedTrick{Winner: trick.Winner, Plays: append([]domain.Play(nil), trick.Plays...)}
	}
	return observation, nil
}
