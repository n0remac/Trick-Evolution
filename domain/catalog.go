package domain

import "fmt"

type RuleID string
type EffectKind string
type RuleTiming string
type RuleStatus string

const (
	RuleHighCardWins     RuleID     = "rule.high_card_wins"
	RuleFollowSuit       RuleID     = "rule.follow_suit"
	RuleLowCardWins      RuleID     = "rule.low_card_wins"
	RuleNoFollowRequired RuleID     = "rule.no_follow_required"
	StatusSupported      RuleStatus = "supported"
	StatusUnsupported    RuleStatus = "unsupported"
	TimingPlay           RuleTiming = "play_validation"
	TimingTrick          RuleTiming = "trick_award"
	EffectHighLead       EffectKind = "highest_lead_suit"
	EffectLowLead        EffectKind = "lowest_lead_suit"
	EffectFollow         SuitEffect = "follow_suit"
	EffectAnySuit        SuitEffect = "any_suit"
)

// SuitEffect is deliberately restricted to the two supported play restrictions.
type SuitEffect string

type RuleParameters struct {
	LeadSuitOnly bool
}

type RuleDefinition struct {
	ID                RuleID
	Title             string
	Text              string
	Category          string // rule or phase, matching the historical print catalog.
	Status            RuleStatus
	UnsupportedReason string
	Timing            RuleTiming
	WinnerEffect      EffectKind
	PlayEffect        SuitEffect
	Parameters        RuleParameters
	Overrides         []RuleID
	InDeck            bool
}

type Catalog struct {
	Version     int
	Definitions []RuleDefinition
}

const CatalogVersion = 1

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
			if d.UnsupportedReason == "" || d.InDeck || d.Timing != "" || d.WinnerEffect != "" || d.PlayEffect != "" || d.Parameters != (RuleParameters{}) || len(d.Overrides) != 0 {
				return fmt.Errorf("%s: unsupported entry must have a reason and no executable behavior", d.ID)
			}
		case StatusSupported:
			if d.Category != "rule" || d.UnsupportedReason != "" {
				return fmt.Errorf("%s: invalid supported entry", d.ID)
			}
			switch d.Timing {
			case TimingPlay:
				if d.WinnerEffect != "" || d.Parameters != (RuleParameters{}) || (d.PlayEffect != EffectFollow && d.PlayEffect != EffectAnySuit) {
					return fmt.Errorf("%s: invalid play effect or parameters", d.ID)
				}
			case TimingTrick:
				if d.PlayEffect != "" || !d.Parameters.LeadSuitOnly || (d.WinnerEffect != EffectHighLead && d.WinnerEffect != EffectLowLead) {
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
			if old.Timing != d.Timing {
				return fmt.Errorf("%s: override target has different timing", d.ID)
			}
		}
	}
	return nil
}

func supported(id RuleID, title, text string, timing RuleTiming, winner EffectKind, play SuitEffect, inDeck bool, overrides ...RuleID) RuleDefinition {
	d := RuleDefinition{ID: id, Title: title, Text: text, Category: "rule", Status: StatusSupported,
		Timing: timing, WinnerEffect: winner, PlayEffect: play, InDeck: inDeck, Overrides: overrides}
	if timing == TimingTrick {
		d.Parameters.LeadSuitOnly = true
	}
	return d
}

func unsupported(id RuleID, title, text, category, reason string) RuleDefinition {
	return RuleDefinition{ID: id, Title: title, Text: text, Category: category, Status: StatusUnsupported, UnsupportedReason: reason}
}

// DefaultCatalog keeps the original printed wording as design material. Only
// entries with explicit typed effects can enter the executable rule deck.
func DefaultCatalog() Catalog {
	const unmodeled = "requires rule semantics or state outside the F1 subset"
	const bids = "requires bids or team state not modeled in F1"
	return Catalog{Version: CatalogVersion, Definitions: []RuleDefinition{
		supported(RuleFollowSuit, "Follow Suit", "You must play the lead suit if able.", TimingPlay, "", EffectFollow, true),
		supported(RuleNoFollowRequired, "No Follow Required", "Play any card even if you hold the lead suit.", TimingPlay, "", EffectAnySuit, true, RuleFollowSuit),
		unsupported("rule.must_underplay", "Must Underplay", "If able, you must play a lower card than the lead.", "rule", unmodeled),
		unsupported("rule.must_beat_lead", "Must Beat Lead", "If able, you must play a higher card than the lead.", "rule", unmodeled),
		unsupported("rule.reverse_play_order", "Reverse Play Order", "Play proceeds counter-clockwise this round.", "rule", unmodeled),
		supported(RuleHighCardWins, "High Card Wins", "Highest card of the lead suit wins the trick.", TimingTrick, EffectHighLead, "", false),
		supported(RuleLowCardWins, "Low Card Wins", "Lowest card of the lead suit wins the trick.", TimingTrick, EffectLowLead, "", true, RuleHighCardWins),
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
