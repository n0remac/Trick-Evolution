package domain

import (
	"fmt"
	"reflect"
)

const (
	defaultOpenRounds = 3
	shortGameRounds   = 3
)

func openRounds(config Config) int {
	if config.OpenRounds > 0 {
		return config.OpenRounds
	}
	return defaultOpenRounds
}

func endlessCyclePosition(round int, config Config) int {
	return (round-1)%(openRounds(config)+shortGameRounds) + 1
}

func lastOpenRound(state MatchState) bool {
	return endlessCyclePosition(state.Round, state.Config) == openRounds(state.Config)
}

func endlessCue(state MatchState, cue string) string {
	if lastOpenRound(state) {
		return "A three-round game starts after this round. " + cue
	}
	return cue
}

// The first three open rounds grow from one to three bots. The roster then
// stays fixed for all three rounds of the short game and repeats.
func endlessRosterSize(round int, config Config) int {
	position := endlessCyclePosition(round, config)
	if position > openRounds(config) {
		position = openRounds(config)
	}
	if position > 3 {
		position = 3
	}
	return position + 1
}

func endlessNextRoster(state MatchState, rules RuleResolution) ([]Seat, bool) {
	if state.Config.Mode != ModeEndless || state.RoundHand != 0 {
		return nil, false
	}
	count := endlessRosterSize(state.Round, state.Config)
	if count == len(state.Seats) {
		return nil, false
	}
	if _, err := HandPlanForRules(state.Config, count, rules); err != nil {
		return nil, false
	}
	seats := make([]Seat, 0, count)
	for _, seat := range state.Seats {
		if len(seat.ID) < 4 || string(seat.ID[:4]) != "bot:" {
			seats = append(seats, seat)
		}
	}
	for i := 1; len(seats) < count; i++ {
		id := SeatID(fmt.Sprintf("bot:%d", i))
		seats = append(seats, Seat{ID: id, Score: state.RunScores[id]})
	}
	return seats, true
}

// ReplaceEndlessBotSeat transfers a live bot's cards and score to a guest.
// Keeping the seat position lets a guest join during a trick without redealing.
func ReplaceEndlessBotSeat(state MatchState, oldID, newID SeatID) (MatchState, error) {
	if state.Config.Mode != ModeEndless || state.Phase == PhaseGameComplete || oldID == "" || newID == "" {
		return state, fmt.Errorf("bot seat cannot be replaced")
	}
	found := false
	for _, seat := range state.Seats {
		if seat.ID == newID {
			return state, fmt.Errorf("guest seat already exists")
		}
		found = found || seat.ID == oldID
	}
	if !found {
		return state, fmt.Errorf("bot seat does not exist")
	}
	next := cloneState(state)
	for i := range next.Seats {
		if next.Seats[i].ID == oldID {
			next.Seats[i].ID = newID
		}
	}
	moveSeatKey(next.Hands, oldID, newID)
	moveSeatKey(next.Carried, oldID, newID)
	moveSeatKey(next.Retained, oldID, newID)
	moveSeatKey(next.Captured, oldID, newID)
	moveSeatKey(next.RunScores, oldID, newID)
	moveSeatKey(next.ShortGameScores, oldID, newID)
	if next.ShortGameResult != nil {
		moveSeatKey(next.ShortGameResult.Scores, oldID, newID)
		for i, id := range next.ShortGameResult.Winners {
			if id == oldID {
				next.ShortGameResult.Winners[i] = newID
			}
		}
	}
	for card, owner := range next.DealOwner {
		if owner == oldID {
			next.DealOwner[card] = newID
		}
	}
	for i := range next.HandAwards {
		if next.HandAwards[i].Seat == oldID {
			next.HandAwards[i].Seat = newID
		}
	}
	for i := range next.CurrentTrick {
		if next.CurrentTrick[i].Seat == oldID {
			next.CurrentTrick[i].Seat = newID
		}
	}
	for _, id := range []*SeatID{&next.Dealer, &next.Leader, &next.Turn, &next.EndedBy} {
		if *id == oldID {
			*id = newID
		}
	}
	next.Revision++
	return next, nil
}

func moveSeatKey[V any](m map[SeatID]V, oldID, newID SeatID) {
	if value, ok := m[oldID]; ok {
		m[newID] = value
		delete(m, oldID)
	}
}

// EndlessCompatible compares the actual controllers and the next deal. Both
// runs must be at open-round boundaries; no mid-hand rule is changed.
func EndlessCompatible(a, b MatchState, catalog Catalog) bool {
	if a.Config.Mode != ModeEndless || b.Config.Mode != ModeEndless ||
		a.RoundHand != 0 || b.RoundHand != 0 || a.Phase != PhaseHandComplete || b.Phase != PhaseHandComplete ||
		a.PendingBoundaryID != "" || b.PendingBoundaryID != "" || a.ShortGameRound != 0 || b.ShortGameRound != 0 ||
		a.Config.HandSize != b.Config.HandSize || a.Config.OpenRounds != b.Config.OpenRounds ||
		endlessCyclePosition(a.Round, a.Config) > openRounds(a.Config) || endlessCyclePosition(b.Round, b.Config) > openRounds(b.Config) || len(a.Seats) > 4 || len(b.Seats) > 4 {
		return false
	}
	ar, ae := ResolveRules(a.Field, catalog, a.Config)
	br, be := ResolveRules(b.Field, catalog, b.Config)
	if ae != nil || be != nil || ar.ValidateMatchEffects(a.Config) != nil || br.ValidateMatchEffects(b.Config) != nil {
		return false
	}
	for _, rule := range append(ar.History, br.History...) {
		if rule.InactiveReason == "prerequisite_inactive" {
			return false
		}
	}
	effects := func(r RuleResolution) map[Mechanic]EffectiveEffect {
		out := make(map[Mechanic]EffectiveEffect)
		for _, e := range r.Effective {
			e.Source = ""
			e.InstanceID = ""
			out[e.Mechanic] = e
		}
		return out
	}
	if !reflect.DeepEqual(effects(ar), effects(br)) {
		return false
	}
	count := endlessRosterSize(max(a.Round, b.Round), a.Config)
	pa, ea := HandPlanForRules(a.Config, count, ar)
	pb, eb := HandPlanForRules(b.Config, count, br)
	return ea == nil && eb == nil && pa == pb
}

// JoinEndless creates the first shared hand from two completed solo runs.
// Each person's seat and cumulative score are copied from their own run.
func JoinEndless(a, b MatchState, catalog Catalog, order []CardID) (MatchState, error) {
	if !EndlessCompatible(a, b, catalog) || len(a.Seats) == 0 || len(b.Seats) == 0 || a.Seats[0].ID == b.Seats[0].ID {
		return MatchState{}, fmt.Errorf("endless runs are not compatible")
	}
	next := cloneState(a)
	next.Round = max(a.Round, b.Round)
	next.RoundHand = 0
	next.CompletedHands = max(a.CompletedHands, b.CompletedHands)
	next.HandNumber = max(a.HandNumber, b.HandNumber)
	next.ShortGameNumber = max(a.ShortGameNumber, b.ShortGameNumber)
	next.ShortGameRound, next.ShortGameScores, next.ShortGameResult = 0, nil, nil
	count := endlessRosterSize(next.Round, a.Config)
	next.Seats = []Seat{a.Seats[0], b.Seats[0]}
	next.RunScores[b.Seats[0].ID] = b.RunScores[b.Seats[0].ID]
	for i := 1; len(next.Seats) < count; i++ {
		id := SeatID(fmt.Sprintf("bot:%d", i))
		next.Seats = append(next.Seats, Seat{ID: id, Score: next.RunScores[id]})
	}
	next.Hands = make(map[SeatID][]CardID)
	next.Retained, next.Carried = nil, nil
	next.Captured = make(map[SeatID][]CardID)
	next.DealOwner = make(map[CardID]SeatID)
	next.CurrentTrick = nil
	next.HandAwards = nil
	next.Dealer = next.Seats[0].ID
	next.Phase = PhaseHandComplete
	next.TableCue = "Two runs joined. A fresh shared round begins."
	next.SharedRound, next.SharedRoundLength = next.Round, 3
	result, _, err := Step(next, Action{Kind: ActionStartHand, Seat: next.Seats[0].ID, DeckOrder: order}, catalog)
	return result, err
}

// EndlessForecastGap searches open boundaries reached after at most two
// scheduled table events in each run. Unknown future drafts keep their field;
// the current draft can be tested with one of its existing choices.
func EndlessForecastGap(a, b MatchState, catalog Catalog, choice RuleID) (int, bool) {
	if a.Config.Mode != ModeEndless || b.Config.Mode != ModeEndless {
		return 0, false
	}
	a, b = cloneState(a), cloneState(b)
	if choice != "" {
		valid := false
		for _, id := range a.DraftOptions {
			valid = valid || id == choice
		}
		if !valid || !applyEndlessRule(&a, catalog, choice) {
			return 0, false
		}
	}
	a.Phase, b.Phase = PhaseHandComplete, PhaseHandComplete
	a.PendingBoundaryID, b.PendingBoundaryID = "", ""
	start := max(a.Round, b.Round)
	best := 1000
	found := false
	for round := start; round < start+24; round++ {
		pa, oka := forecastEndlessTo(a, round, catalog)
		pb, okb := forecastEndlessTo(b, round, catalog)
		if !oka || !okb {
			continue
		}
		if endlessCyclePosition(round, pa.Config) > openRounds(pa.Config) || endlessCyclePosition(round, pb.Config) > openRounds(pb.Config) {
			continue
		}
		pa.RoundHand, pb.RoundHand = 0, 0
		pa.ShortGameRound, pb.ShortGameRound = 0, 0
		gap := endlessEffectGap(pa, pb, catalog)
		if gap < best {
			best, found = gap, true
		}
		if best == 0 {
			break
		}
	}
	return best, found
}

// EndlessChoiceGap compares the current resolved fields after an offered
// choice, independently of whether either hand has reached its boundary.
func EndlessChoiceGap(a, b MatchState, catalog Catalog, choice RuleID) (int, bool) {
	if a.Config.Mode != ModeEndless || b.Config.Mode != ModeEndless {
		return 0, false
	}
	a = cloneState(a)
	if choice != "" {
		valid := false
		for _, id := range a.DraftOptions {
			valid = valid || id == choice
		}
		if !valid || !applyEndlessRule(&a, catalog, choice) {
			return 0, false
		}
	}
	a.Round = b.Round
	return endlessEffectGap(a, b, catalog), true
}

func forecastEndlessTo(state MatchState, round int, catalog Catalog) (MatchState, bool) {
	state = cloneState(state)
	changes := 0
	for r := state.Round + 1; r <= round; r++ {
		state.Round = r
		_, event := EndlessRound(r)
		if event != "table" {
			continue
		}
		changes++
		if changes > 2 {
			return state, false
		}
		if r == 2 {
			if !applyEndlessRule(&state, catalog, RuleExpandedHand) {
				return state, false
			}
			continue
		}
		added := false
		for _, id := range state.RuleDraw {
			if applyEndlessRule(&state, catalog, id) {
				added = true
				break
			}
		}
		if !added {
			retireEndlessRule(&state, catalog)
		}
	}
	return state, true
}

func endlessEffectGap(a, b MatchState, catalog Catalog) int {
	ar, ae := ResolveRules(a.Field, catalog, a.Config)
	br, be := ResolveRules(b.Field, catalog, b.Config)
	if ae != nil || be != nil || ar.ValidateMatchEffects(a.Config) != nil || br.ValidateMatchEffects(b.Config) != nil {
		return 1000
	}
	for _, rule := range append(ar.History, br.History...) {
		if rule.InactiveReason == "prerequisite_inactive" {
			return 1000
		}
	}
	values := func(r RuleResolution) map[Mechanic]EffectiveEffect {
		m := make(map[Mechanic]EffectiveEffect)
		for _, e := range r.Effective {
			e.Source, e.InstanceID = "", ""
			m[e.Mechanic] = e
		}
		return m
	}
	av, bv := values(ar), values(br)
	gap := 0
	for mechanic, effect := range av {
		if other, ok := bv[mechanic]; !ok || effect != other {
			gap++
		}
	}
	for mechanic := range bv {
		if _, ok := av[mechanic]; !ok {
			gap++
		}
	}
	if a.Config.HandSize != b.Config.HandSize || a.Config.OpenRounds != b.Config.OpenRounds {
		gap += 2
	}
	count := endlessRosterSize(a.Round, a.Config)
	pa, ea := HandPlanForRules(a.Config, count, ar)
	pb, eb := HandPlanForRules(b.Config, count, br)
	if ea != nil || eb != nil {
		gap += 10
	} else if pa != pb {
		gap += 2
	}
	return gap
}

func finishShortGame(state *MatchState) {
	best := -1
	var winners []SeatID
	for _, seat := range state.Seats {
		score := state.ShortGameScores[seat.ID]
		if score > best {
			best, winners = score, []SeatID{seat.ID}
		} else if score == best {
			winners = append(winners, seat.ID)
		}
	}
	scores := make(map[SeatID]int, len(state.ShortGameScores))
	for id, points := range state.ShortGameScores {
		scores[id] = points
	}
	state.ShortGameResult = &ShortGameResult{Number: state.ShortGameNumber, Scores: scores, Winners: winners}
	state.ShortGameRound = 0
	state.ShortGameScores = nil
}

// EndlessRound returns the number of complete hands and boundary event for a round.
// The late cadence is deliberately deterministic so a run can be replayed.
func EndlessRound(round int) (int, string) {
	switch round {
	case 1:
		return 3, "base"
	case 2:
		return 1, "table"
	case 4:
		return 1, "draft"
	case 3:
		return 1, "table"
	case 5:
		return 3, "hold"
	case 6:
		return 2, "table"
	}
	if round < 1 {
		return 0, ""
	}
	length := 2 + (round-7)%3
	if round%2 == 1 {
		return length, "draft"
	}
	if (round/2)%2 == 0 {
		return length, "hold"
	}
	return length, "table"
}

func endlessRoundLength(state MatchState) int {
	if state.Round == state.SharedRound && state.SharedRoundLength > 0 {
		return state.SharedRoundLength
	}
	length, _ := EndlessRound(state.Round)
	return length
}

func eligibleEndlessRule(state MatchState, catalog Catalog, id RuleID) ([]RuleInstance, bool) {
	d, ok := catalog.Definition(id)
	if !ok || !d.InDeck || d.Status != StatusSupported {
		return nil, false
	}
	if !restorationEligible(state.Field, catalog, state.Config, id) {
		return nil, false
	}
	for _, r := range state.Field {
		if r.DefinitionID == id && !r.Removed {
			return nil, false
		}
	}
	field := append(append([]RuleInstance(nil), state.Field...), RuleInstance{
		InstanceID: fmt.Sprintf("rule:endless:%d", len(state.Field)), DefinitionID: id,
		Sequence: len(state.Field), IntroducedAfterHand: state.CompletedHands,
	})
	resolved, err := ResolveRules(field, catalog, state.Config)
	if err != nil || !resolved.History[len(field)-1].Active {
		return nil, false
	}
	if err = resolved.ValidateMatchEffects(state.Config); err != nil {
		return nil, false
	}
	if _, err = HandPlanForRules(state.Config, len(state.Seats), resolved); err != nil {
		return nil, false
	}
	if !preservesDependencies(state.Field, field, catalog, state.Config) {
		return nil, false
	}
	return field, true
}

func preservesDependencies(before, after []RuleInstance, catalog Catalog, config Config) bool {
	old, err := ResolveRules(before, catalog, config)
	if err != nil {
		return false
	}
	next, err := ResolveRules(after, catalog, config)
	if err != nil {
		return false
	}
	for i, r := range old.History {
		if !r.Active {
			continue
		}
		d, _ := catalog.Definition(before[i].DefinitionID)
		if len(d.Prerequisites)+len(d.PrerequisiteAny) == 0 {
			continue
		}
		if after[i].Removed {
			continue
		}
		// A directly superseded rule may disappear; an unrelated dependent may not.
		if !next.History[i].Active && next.History[i].InactiveReason == "prerequisite_inactive" {
			return false
		}
	}
	return true
}

func endlessOptions(state MatchState, catalog Catalog) []RuleID {
	options := make([]RuleID, 0, 3)
	if state.Round == 2 {
		for _, id := range []RuleID{RuleLastTrickBonus, RuleAvoidHearts, RuleExpandedHand} {
			if _, ok := eligibleEndlessRule(state, catalog, id); ok {
				options = append(options, id)
			}
		}
	}
	for _, id := range state.RuleDraw {
		if len(options) == 3 {
			break
		}
		found := false
		for _, existing := range options {
			found = found || existing == id
		}
		if found {
			continue
		}
		if _, ok := eligibleEndlessRule(state, catalog, id); ok {
			options = append(options, id)
		}
	}
	return options
}

func applyEndlessRule(state *MatchState, catalog Catalog, id RuleID) bool {
	field, ok := eligibleEndlessRule(*state, catalog, id)
	if !ok {
		return false
	}
	state.Field = field
	for i, draw := range state.RuleDraw {
		if draw == id {
			state.RuleDraw = append(state.RuleDraw[:i], state.RuleDraw[i+1:]...)
			break
		}
	}
	return true
}

func retireEndlessRule(state *MatchState, catalog Catalog) (RuleID, bool) {
	for i := len(state.Field) - 1; i >= 0; i-- {
		instance := state.Field[i]
		if instance.Starting || instance.Removed {
			continue
		}
		candidate := append([]RuleInstance(nil), state.Field...)
		candidate[i].Removed = true
		resolved, err := ResolveRules(candidate, catalog, state.Config)
		if err != nil || resolved.ValidateMatchEffects(state.Config) != nil {
			continue
		}
		if _, err := HandPlanForRules(state.Config, len(state.Seats), resolved); err != nil {
			continue
		}
		if !preservesDependencies(state.Field, candidate, catalog, state.Config) {
			continue
		}
		state.Field = candidate
		return instance.DefinitionID, true
	}
	return "", false
}

func settleEndlessBoundary(state *MatchState, catalog Catalog) error {
	length := endlessRoundLength(*state)
	if state.RoundHand < length {
		state.TableCue = endlessCue(*state, fmt.Sprintf("Round %d: %d hand(s) until the next round.", state.Round, length-state.RoundHand))
		return nil
	}
	if state.ShortGameRound == shortGameRounds {
		finishShortGame(state)
	}
	state.Round++
	state.RoundHand = 0
	position := endlessCyclePosition(state.Round, state.Config)
	if position == openRounds(state.Config)+1 {
		state.ShortGameNumber++
		state.ShortGameRound = 1
		state.ShortGameScores = make(map[SeatID]int, len(state.Seats))
		for _, seat := range state.Seats {
			state.ShortGameScores[seat.ID] = 0
		}
		state.ShortGameResult = nil
	} else if state.ShortGameRound > 0 {
		state.ShortGameRound++
	}
	state.PendingBoundaryID = ""
	state.DraftOptions = nil
	_, event := EndlessRound(state.Round)
	switch event {
	case "draft":
		state.DraftOptions = endlessOptions(*state, catalog)
		if len(state.DraftOptions) > 0 {
			state.PendingBoundaryID = fmt.Sprintf("round:%d:after:%d", state.Round, state.CompletedHands)
			state.Phase = PhaseAwaitingChoice
			state.TableCue = endlessCue(*state, fmt.Sprintf("Round %d: choose a rule or keep the field.", state.Round))
			return nil
		}
		state.TableCue = fmt.Sprintf("Round %d: no eligible change; rules stay the same.", state.Round)
	case "table":
		if state.Round == 2 {
			if !applyEndlessRule(state, catalog, RuleExpandedHand) {
				return fmt.Errorf("opening Expanded Hand rule is unavailable")
			}
			definition, _ := catalog.Definition(RuleExpandedHand)
			state.TableCue = endlessCue(*state, "The table added "+definition.Title+".")
			return nil
		}
		for _, id := range state.RuleDraw {
			if applyEndlessRule(state, catalog, id) {
				definition, _ := catalog.Definition(id)
				state.TableCue = endlessCue(*state, "The table added "+definition.Title+".")
				return nil
			}
		}
		if id, ok := retireEndlessRule(state, catalog); ok {
			definition, _ := catalog.Definition(id)
			state.TableCue = "The table retired " + definition.Title + "."
		} else {
			state.TableCue = "The table found no legal change; rules stay the same."
		}
	case "hold":
		state.TableCue = fmt.Sprintf("Round %d: rules stay the same.", state.Round)
	}
	state.TableCue = endlessCue(*state, state.TableCue)
	return nil
}

func stepChooseRule(state MatchState, action Action, seatIndex int, catalog Catalog) (MatchState, []Event, error) {
	if state.Config.Mode != ModeEndless || state.Phase != PhaseAwaitingChoice {
		return state, nil, transitionError(ErrWrongPhase, "no rule choice is pending")
	}
	if seatIndex != 0 {
		return state, nil, transitionError(ErrNotHost, "only the runner may choose")
	}
	if action.BoundaryID == "" || action.BoundaryID != state.PendingBoundaryID {
		return state, nil, transitionError(ErrInvalidState, "stale round boundary")
	}
	next := cloneState(state)
	if action.Rule != "" {
		valid := false
		for _, id := range state.DraftOptions {
			valid = valid || id == action.Rule
		}
		if !valid || !applyEndlessRule(&next, catalog, action.Rule) {
			return state, nil, transitionError(ErrInvalidState, "rule is not an eligible choice")
		}
		definition, _ := catalog.Definition(action.Rule)
		next.TableCue = endlessCue(next, "You chose "+definition.Title+".")
	} else {
		next.TableCue = endlessCue(next, "You kept the current rules.")
	}
	next.PendingBoundaryID, next.DraftOptions = "", nil
	next.Phase = PhaseHandComplete
	next.Revision++
	return next, []Event{{Kind: EventRoundDecision, Revision: next.Revision, Hand: next.HandNumber, Rule: action.Rule}}, nil
}
