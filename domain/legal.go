package domain

import "fmt"

// CardDetails resolves a stable standard-card ID without match-private state.
func CardDetails(id CardID) (StandardCard, bool) { return cardByID(id) }

// LegalCards uses only a seat's projected view. The play restriction is the
// same helper used by Step, so callers cannot infer or need opponents' hands.
func LegalCards(view PlayerView, catalog Catalog) ([]CardID, error) {
	if view.Public.Phase != PhasePlayingTrick {
		return nil, transitionError(ErrWrongPhase, "cards can only be played during a trick")
	}
	if view.Seat != view.Public.Turn {
		return nil, transitionError(ErrNotYourTurn, "another seat has the turn")
	}
	var effect SuitEffect
	for _, rule := range view.Public.Rules {
		if !rule.Active {
			continue
		}
		definition, ok := catalog.Definition(rule.Instance.DefinitionID)
		if !ok || definition.Status != StatusSupported {
			return nil, fmt.Errorf("unknown active rule %s", rule.Instance.DefinitionID)
		}
		if definition.PlayEffect != "" {
			if effect != "" {
				return nil, fmt.Errorf("multiple active play effects")
			}
			effect = definition.PlayEffect
		}
	}
	return legalCardsForHand(view.Hand, view.Public.CurrentTrick, effect)
}

func legalCardsForHand(hand []CardID, trick []Play, effect SuitEffect) ([]CardID, error) {
	if effect != "" && effect != EffectFollow && effect != EffectAnySuit {
		return nil, fmt.Errorf("unknown play effect %s", effect)
	}
	var lead Suit
	if len(trick) > 0 {
		card, ok := cardByID(trick[0].Card)
		if !ok {
			return nil, fmt.Errorf("unknown lead card")
		}
		lead = card.Suit
	}
	all := append([]CardID(nil), hand...)
	var matching []CardID
	for _, id := range hand {
		card, ok := cardByID(id)
		if !ok {
			return nil, fmt.Errorf("unknown hand card %s", id)
		}
		if card.Suit == lead {
			matching = append(matching, id)
		}
	}
	if effect == EffectFollow && lead != "" && len(matching) > 0 {
		return matching, nil
	}
	return all, nil
}
