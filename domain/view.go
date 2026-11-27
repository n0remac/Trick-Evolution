package domain

type PublicSeat struct {
	ID            SeatID
	Score         int
	TricksWon     int
	HandCount     int
	CapturedCount int
	CarriedCount  int
	RetainedCount int
}

type PublicView struct {
	Revision          uint64
	Phase             Phase
	Mode              MatchMode
	Round             int
	RoundHand         int
	RoundLength       int
	PendingBoundaryID string
	DraftOptions      []RuleID
	DraftCues         map[RuleID]string
	TableCue          string
	RunScores         map[SeatID]int
	ShortGameRound    int
	ShortGameNumber   int
	ShortGameScores   map[SeatID]int
	ShortGameResult   *ShortGameResult
	GameNumber        int
	GameRuleKey       string
	GameScores        map[SeatID]int
	GameTeamScores    map[int]int
	GameTarget        int
	WinOnBid          bool
	GameResult        *TargetGameResult
	KnownGame         string
	RecognitionNotice string
	NextRosterSize    int
	HandSize          int
	DealSize          int
	TricksPerHand     int
	MaxHands          int
	RevealEvery       int
	InitialRules      []RuleID
	HandAwards        []HandAward
	LastScorecard     *Scorecard
	Seats             []PublicSeat
	CurrentTrick      []Play
	TrumpSuit         Suit
	TurnedTrumpCard   CardID
	HandTeams         map[SeatID]int
	TeamScores        map[int]int
	Auction           *AuctionState
	BrokenSpades      bool
	BrokenHearts      bool
	Dealer            SeatID
	Leader            SeatID
	Turn              SeatID
	HandNumber        int
	TrickNumber       int
	CompletedHands    int
	EndedEarly        bool
	EndedBy           SeatID
	Rules             []ResolvedRule
	ActiveField       []ResolvedRule
	Effective         []EffectiveEffect
	RulesRemaining    int
	Winners           []SeatID
}

type PlayerView struct {
	Public   PublicView
	Seat     SeatID
	Hand     []CardID
	Carried  CardID
	Retained CardID
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
	length := endlessRoundLength(state)
	view := PublicView{Revision: state.Revision, Phase: state.Phase, Mode: state.Config.Mode, Round: state.Round, RoundHand: state.RoundHand, RoundLength: length, PendingBoundaryID: state.PendingBoundaryID, DraftOptions: append([]RuleID(nil), state.DraftOptions...), TableCue: state.TableCue, HandSize: state.Config.HandSize, DealSize: state.DealSize, TricksPerHand: state.TricksPerHand, MaxHands: state.Config.MaxHands, RevealEvery: state.Config.RevealEvery, InitialRules: append([]RuleID(nil), state.Config.InitialRules...), Dealer: state.Dealer, Leader: state.Leader,
		Turn: state.Turn, HandNumber: state.HandNumber, TrickNumber: state.TrickNumber, CompletedHands: state.CompletedHands,
		EndedEarly: state.EndedEarly, EndedBy: state.EndedBy,
		Rules: rules.History, ActiveField: rules.ActiveField, Effective: rules.Effective, RulesRemaining: len(state.RuleDraw), CurrentTrick: append([]Play(nil), state.CurrentTrick...), TrumpSuit: state.TrumpSuit, BrokenSpades: state.BrokenSpades, BrokenHearts: state.BrokenHearts,
		Seats: make([]PublicSeat, len(state.Seats))}
	view.LastScorecard = cloneScorecard(state.LastScorecard)
	view.GameNumber, view.GameRuleKey, view.GameScores, view.GameResult = state.GameNumber, state.GameRuleKey, cloneSeatScores(state.GameScores), cloneTargetResult(state.GameResult)
	view.GameTeamScores = make(map[int]int, len(state.GameTeamScores))
	for team, score := range state.GameTeamScores {
		view.GameTeamScores[team] = score
	}
	view.GameTarget = targetForRules(rules)
	_, bidErr := rules.Effect(MechanicWinOnBid)
	view.WinOnBid = bidErr == nil
	view.KnownGame, view.RecognitionNotice = state.KnownGame, state.RecognitionNotice
	view.TurnedTrumpCard = state.TurnedTrumpCard
	if state.Auction != nil {
		a := *state.Auction
		a.Passed = make(map[SeatID]bool, len(state.Auction.Passed))
		for id, passed := range state.Auction.Passed {
			a.Passed[id] = passed
		}
		a.Calls = append([]AuctionCall(nil), state.Auction.Calls...)
		if a.Contract != nil {
			c := *a.Contract
			a.Contract = &c
		}
		view.Auction = &a
	}
	if len(state.HandTeams) > 0 {
		view.HandTeams = make(map[SeatID]int, len(state.HandTeams))
		for id, team := range state.HandTeams {
			view.HandTeams[id] = team
		}
		view.TeamScores = make(map[int]int, len(state.TeamScores))
		for team, score := range state.TeamScores {
			view.TeamScores[team] = score
		}
	}
	if state.Config.Mode == ModeEndless {
		if seats, ok := endlessNextRoster(state, rules); ok {
			view.NextRosterSize = len(seats)
		}
		view.RunScores = make(map[SeatID]int, len(state.RunScores))
		for id, score := range state.RunScores {
			view.RunScores[id] = score
		}
		view.ShortGameRound, view.ShortGameNumber = state.ShortGameRound, state.ShortGameNumber
		if state.ShortGameScores != nil {
			view.ShortGameScores = make(map[SeatID]int, len(state.ShortGameScores))
			for id, score := range state.ShortGameScores {
				view.ShortGameScores[id] = score
			}
		}
		if state.ShortGameResult != nil {
			result := *state.ShortGameResult
			result.Scores = make(map[SeatID]int, len(result.Scores))
			for id, score := range state.ShortGameResult.Scores {
				result.Scores[id] = score
			}
			result.Winners = append([]SeatID(nil), result.Winners...)
			view.ShortGameResult = &result
		}
	}
	if state.Phase == PhaseGameComplete {
		view.Winners = matchWinners(state)
	}
	if state.Phase == PhaseHandComplete || state.Phase == PhaseAwaitingChoice || state.Phase == PhaseGameComplete {
		view.HandAwards = append([]HandAward(nil), state.HandAwards...)
		retained := make(map[CardID]bool, len(state.Retained))
		for _, id := range state.Retained {
			retained[id] = true
		}
		for i := range view.HandAwards {
			if retained[view.HandAwards[i].Card] {
				view.HandAwards[i].Card = ""
				view.HandAwards[i].HiddenCard = true
			}
		}
	}
	for i, seat := range state.Seats {
		view.Seats[i] = PublicSeat{ID: seat.ID, Score: seat.Score, TricksWon: seat.TricksWon,
			HandCount: len(state.Hands[seat.ID]), CapturedCount: len(state.Captured[seat.ID])}
		if state.Carried[seat.ID] != "" {
			view.Seats[i].CarriedCount = 1
		}
		if state.Retained[seat.ID] != "" {
			view.Seats[i].RetainedCount = 1
		}
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
	return PlayerView{Public: public, Seat: seat, Hand: hand, Carried: state.Carried[seat], Retained: state.Retained[seat]}, nil
}
