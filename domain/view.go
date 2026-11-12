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
	Seats          []PublicSeat
	CurrentTrick   []Play
	Dealer         SeatID
	Leader         SeatID
	Turn           SeatID
	HandNumber     int
	TrickNumber    int
	CompletedHands int
	Rules          []ResolvedRule
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
	rules, err := ResolveField(state.Field, catalog)
	if err != nil {
		return PublicView{}, transitionError(ErrInvalidState, err.Error())
	}
	view := PublicView{Revision: state.Revision, Phase: state.Phase, Dealer: state.Dealer, Leader: state.Leader,
		Turn: state.Turn, HandNumber: state.HandNumber, TrickNumber: state.TrickNumber, CompletedHands: state.CompletedHands,
		Rules: rules, RulesRemaining: len(state.RuleDraw), CurrentTrick: append([]Play(nil), state.CurrentTrick...),
		Seats: make([]PublicSeat, len(state.Seats))}
	if state.Phase == PhaseGameComplete {
		view.Winners = gameWinners(state.Seats)
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
	return PlayerView{Public: public, Seat: seat, Hand: append([]CardID(nil), state.Hands[seat]...)}, nil
}
