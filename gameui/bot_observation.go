package gameui

import "github.com/n0remac/trick-evolution/domain"

// ObservedPlay contains only facts visible when a card was played. The
// follow flag must be stored at that time; later rule reveals cannot change
// whether an off-suit play proved a void suit.
type ObservedPlay struct {
	Seat           domain.SeatID
	Card           domain.CardID
	LeadSuit       domain.Suit
	FollowRequired bool
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
			follow := false
			resolved, _ := domain.ResolveField(previous.Field, r.server.catalog)
			for _, rule := range resolved {
				if rule.Active && rule.Instance.DefinitionID == domain.RuleFollowSuit {
					follow = true
				}
			}
			r.publicPlays = append(r.publicPlays, ObservedPlay{Seat: event.Seat, Card: event.Card, LeadSuit: lead.Suit, FollowRequired: follow})
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
