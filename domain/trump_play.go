package domain

import "fmt"

// HandFacts are public facts for the current hand. They are copied into every
// seat projection and sampled rollout world.
type HandFacts struct {
	TrumpSuit    Suit
	BrokenSpades bool
	BrokenHearts bool
}

func factsFromState(state MatchState) HandFacts {
	return HandFacts{state.TrumpSuit, state.BrokenSpades, state.BrokenHearts}
}

func factsFromView(view PublicView) HandFacts {
	return HandFacts{view.TrumpSuit, view.BrokenSpades, view.BrokenHearts}
}

func validSuit(suit Suit) bool {
	return suit == Hearts || suit == Diamonds || suit == Clubs || suit == Spades
}

// TrumpForTrick uses the selected hand trump. Pitch's opening lead supplies it
// for that very trick, including hypothetical leads evaluated by bots.
func TrumpForTrick(rules RuleResolution, selected Suit, plays []Play) (Suit, error) {
	effect, err := rules.Effect(MechanicTrump)
	if err != nil {
		return "", err
	}
	switch effect.Value {
	case ValueNoTrump:
		return "", nil
	case ValueSpadesTrump:
		return Spades, nil
	case ValueTurnUpTrump:
		if selected != "" && !validSuit(selected) {
			return "", fmt.Errorf("invalid selected trump %s", selected)
		}
		return selected, nil
	case ValueFirstLeadTrump:
		if selected != "" {
			if !validSuit(selected) {
				return "", fmt.Errorf("invalid selected trump %s", selected)
			}
			return selected, nil
		}
		if len(plays) > 0 {
			card, ok := cardByID(plays[0].Card)
			if !ok {
				return "", fmt.Errorf("unknown opening card")
			}
			return card.Suit, nil
		}
		return "", nil
	default:
		return "", fmt.Errorf("unsupported trump selection %s", effect.Value)
	}
}

// WinningPlay is the one rank and eligibility comparator for engine and bots.
// Trump eligibility takes precedence; otherwise only the lead suit is eligible.
func WinningPlay(plays []Play, rules RuleResolution, selected Suit) (Play, error) {
	if len(plays) == 0 {
		return Play{}, fmt.Errorf("empty trick")
	}
	eligibility, err := rules.Effect(MechanicWinnerEligibility)
	if err != nil || eligibility.Value != ValueLeadSuit {
		return Play{}, fmt.Errorf("unsupported winner eligibility")
	}
	rank, err := rules.Effect(MechanicRankComparison)
	if err != nil || rank.Value != ValueHigh && rank.Value != ValueLow {
		return Play{}, fmt.Errorf("unsupported rank comparison")
	}
	trump, err := TrumpForTrick(rules, selected, plays)
	if err != nil {
		return Play{}, err
	}
	lead, ok := cardByID(plays[0].Card)
	if !ok {
		return Play{}, fmt.Errorf("unknown lead card")
	}
	eligibleSuit := lead.Suit
	if trump != "" {
		for _, play := range plays {
			card, ok := cardByID(play.Card)
			if !ok {
				return Play{}, fmt.Errorf("unknown played card")
			}
			if card.Suit == trump {
				eligibleSuit = trump
				break
			}
		}
	}
	var winner Play
	var best uint8
	for _, play := range plays {
		card, ok := cardByID(play.Card)
		if !ok {
			return Play{}, fmt.Errorf("unknown played card")
		}
		if card.Suit != eligibleSuit {
			continue
		}
		if winner.Seat == "" || rank.Value == ValueHigh && card.Rank > best || rank.Value == ValueLow && card.Rank < best {
			winner, best = play, card.Rank
		}
	}
	return winner, nil
}

// legalCardsWithRules keeps lead restrictions distinct from following and
// winner eligibility. If both unbroken-suit bans exhaust a lead, all cards
// become legal so every hand can progress.
func legalCardsWithRules(hand []CardID, trick []Play, rules RuleResolution, facts HandFacts) ([]CardID, error) {
	playRule, err := rules.Effect(MechanicPlayRestriction)
	if err != nil {
		return nil, err
	}
	all := append([]CardID(nil), hand...)
	cards := make([]StandardCard, len(hand))
	for i, id := range hand {
		card, ok := cardByID(id)
		if !ok {
			return nil, fmt.Errorf("unknown hand card %s", id)
		}
		cards[i] = card
	}
	if len(trick) == 0 {
		_, breakSpades := rules.Effect(MechanicBreakSpades)
		_, breakHearts := rules.Effect(MechanicBreakHearts)
		allowed := []CardID{}
		for i, card := range cards {
			if breakSpades == nil && !facts.BrokenSpades && card.Suit == Spades || breakHearts == nil && !facts.BrokenHearts && card.Suit == Hearts {
				continue
			}
			allowed = append(allowed, hand[i])
		}
		if len(allowed) == 0 {
			return all, nil
		}
		return allowed, nil
	}
	lead, ok := cardByID(trick[0].Card)
	if !ok {
		return nil, fmt.Errorf("unknown lead card")
	}
	matching := []CardID{}
	for i, card := range cards {
		if card.Suit == lead.Suit {
			matching = append(matching, hand[i])
		}
	}
	switch playRule.Value {
	case ValueAnySuit:
		return all, nil
	case ValueFollow:
		if len(matching) > 0 {
			return matching, nil
		}
		return all, nil
	case ValueFollowOrTrump:
		if len(matching) == 0 {
			return all, nil
		}
		trump, err := TrumpForTrick(rules, facts.TrumpSuit, trick)
		if err != nil {
			return nil, err
		}
		if trump == "" || lead.Suit == trump {
			return matching, nil
		}
		allowed := []CardID{}
		for i, card := range cards {
			if card.Suit == lead.Suit || card.Suit == trump {
				allowed = append(allowed, hand[i])
			}
		}
		return allowed, nil
	default:
		return nil, fmt.Errorf("unsupported play restriction %s", playRule.Value)
	}
}

// IllegalPlayReason names the effective restriction for local highlighting and
// authoritative action errors. It never uses another player's private hand.
func IllegalPlayReason(hand []CardID, trick []Play, cardID CardID, rules RuleResolution, facts HandFacts) string {
	card, ok := cardByID(cardID)
	if !ok {
		return "Unknown card."
	}
	if len(trick) == 0 {
		if card.Suit == Spades && !facts.BrokenSpades {
			if _, err := rules.Effect(MechanicBreakSpades); err == nil {
				return "Break Spades: lead another suit until spades are broken, unless only spades remain."
			}
		}
		if card.Suit == Hearts && !facts.BrokenHearts {
			if _, err := rules.Effect(MechanicBreakHearts); err == nil {
				return "Break Hearts: lead another suit until hearts are broken, unless only hearts remain."
			}
		}
		return "Lead a legal card."
	}
	lead, ok := cardByID(trick[0].Card)
	if !ok {
		return "Follow the lead suit."
	}
	play, _ := rules.Effect(MechanicPlayRestriction)
	if play.Value == ValueFollowOrTrump {
		trump, _ := TrumpForTrick(rules, facts.TrumpSuit, trick)
		if lead.Suit == trump {
			return "Follow Suit or Trump: follow the trump lead when able."
		}
		return "Follow Suit or Trump: play the lead suit or trump while you hold the lead suit."
	}
	_ = hand
	return "Follow Suit: play the lead suit while you hold it."
}

func validateHandFacts(state MatchState, rules RuleResolution) error {
	if state.TrumpSuit != "" && !validSuit(state.TrumpSuit) {
		return fmt.Errorf("invalid hand trump suit")
	}
	if state.Phase != PhasePlayingTrick && state.Phase != PhaseAuction {
		return nil // A rule revealed after the hand applies from the next deal.
	}
	plan, err := HandPlanForRules(state.Config, len(state.Seats), rules)
	if err != nil || state.DealSize != plan.DealSize || state.TricksPerHand != plan.Tricks || state.TrickNumber > state.TricksPerHand {
		return fmt.Errorf("invalid frozen hand size or trick count")
	}
	if state.HandNumber < 1 || state.TrickNumber < 1 || state.TrickNumber == 1 && len(state.CurrentTrick) < 2 && (state.BrokenSpades || state.BrokenHearts) {
		return fmt.Errorf("invalid current-hand facts")
	}
	effect, err := rules.Effect(MechanicTrump)
	if err != nil {
		return err
	}
	switch effect.Value {
	case ValueNoTrump:
		if state.TrumpSuit != "" {
			return fmt.Errorf("no-trump hand has a trump suit")
		}
	case ValueSpadesTrump:
		if state.TrumpSuit != Spades {
			return fmt.Errorf("fixed-spades hand has wrong trump")
		}
	case ValueTurnUpTrump:
		card, ok := cardByID(state.TurnedTrumpCard)
		if !ok || state.DealOwner[card.ID] != state.Dealer || state.TrumpSuit != card.Suit {
			return fmt.Errorf("turn-up card does not match dealer or trump")
		}
	case ValueFirstLeadTrump:
		led := state.TrickNumber > 1 || len(state.CurrentTrick) > 0
		if led && state.TrumpSuit == "" || !led && state.TrumpSuit != "" {
			return fmt.Errorf("pitched trump does not match opening state")
		}
		if state.TrickNumber == 1 && len(state.CurrentTrick) > 0 {
			lead, ok := cardByID(state.CurrentTrick[0].Card)
			if !ok || state.TrumpSuit != lead.Suit {
				return fmt.Errorf("pitched trump disagrees with first lead")
			}
		}
	default:
		return fmt.Errorf("unsupported trump selection")
	}
	return nil
}
