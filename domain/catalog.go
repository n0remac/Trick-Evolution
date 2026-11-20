package domain

import "fmt"

type RuleID string
type EffectKind string
type RuleTiming string
type RuleStatus string
type FieldCategory string
type Mechanic string
type EffectValue string
type ConfigKey string

const (
	FieldPlay                 FieldCategory   = "play"
	FieldTrick                FieldCategory   = "trick"
	FieldScoreVictory         FieldCategory   = "score_victory"
	FieldHand                 FieldCategory   = "hand"
	Visible                   FieldVisibility = "visible"
	Hidden                    FieldVisibility = "hidden"
	NotStarting               FieldVisibility = "not_starting"
	MechanicDeck              Mechanic        = "deck"
	MechanicHandSize          Mechanic        = "hand_size"
	MechanicExpandedHand      Mechanic        = "expanded_hand"
	MechanicKeepOne           Mechanic        = "keep_one_card"
	MechanicUnused            Mechanic        = "unused_cards"
	MechanicScoreOwnership    Mechanic        = "score_ownership"
	MechanicTurnOrder         Mechanic        = "turn_order"
	MechanicFirstLeader       Mechanic        = "first_leader"
	MechanicNextLeader        Mechanic        = "next_leader"
	MechanicTrump             Mechanic        = "trump"
	MechanicBreakSpades       Mechanic        = "break_spades"
	MechanicBreakHearts       Mechanic        = "break_hearts"
	MechanicPlayRestriction   Mechanic        = "play_restriction"
	MechanicWinnerEligibility Mechanic        = "winner_eligibility"
	MechanicRankComparison    Mechanic        = "rank_comparison"
	MechanicTieBreak          Mechanic        = "tie_break"
	MechanicTrickPoints       Mechanic        = "trick_points"
	MechanicHighTrump         Mechanic        = "high_trump_score"
	MechanicLowTrump          Mechanic        = "low_trump_score"
	MechanicLowOwner          Mechanic        = "low_trump_owner"
	MechanicJackTrump         Mechanic        = "jack_trump_score"
	MechanicGamePoint         Mechanic        = "game_point"
	MechanicAvoidQueenHearts  Mechanic        = "avoid_queen_hearts"
	MechanicAvoidHearts       Mechanic        = "avoid_hearts"
	MechanicLastTrickBonus    Mechanic        = "last_trick_bonus"
	MechanicHandLimit         Mechanic        = "hand_limit"
	MechanicVictory           Mechanic        = "victory"
	MechanicReveal            Mechanic        = "reveal"
	ValueStandardDeck         EffectValue     = "standard_52"
	ValueConfigured           EffectValue     = "configured"
	ValuePlusTwo              EffectValue     = "plus_two"
	ValueOutOfPlay            EffectValue     = "out_of_play"
	ValueIndividual           EffectValue     = "individual"
	ValueClockwise            EffectValue     = "clockwise"
	ValueLeftOfDealer         EffectValue     = "left_of_dealer_rotating"
	ValueWinnerLeads          EffectValue     = "winner_leads"
	ValueNoTrump              EffectValue     = "none"
	ValueSpadesTrump          EffectValue     = "spades"
	ValueFirstLeadTrump       EffectValue     = "first_lead"
	ValueBreakSpades          EffectValue     = "spades_unbroken"
	ValueBreakHearts          EffectValue     = "hearts_unbroken"
	ValueFollow               EffectValue     = "follow_suit"
	ValueAnySuit              EffectValue     = "any_suit"
	ValueFollowOrTrump        EffectValue     = "follow_or_trump"
	ValueLeadSuit             EffectValue     = "lead_suit"
	ValueHigh                 EffectValue     = "high_ace_to_two"
	ValueLow                  EffectValue     = "low_two_to_ace"
	ValueFirstPlay            EffectValue     = "first_play"
	ValueOnePoint             EffectValue     = "one_per_trick"
	ValueNoPoints             EffectValue     = "no_points"
	ValueAward                EffectValue     = "award"
	ValueCaptureLow           EffectValue     = "capture_low"
	ValueKeepLow              EffectValue     = "keep_low"
	ValueHighestShared        EffectValue     = "highest_shared_ties"
)

type FieldVisibility string
type TypedEffect struct {
	Mechanic Mechanic
	Value    EffectValue
	Config   ConfigKey
}

const (
	RuleHighCardWins      RuleID     = "rule.high_card_wins"
	RuleFollowSuit        RuleID     = "rule.follow_suit"
	RuleLowCardWins       RuleID     = "rule.low_card_wins"
	RuleNoFollowRequired  RuleID     = "rule.no_follow_required"
	RuleSpadesTrump       RuleID     = "rule.spades_are_trump"
	RulePitchTrump        RuleID     = "rule.pitch_the_trump"
	RuleFollowOrTrump     RuleID     = "rule.follow_suit_or_trump"
	RuleBreakSpades       RuleID     = "rule.break_spades"
	RuleBreakHearts       RuleID     = "rule.break_hearts"
	RuleHighTrump         RuleID     = "rule.high_trump_scores"
	RuleLowTrump          RuleID     = "rule.low_trump_scores"
	RuleCaptureLow        RuleID     = "rule.capture_low"
	RuleKeepLow           RuleID     = "rule.keep_low"
	RuleJackTrump         RuleID     = "rule.trump_jack_scores"
	RuleGamePoint         RuleID     = "rule.game_point"
	RuleAvoidQueenHearts  RuleID     = "rule.avoid_queen_hearts"
	RuleAvoidHearts       RuleID     = "rule.avoid_hearts"
	RuleLastTrickBonus    RuleID     = "rule.last_trick_bonus"
	RuleExpandedHand      RuleID     = "rule.expanded_hand"
	RuleKeepOne           RuleID     = "rule.keep_one_card"
	RuleNoPoints          RuleID     = "rule.no_points_for_tricks"
	RulePointsPerTrick    RuleID     = "rule.points_per_trick"
	RuleDeckStandard      RuleID     = "baseline.standard_deck"
	RuleHandSize          RuleID     = "baseline.hand_size"
	RuleUnusedCards       RuleID     = "baseline.unused_cards"
	RuleIndividualScore   RuleID     = "baseline.individual_score"
	RuleClockwise         RuleID     = "baseline.clockwise"
	RuleFirstLeader       RuleID     = "baseline.first_leader"
	RuleWinnerLeads       RuleID     = "baseline.winner_leads"
	RuleNoTrump           RuleID     = "baseline.no_trump"
	RuleTieBreak          RuleID     = "baseline.tie_break"
	RuleTrickPoint        RuleID     = "baseline.trick_point"
	RuleDefaultCaptureLow RuleID     = "baseline.capture_low"
	RuleHandLimit         RuleID     = "baseline.hand_limit"
	RuleVictory           RuleID     = "baseline.victory"
	RuleReveal            RuleID     = "baseline.reveal"
	StatusSupported       RuleStatus = "supported"
	StatusUnsupported     RuleStatus = "unsupported"
	TimingPlay            RuleTiming = "play_validation"
	TimingTrick           RuleTiming = "trick_award"
	EffectHighLead        EffectKind = "highest_lead_suit"
	EffectLowLead         EffectKind = "lowest_lead_suit"
	EffectFollow          SuitEffect = "follow_suit"
	EffectAnySuit         SuitEffect = "any_suit"
)

// SuitEffect is deliberately restricted to the two supported play restrictions.
type SuitEffect string

type RuleParameters struct {
	LeadSuitOnly bool
}

type RuleDefinition struct {
	ID                 RuleID
	Title              string
	Text               string
	Category           string // rule or phase, matching the historical print catalog.
	Status             RuleStatus
	UnsupportedReason  string
	Timing             RuleTiming
	WinnerEffect       EffectKind
	PlayEffect         SuitEffect
	Parameters         RuleParameters
	Overrides          []RuleID
	InDeck             bool
	FieldCategory      FieldCategory
	Effects            []TypedEffect
	Prerequisites      []RuleID
	PrerequisiteAny    []RuleID // At least one named controller must be active.
	StartActive        bool
	StartingVisibility FieldVisibility
}

type Catalog struct {
	Version     int
	Definitions []RuleDefinition
}

const CatalogVersion = 7

func (c Catalog) Definition(id RuleID) (RuleDefinition, bool) {
	for _, d := range c.Definitions {
		if d.ID == id {
			return d, true
		}
	}
	return RuleDefinition{}, false
}

func (c Catalog) RuleDeck() []RuleID {
	var deck []RuleID
	for _, d := range c.Definitions {
		if d.InDeck {
			deck = append(deck, d.ID)
		}
	}
	return deck
}

func (c Catalog) Validate() error {
	if c.Version != CatalogVersion {
		return fmt.Errorf("unsupported catalog version %d", c.Version)
	}
	if len(c.Definitions) == 0 {
		return fmt.Errorf("empty catalog")
	}
	byID := map[RuleID]RuleDefinition{}
	for _, d := range c.Definitions {
		if d.ID == "" || d.Title == "" || d.Text == "" {
			return fmt.Errorf("rule ID, title and text are required")
		}
		if _, exists := byID[d.ID]; exists {
			return fmt.Errorf("duplicate rule ID %q", d.ID)
		}
		byID[d.ID] = d
		if d.Category != "rule" && d.Category != "phase" {
			return fmt.Errorf("%s: unknown category", d.ID)
		}
		switch d.Status {
		case StatusUnsupported:
			if d.UnsupportedReason == "" || d.InDeck || d.Timing != "" || d.WinnerEffect != "" || d.PlayEffect != "" || d.Parameters != (RuleParameters{}) || len(d.Overrides) != 0 || len(d.Effects) != 0 || len(d.Prerequisites) != 0 || len(d.PrerequisiteAny) != 0 || d.StartActive {
				return fmt.Errorf("%s: unsupported entry must have a reason and no executable behavior", d.ID)
			}
		case StatusSupported:
			if d.Category != "rule" || d.UnsupportedReason != "" || len(d.Effects) == 0 || (d.InDeck && d.StartActive) ||
				(d.StartActive && d.StartingVisibility == NotStarting) ||
				(!d.StartActive && d.StartingVisibility != NotStarting) ||
				(d.StartingVisibility != Visible && d.StartingVisibility != Hidden && d.StartingVisibility != NotStarting) ||
				(d.FieldCategory != FieldPlay && d.FieldCategory != FieldTrick && d.FieldCategory != FieldHand && d.FieldCategory != FieldScoreVictory) {
				return fmt.Errorf("%s: invalid supported entry", d.ID)
			}
			mechanics := map[Mechanic]bool{}
			for _, effect := range d.Effects {
				if !validTypedEffect(effect) || mechanics[effect.Mechanic] {
					return fmt.Errorf("%s: invalid or duplicate typed effect", d.ID)
				}
				mechanics[effect.Mechanic] = true
			}
			if d.Timing == "" && d.WinnerEffect == "" && d.PlayEffect == "" {
				break
			}
			switch d.Timing {
			case TimingPlay:
				want := ValueFollow
				if d.PlayEffect == EffectAnySuit {
					want = ValueAnySuit
				}
				if d.WinnerEffect != "" || d.Parameters != (RuleParameters{}) || (d.PlayEffect != EffectFollow && d.PlayEffect != EffectAnySuit) ||
					!reflectTypedEffect(d.Effects, MechanicPlayRestriction, want) || !reflectTypedEffect(d.Effects, MechanicWinnerEligibility, ValueLeadSuit) || len(d.Effects) != 2 {
					return fmt.Errorf("%s: invalid play effect or parameters", d.ID)
				}
			case TimingTrick:
				want := ValueHigh
				if d.WinnerEffect == EffectLowLead {
					want = ValueLow
				}
				if d.PlayEffect != "" || d.Parameters.LeadSuitOnly || (d.WinnerEffect != EffectHighLead && d.WinnerEffect != EffectLowLead) ||
					!reflectTypedEffect(d.Effects, MechanicRankComparison, want) || len(d.Effects) != 1 {
					return fmt.Errorf("%s: invalid winner effect or parameters", d.ID)
				}
			default:
				return fmt.Errorf("%s: unknown timing", d.ID)
			}
		default:
			return fmt.Errorf("%s: unknown support status", d.ID)
		}
	}
	for _, d := range c.Definitions {
		seen := map[RuleID]bool{}
		for _, target := range d.Overrides {
			old, ok := byID[target]
			if !ok || old.Status != StatusSupported {
				return fmt.Errorf("%s: unknown or unsupported override target %s", d.ID, target)
			}
			if target == d.ID || seen[target] {
				return fmt.Errorf("%s: self or duplicate override", d.ID)
			}
			seen[target] = true
			shared := false
			for _, a := range d.Effects {
				for _, b := range old.Effects {
					if a.Mechanic == b.Mechanic {
						shared = true
					}
				}
			}
			if !shared {
				return fmt.Errorf("%s: override target has no shared mechanic", d.ID)
			}
		}
		prerequisites := map[RuleID]bool{}
		for _, prerequisite := range append(append([]RuleID(nil), d.Prerequisites...), d.PrerequisiteAny...) {
			if prerequisites[prerequisite] {
				return fmt.Errorf("%s: duplicate prerequisite %s", d.ID, prerequisite)
			}
			prerequisites[prerequisite] = true
			old, ok := byID[prerequisite]
			if !ok || old.Status != StatusSupported || prerequisite == d.ID {
				return fmt.Errorf("%s: invalid prerequisite %s", d.ID, prerequisite)
			}
			for _, effect := range d.Effects {
				for _, prior := range old.Effects {
					if effect.Mechanic == prior.Mechanic {
						return fmt.Errorf("%s: prerequisite replaces its own mechanic", d.ID)
					}
				}
			}
		}
	}
	visiting := map[RuleID]bool{}
	visited := map[RuleID]bool{}
	var checkCycle func(RuleID) error
	checkCycle = func(id RuleID) error {
		if visiting[id] {
			return fmt.Errorf("prerequisite cycle at %s", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, p := range append(append([]RuleID(nil), byID[id].Prerequisites...), byID[id].PrerequisiteAny...) {
			if err := checkCycle(p); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		return nil
	}
	for id := range byID {
		if err := checkCycle(id); err != nil {
			return err
		}
	}
	return nil
}

func reflectTypedEffect(effects []TypedEffect, mechanic Mechanic, value EffectValue) bool {
	for _, effect := range effects {
		if effect.Mechanic == mechanic && effect.Value == value {
			return true
		}
	}
	return false
}

func supported(id RuleID, title, text string, timing RuleTiming, winner EffectKind, play SuitEffect, inDeck bool, overrides ...RuleID) RuleDefinition {
	d := RuleDefinition{ID: id, Title: title, Text: text, Category: "rule", Status: StatusSupported,
		Timing: timing, WinnerEffect: winner, PlayEffect: play, InDeck: inDeck, Overrides: overrides, StartingVisibility: NotStarting}
	if timing == TimingTrick {
		d.FieldCategory = FieldTrick
		value := ValueHigh
		if winner == EffectLowLead {
			value = ValueLow
		}
		d.Effects = []TypedEffect{{MechanicRankComparison, value, ""}}
	} else {
		d.FieldCategory = FieldPlay
		value := ValueFollow
		if play == EffectAnySuit {
			value = ValueAnySuit
		}
		d.Effects = []TypedEffect{{MechanicPlayRestriction, value, ""}, {MechanicWinnerEligibility, ValueLeadSuit, ""}}
	}
	return d
}

func baseline(id RuleID, title, text string, category FieldCategory, effect TypedEffect) RuleDefinition {
	return RuleDefinition{ID: id, Title: title, Text: text, Category: "rule", Status: StatusSupported, FieldCategory: category, Effects: []TypedEffect{effect}, StartActive: true, StartingVisibility: Hidden}
}

func validTypedEffect(e TypedEffect) bool {
	values := map[Mechanic]EffectValue{MechanicDeck: ValueStandardDeck, MechanicHandSize: ValueConfigured, MechanicExpandedHand: ValuePlusTwo, MechanicKeepOne: ValueAward, MechanicUnused: ValueOutOfPlay, MechanicScoreOwnership: ValueIndividual, MechanicTurnOrder: ValueClockwise, MechanicFirstLeader: ValueLeftOfDealer, MechanicNextLeader: ValueWinnerLeads, MechanicTrump: ValueNoTrump, MechanicBreakSpades: ValueBreakSpades, MechanicBreakHearts: ValueBreakHearts, MechanicPlayRestriction: ValueFollow, MechanicWinnerEligibility: ValueLeadSuit, MechanicRankComparison: ValueHigh, MechanicTieBreak: ValueFirstPlay, MechanicTrickPoints: ValueOnePoint, MechanicLowOwner: ValueCaptureLow, MechanicHighTrump: ValueAward, MechanicLowTrump: ValueAward, MechanicJackTrump: ValueAward, MechanicGamePoint: ValueAward, MechanicAvoidQueenHearts: ValueAward, MechanicAvoidHearts: ValueAward, MechanicLastTrickBonus: ValueAward, MechanicHandLimit: ValueConfigured, MechanicVictory: ValueHighestShared, MechanicReveal: ValueConfigured}
	want, ok := values[e.Mechanic]
	if !ok {
		return false
	}
	if e.Mechanic == MechanicPlayRestriction {
		return (e.Value == ValueFollow || e.Value == ValueAnySuit || e.Value == ValueFollowOrTrump) && e.Config == ""
	}
	if e.Mechanic == MechanicTrump {
		return (e.Value == ValueNoTrump || e.Value == ValueSpadesTrump || e.Value == ValueFirstLeadTrump) && e.Config == ""
	}
	if e.Mechanic == MechanicRankComparison {
		return (e.Value == ValueHigh || e.Value == ValueLow) && e.Config == ""
	}
	if e.Mechanic == MechanicTrickPoints {
		return (e.Value == ValueOnePoint || e.Value == ValueNoPoints) && e.Config == ""
	}
	if e.Mechanic == MechanicLowOwner {
		return (e.Value == ValueCaptureLow || e.Value == ValueKeepLow) && e.Config == ""
	}
	if e.Value != want {
		return false
	}
	switch e.Mechanic {
	case MechanicHandSize:
		return e.Config == "hand_size"
	case MechanicHandLimit:
		return e.Config == "max_hands"
	case MechanicReveal:
		return e.Config == "reveal_every"
	default:
		return e.Config == ""
	}
}

func unsupported(id RuleID, title, text, category, reason string) RuleDefinition {
	return RuleDefinition{ID: id, Title: title, Text: text, Category: category, Status: StatusUnsupported, UnsupportedReason: reason}
}

// DefaultCatalog keeps the original printed wording as design material. Only
// entries with explicit typed effects can enter the executable rule deck.
func DefaultCatalog() Catalog {
	const unmodeled = "requires rule semantics or state outside the F1 subset"
	const bids = "requires bids or team state not modeled in F1"
	follow := supported(RuleFollowSuit, "Follow Suit", "You must play the lead suit if able. Only lead-suit cards can win without trump.", TimingPlay, "", EffectFollow, false, RuleNoFollowRequired)
	follow.StartActive, follow.StartingVisibility = true, Visible
	high := supported(RuleHighCardWins, "High Card Wins", "Among eligible cards, highest rank wins (ace high).", TimingTrick, EffectHighLead, "", false, RuleLowCardWins)
	high.StartActive, high.StartingVisibility = true, Visible
	return Catalog{Version: CatalogVersion, Definitions: []RuleDefinition{
		baseline(RuleDeckStandard, "Standard Deck", "Use a standard 52-card deck.", FieldHand, TypedEffect{MechanicDeck, ValueStandardDeck, ""}),
		baseline(RuleHandSize, "Hand Size", "The configured cards per player are the base deal size before hand modifiers.", FieldHand, TypedEffect{MechanicHandSize, ValueConfigured, "hand_size"}),
		{ID: RuleExpandedHand, Title: "Expanded Hand Size", Text: "While active, add two to the configured cards per player at each deal; the increase does not accumulate. A reveal first affects the next deal. The full deal must fit 52 cards.", Category: "rule", Status: StatusSupported, FieldCategory: FieldHand, Effects: []TypedEffect{{Mechanic: MechanicExpandedHand, Value: ValuePlusTwo}}, InDeck: true, StartingVisibility: NotStarting},
		{ID: RuleKeepOne, Title: "Keep One Card", Text: "Stop each hand with one unplayed card per player. Your last card stays private and joins your next hand; deal one fewer new card then. A final-hand leftover is not captured or scored as a capture.", Category: "rule", Status: StatusSupported, FieldCategory: FieldHand, Effects: []TypedEffect{{Mechanic: MechanicKeepOne, Value: ValueAward}}, InDeck: true, StartingVisibility: NotStarting},
		baseline(RuleUnusedCards, "Unused Cards", "Cards not dealt are out of play for this hand.", FieldHand, TypedEffect{MechanicUnused, ValueOutOfPlay, ""}),
		baseline(RuleIndividualScore, "Individual Scores", "Each player has an individual score.", FieldScoreVictory, TypedEffect{MechanicScoreOwnership, ValueIndividual, ""}),
		baseline(RuleClockwise, "Clockwise Play", "Play proceeds clockwise.", FieldPlay, TypedEffect{MechanicTurnOrder, ValueClockwise, ""}),
		baseline(RuleFirstLeader, "First Leader", "Player left of the dealer leads; dealer rotates each hand.", FieldPlay, TypedEffect{MechanicFirstLeader, ValueLeftOfDealer, ""}),
		baseline(RuleWinnerLeads, "Winner Leads", "Trick winner leads the next trick.", FieldTrick, TypedEffect{MechanicNextLeader, ValueWinnerLeads, ""}),
		baseline(RuleNoTrump, "No Trump", "No suit is trump.", FieldTrick, TypedEffect{MechanicTrump, ValueNoTrump, ""}),
		baseline(RuleTieBreak, "Deterministic Ties", "Earlier play wins an equal-rank tie.", FieldTrick, TypedEffect{MechanicTieBreak, ValueFirstPlay, ""}),
		baseline(RuleTrickPoint, "One Point per Trick", "Each captured trick scores one point.", FieldScoreVictory, TypedEffect{MechanicTrickPoints, ValueOnePoint, ""}),
		baseline(RuleDefaultCaptureLow, "Capture Low", "When Low Trump Scores is active, the captor owns its point unless Keep Low replaces this rule.", FieldScoreVictory, TypedEffect{MechanicLowOwner, ValueCaptureLow, ""}),
		baseline(RuleHandLimit, "Fixed Hand Limit", "End after the configured number of hands.", FieldScoreVictory, TypedEffect{MechanicHandLimit, ValueConfigured, "max_hands"}),
		baseline(RuleVictory, "Highest Score Wins", "Highest score wins; tied leaders share victory.", FieldScoreVictory, TypedEffect{MechanicVictory, ValueHighestShared, ""}),
		baseline(RuleReveal, "Rule Reveal", "Reveal at the configured interval between hands if an eligible card remains.", FieldPlay, TypedEffect{MechanicReveal, ValueConfigured, "reveal_every"}),
		{ID: RuleSpadesTrump, Title: "Spades Are Trump", Text: "Spades beat nontrump cards. If no trump is played, the lead suit wins.", Category: "rule", Status: StatusSupported, FieldCategory: FieldTrick, Effects: []TypedEffect{{Mechanic: MechanicTrump, Value: ValueSpadesTrump}}, Overrides: []RuleID{RuleNoTrump, RulePitchTrump}, InDeck: true, StartingVisibility: NotStarting},
		{ID: RulePitchTrump, Title: "Pitch the Trump", Text: "The first card led each hand chooses its trump suit, including the first trick.", Category: "rule", Status: StatusSupported, FieldCategory: FieldTrick, Effects: []TypedEffect{{Mechanic: MechanicTrump, Value: ValueFirstLeadTrump}}, Overrides: []RuleID{RuleNoTrump, RuleSpadesTrump}, InDeck: true, StartingVisibility: NotStarting},
		{ID: RuleFollowOrTrump, Title: "Follow Suit or Trump", Text: "Follow suit or play trump when holding the lead suit. On a trump lead, follow trump if able.", Category: "rule", Status: StatusSupported, FieldCategory: FieldPlay, Effects: []TypedEffect{{Mechanic: MechanicPlayRestriction, Value: ValueFollowOrTrump}}, Overrides: []RuleID{RuleFollowSuit, RuleNoFollowRequired}, PrerequisiteAny: []RuleID{RuleSpadesTrump, RulePitchTrump}, InDeck: true, StartingVisibility: NotStarting},
		{ID: RuleBreakSpades, Title: "Break Spades", Text: "Do not lead spades until a spade has been played on another suit lead, unless only spades remain.", Category: "rule", Status: StatusSupported, FieldCategory: FieldPlay, Effects: []TypedEffect{{Mechanic: MechanicBreakSpades, Value: ValueBreakSpades}}, Prerequisites: []RuleID{RuleSpadesTrump}, InDeck: true, StartingVisibility: NotStarting},
		{ID: RuleBreakHearts, Title: "Break Hearts", Text: "Do not lead hearts until a heart has been played on another suit lead, unless only hearts remain.", Category: "rule", Status: StatusSupported, FieldCategory: FieldPlay, Effects: []TypedEffect{{Mechanic: MechanicBreakHearts, Value: ValueBreakHearts}}, InDeck: true, StartingVisibility: NotStarting},
		{ID: RuleHighTrump, Title: "High Trump Scores", Text: "At hand end, the original holder of the highest dealt trump scores one point (ace high). Requires trump.", Category: "rule", Status: StatusSupported, FieldCategory: FieldScoreVictory, Effects: []TypedEffect{{Mechanic: MechanicHighTrump, Value: ValueAward}}, PrerequisiteAny: []RuleID{RuleSpadesTrump, RulePitchTrump}, InDeck: true, StartingVisibility: NotStarting},
		{ID: RuleLowTrump, Title: "Low Trump Scores", Text: "At hand end, the lowest dealt trump scores one point for its captor by default. Keep Low changes ownership. Requires trump.", Category: "rule", Status: StatusSupported, FieldCategory: FieldScoreVictory, Effects: []TypedEffect{{Mechanic: MechanicLowTrump, Value: ValueAward}}, PrerequisiteAny: []RuleID{RuleSpadesTrump, RulePitchTrump}, InDeck: true, StartingVisibility: NotStarting},
		{ID: RuleCaptureLow, Title: "Capture Low", Text: "The captor of the lowest dealt trump gets its point. Replaces Keep Low.", Category: "rule", Status: StatusSupported, FieldCategory: FieldScoreVictory, Effects: []TypedEffect{{Mechanic: MechanicLowOwner, Value: ValueCaptureLow}}, Overrides: []RuleID{RuleDefaultCaptureLow, RuleKeepLow}, Prerequisites: []RuleID{RuleLowTrump}, InDeck: true, StartingVisibility: NotStarting},
		{ID: RuleKeepLow, Title: "Keep Low", Text: "The original holder of the lowest dealt trump gets its point. Replaces Capture Low.", Category: "rule", Status: StatusSupported, FieldCategory: FieldScoreVictory, Effects: []TypedEffect{{Mechanic: MechanicLowOwner, Value: ValueKeepLow}}, Overrides: []RuleID{RuleDefaultCaptureLow, RuleCaptureLow}, Prerequisites: []RuleID{RuleLowTrump}, InDeck: true, StartingVisibility: NotStarting},
		{ID: RuleJackTrump, Title: "Trump Jack Scores", Text: "At hand end, the captor of the dealt trump Jack scores one point. Requires trump.", Category: "rule", Status: StatusSupported, FieldCategory: FieldScoreVictory, Effects: []TypedEffect{{Mechanic: MechanicJackTrump, Value: ValueAward}}, PrerequisiteAny: []RuleID{RuleSpadesTrump, RulePitchTrump}, InDeck: true, StartingVisibility: NotStarting},
		{ID: RuleGamePoint, Title: "Game Point", Text: "At hand end, the unique highest captured-card tally scores one point: 10=10, A=4, K=3, Q=2, J=1. Ties score nothing.", Category: "rule", Status: StatusSupported, FieldCategory: FieldScoreVictory, Effects: []TypedEffect{{Mechanic: MechanicGamePoint, Value: ValueAward}}, InDeck: true, StartingVisibility: NotStarting},
		{ID: RuleNoPoints, Title: "No Points for Tricks", Text: "Captured tricks give no ordinary trick point; other scoring rules still apply.", Category: "rule", Status: StatusSupported, FieldCategory: FieldScoreVictory, Effects: []TypedEffect{{Mechanic: MechanicTrickPoints, Value: ValueNoPoints}}, Overrides: []RuleID{RuleTrickPoint, RulePointsPerTrick}, InDeck: true, StartingVisibility: NotStarting},
		{ID: RulePointsPerTrick, Title: "Points per Trick", Text: "Each captured trick scores one point. Replaces No Points for Tricks.", Category: "rule", Status: StatusSupported, FieldCategory: FieldScoreVictory, Effects: []TypedEffect{{Mechanic: MechanicTrickPoints, Value: ValueOnePoint}}, Overrides: []RuleID{RuleTrickPoint, RuleNoPoints}, InDeck: true, StartingVisibility: NotStarting},
		{ID: RuleAvoidQueenHearts, Title: "Avoid Queen of Hearts", Text: "Capturing the Queen of Hearts costs 5 match points when its trick completes. Stacks with Avoid Hearts; undealt or unplayed cards do not score.", Category: "rule", Status: StatusSupported, FieldCategory: FieldScoreVictory, Effects: []TypedEffect{{Mechanic: MechanicAvoidQueenHearts, Value: ValueAward}}, InDeck: true, StartingVisibility: NotStarting},
		{ID: RuleAvoidHearts, Title: "Avoid Hearts", Text: "Each heart captured in a completed trick costs 1 match point, including the Queen of Hearts. Stacks with other scoring rules.", Category: "rule", Status: StatusSupported, FieldCategory: FieldScoreVictory, Effects: []TypedEffect{{Mechanic: MechanicAvoidHearts, Value: ValueAward}}, InDeck: true, StartingVisibility: NotStarting},
		{ID: RuleLastTrickBonus, Title: "Last Trick Bonus", Text: "The winner of the final completed trick of a completed hand gains 2 match points. An unfinished hand earns no bonus.", Category: "rule", Status: StatusSupported, FieldCategory: FieldScoreVictory, Effects: []TypedEffect{{Mechanic: MechanicLastTrickBonus, Value: ValueAward}}, InDeck: true, StartingVisibility: NotStarting},
		follow,
		supported(RuleNoFollowRequired, "No Follow Required", "Play any card even if you hold the lead suit. Only lead-suit cards can win without trump.", TimingPlay, "", EffectAnySuit, false, RuleFollowSuit),
		unsupported("rule.must_underplay", "Must Underplay", "If able, you must play a lower card than the lead.", "rule", unmodeled),
		unsupported("rule.must_beat_lead", "Must Beat Lead", "If able, you must play a higher card than the lead.", "rule", unmodeled),
		unsupported("rule.reverse_play_order", "Reverse Play Order", "Play proceeds counter-clockwise this round.", "rule", unmodeled),
		high,
		supported(RuleLowCardWins, "Low Card Wins", "Among eligible cards, lowest rank wins.", TimingTrick, EffectLowLead, "", false, RuleHighCardWins),
		unsupported("rule.even_card_wins", "Even Card Wins", "Highest even‑ranked card wins the trick.", "rule", unmodeled),
		unsupported("rule.odd_card_wins", "Odd Card Wins", "Highest odd‑ranked card wins the trick.", "rule", unmodeled),
		unsupported("rule.hidden_tricks", "Hidden Tricks", "Tricks are kept face-down and revealed at round end.", "rule", unmodeled),
		unsupported("rule.last_trick_wins", "Last Trick Wins", "Only the final trick each round counts for points.", "rule", unmodeled),
		unsupported("rule.no_trump", "No Trump", "No trump suit this round.", "rule", unmodeled),
		unsupported("rule.fixed_trump", "Fixed Trump", "Hearts are trump this round.", "rule", unmodeled),
		unsupported("rule.random_trump", "Random Trump", "Randomly select a trump suit at start of round.", "rule", unmodeled),
		unsupported("rule.lead_chooses_trump", "Lead Chooses Trump", "Leader of first trick selects trump suit.", "rule", unmodeled),
		unsupported("rule.per_turn_trump", "Per Turn Trump", "Leader declares trump suit each trick.", "rule", unmodeled),
		unsupported("rule.jack_is_trump", "Jack is Trump", "All Jacks are trump cards this round.", "rule", unmodeled),
		unsupported("rule.suitless_play", "Suitless Play", "Ignore suits. Ranks determine trick winners.", "rule", unmodeled),
		unsupported("rule.king_kills_ace", "King Kills Ace", "Kings beat Aces in all suits this round.", "rule", unmodeled),
		unsupported("rule.score_by_tricks", "Score by Tricks", "Each trick captured = 1 point.", "rule", "baseline scoring is provisional; printed effect is not a rule draw"),
		unsupported("rule.red_suits_score", "Red Suits Score", "Only tricks containing ♥ or ♦ score.", "rule", unmodeled),
		unsupported("rule.avoid_queen_of_spades", "Avoid Queen of Spades", "Capturing Q♠ is –5 points.", "rule", unmodeled),
		unsupported("rule.bonus_for_jacks", "Bonus for Jacks", "+2 points for each Jack captured.", "rule", unmodeled),
		unsupported("rule.target_trick_count", "Target Trick Count", "Score only if your trick total matches your bid.", "rule", bids),
		unsupported("rule.momentum_bonus", "Momentum Bonus", "+1 point if you win 3+ tricks in a row.", "rule", unmodeled),
		unsupported("rule.chaos_points", "Chaos Points", "+1 point if you win exactly 0 or 5 tricks.", "rule", unmodeled),
		unsupported("rule.random_lead", "Random Lead", "Random player leads each round.", "rule", unmodeled),
		unsupported("rule.winner_leads", "Winner Leads", "Winner of previous trick leads next.", "rule", unmodeled),
		unsupported("rule.fixed_rotation", "Fixed Rotation", "Lead passes clockwise each trick.", "rule", unmodeled),
		unsupported("rule.plus_one_hand_size", "+1 Hand Size", "Draw one extra card at start of round.", "rule", unmodeled),
		unsupported("rule.plus_two_hand_size", "+2 Hand Size", "Draw two extra cards at start of round.", "rule", unmodeled),
		unsupported("rule.minus_one_rule_cards", "-1 Rule Cards", "Discard one active rule card from play.", "rule", unmodeled),
		unsupported("rule.minus_two_rule_cards", "-2 Rule Cards", "Discard two active rule cards from play.", "rule", unmodeled),
		unsupported("phase.carry_one_card", "Carry One Card", "Keep one card from your previous hand to include in your next hand.", "phase", unmodeled),
		unsupported("phase.reveal_one_opponent_card", "Reveal One Opponent Card", "Peek at one random card from an opponent’s hand at round start.", "phase", unmodeled),
		unsupported("phase.partner_across", "Partner Across", "Partner with the player seated directly across from you.", "phase", bids),
		unsupported("phase.bid_for_lead", "Bid for Lead", "Bid the number of tricks you will win. Highest bidder leads. Lose bid if you fail.", "phase", bids),
		unsupported("phase.declare_goals", "Declare Goals", "Announce the number of tricks you aim to win. Lose bid if you fail.", "phase", bids),
		unsupported("phase.two_cards_forward", "Two Cards Forward", "Pass two cards to the player on your left before play begins.", "phase", unmodeled),
		unsupported("phase.swap_one", "Swap One", "Trade one card with the player on your right before the round starts.", "phase", unmodeled),
		unsupported("phase.reward_exact", "Reward Exact", "Gain +1 point if your trick total matches your bid exactly.", "phase", bids),
		unsupported("phase.penalty_for_overbid", "Penalty for Overbid", "Lose 1 point for each trick over your bid.", "phase", bids),
		unsupported("phase.underscore_bonus", "Underscore Bonus", "Gain +1 point if you win fewer tricks than your bid (not zero).", "phase", bids),
		unsupported("phase.carry_points", "Carry Points", "If you hit your bid, save unused bid points for later rounds.", "phase", bids),
		unsupported("phase.solo_challenge", "Solo Challenge", "Declare solo play. Score double if you meet your bid alone.", "phase", bids),
		unsupported("phase.team_bid", "Team Bid", "Partner bids are combined. Both players must hit the total to score.", "phase", bids),
		unsupported("phase.fixed_rounds", "Fixed Rounds", "Game ends after an agreed number of rounds.", "phase", unmodeled),
		unsupported("phase.sudden_death", "Sudden Death", "Game ends after this round. Highest score wins.", "phase", unmodeled),
		unsupported("phase.exact_score_win", "Exact Score Win", "Reach exactly 15 (or 20) points to win. Overshooting resets to 10.", "phase", unmodeled),
		unsupported("phase.exact_or_higher", "Exact or Higher", "Reach 15 (or 20) points or more to win.", "phase", unmodeled),
		unsupported("phase.lowest_score_wins", "Lowest Score Wins", "Player with the fewest points at game end wins.", "phase", unmodeled),
		unsupported("phase.declaring_victory", "Declaring Victory", "You may declare victory before a round. Player at end of round with most tricks wins.", "phase", unmodeled),
		unsupported("phase.shoot_the_moon", "Shoot the Moon", "Declare that you will win all the tricks. If you do, you win. If not, the highest-scoring opponent wins.", "phase", unmodeled),
		unsupported("phase.elimination_rounds", "Elimination Rounds", "Last-place player is eliminated every 3 rounds.", "phase", unmodeled),
		unsupported("phase.sudden_surge", "Sudden Surge", "If any player scores 5 points in a round, game ends immediately.", "phase", unmodeled),
	}}
}
