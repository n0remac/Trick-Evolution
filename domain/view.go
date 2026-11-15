package domain

type PublicSeat struct {
	ID            SeatID
	Score         int
	TricksWon     int
	HandCount     int
	CapturedCount int
}

type PublicView struct {
	Revision       uint64
	Phase          Phase
	HandSize       int
	MaxHands       int
	RevealEvery    int
	InitialRules   []RuleID
	HandAwards     []HandAward
	Seats          []PublicSeat
	CurrentTrick   []Play
	TrumpSuit      Suit
	BrokenSpades   bool
	BrokenHearts   bool
	Dealer         SeatID
	Leader         SeatID
	Turn           SeatID
	HandNumber     int
	TrickNumber    int
	CompletedHands int
	EndedEarly     bool
	EndedBy        SeatID
	Rules          []ResolvedRule
	ActiveField    []ResolvedRule
	Effective      []EffectiveEffect
	RulesRemaining int
	Winners        []SeatID
}

type PlayerView struct {
	Public PublicView
	Seat   SeatID
	Hand   []CardID
}

// ProjectPublic contains only information visible to every seat.
func ProjectPublic(state MatchState, catalog Catalog) (PublicView, error) {
	if state.CatalogVersion != catalog.Version {
		return PublicView{}, transitionError(ErrInvalidState, "catalog version mismatch")
	}
	rules, err := ResolveRules(state.Field, catalog, state.Config)
	if err != nil {
		return PublicView{}, transitionError(ErrInvalidState, err.Error())
	}
	if err := rules.ValidateMatchEffects(state.Config); err != nil {
		return PublicView{}, transitionError(ErrInvalidState, err.Error())
	}
	if err := validateHandFacts(state, rules); err != nil {
		return PublicView{}, transitionError(ErrInvalidState, err.Error())
	}
	view := PublicView{Revision: state.Revision, Phase: state.Phase, HandSize: state.Config.HandSize, MaxHands: state.Config.MaxHands, RevealEvery: state.Config.RevealEvery, InitialRules: append([]RuleID(nil), state.Config.InitialRules...), Dealer: state.Dealer, Leader: state.Leader,
		Turn: state.Turn, HandNumber: state.HandNumber, TrickNumber: state.TrickNumber, CompletedHands: state.CompletedHands,
		EndedEarly: state.EndedEarly, EndedBy: state.EndedBy,
		Rules: rules.History, ActiveField: rules.ActiveField, Effective: rules.Effective, RulesRemaining: len(state.RuleDraw), CurrentTrick: append([]Play(nil), state.CurrentTrick...), TrumpSuit: state.TrumpSuit, BrokenSpades: state.BrokenSpades, BrokenHearts: state.BrokenHearts,
		Seats: make([]PublicSeat, len(state.Seats))}
	if state.Phase == PhaseGameComplete {
		view.Winners = gameWinners(state.Seats)
	}
	if state.Phase == PhaseHandComplete || state.Phase == PhaseGameComplete && state.CompletedHands == state.HandNumber {
		view.HandAwards = append([]HandAward(nil), state.HandAwards...)
	}
	for i, seat := range state.Seats {
		view.Seats[i] = PublicSeat{ID: seat.ID, Score: seat.Score, TricksWon: seat.TricksWon,
			HandCount: len(state.Hands[seat.ID]), CapturedCount: len(state.Captured[seat.ID])}
	}
	return view, nil
}

// ProjectForSeat attaches only the requested seat's private cards.
func ProjectForSeat(state MatchState, catalog Catalog, seat SeatID) (PlayerView, error) {
	if indexOfSeat(state.Seats, seat) < 0 {
		return PlayerView{}, transitionError(ErrUnknownSeat, "seat is not in this match")
	}
	public, err := ProjectPublic(state, catalog)
	if err != nil {
		return PlayerView{}, err
	}
	hand := make([]CardID, len(state.Hands[seat]))
	copy(hand, state.Hands[seat])
	return PlayerView{Public: public, Seat: seat, Hand: hand}, nil
}
