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
	effective, err := EffectiveForView(view.Public, catalog)
	if err != nil {
		return nil, err
	}
	rule, err := effective.Effect(MechanicPlayRestriction)
	if err != nil {
		return nil, err
	}
	effect := EffectFollow
	if rule.Value == ValueAnySuit {
		effect = EffectAnySuit
	}
	return legalCardsForHand(view.Hand, view.Public.CurrentTrick, effect)
}

// EffectiveForView uses the public projection; fixture views may omit the
// effective list, in which case the same interpreter derives it from history.
func EffectiveForView(view PublicView, catalog Catalog) (RuleResolution, error) {
	if len(view.Effective) > 0 {
		return RuleResolution{History: view.Rules, ActiveField: view.ActiveField, Effective: view.Effective}, nil
	}
	field := make([]RuleInstance, len(view.Rules))
	for i, r := range view.Rules {
		field[i] = r.Instance
	}
	return ResolveRules(field, catalog, Config{HandSize: view.HandSize, RevealEvery: view.RevealEvery, MaxHands: view.MaxHands})
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
