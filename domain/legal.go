package domain

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
	return legalCardsWithRules(view.Hand, view.Public.CurrentTrick, effective, factsFromView(view.Public))
}

func LegalCardReason(view PlayerView, card CardID, catalog Catalog) (string, error) {
	effective, err := EffectiveForView(view.Public, catalog)
	if err != nil {
		return "", err
	}
	return IllegalPlayReason(view.Hand, view.Public.CurrentTrick, card, effective, factsFromView(view.Public)), nil
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
