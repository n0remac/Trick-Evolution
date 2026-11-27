package domain

import "fmt"

// HandPlan is frozen when a hand is dealt. A later reveal changes only the
// next plan, never the number of tricks in the hand that just finished.
type HandPlan struct {
	DealSize int
	Tricks   int
	KeepOne  bool
}

func HandPlanForRules(config Config, seatCount int, rules RuleResolution) (HandPlan, error) {
	if err := config.Validate(seatCount); err != nil {
		return HandPlan{}, err
	}
	plan := HandPlan{DealSize: config.HandSize}
	_, full := rules.Effect(MechanicFullDeal)
	_, six := rules.Effect(MechanicSixCardHand)
	_, expanded := rules.Effect(MechanicExpandedHand)
	_, keep := rules.Effect(MechanicKeepOne)
	if full == nil && (six == nil || expanded == nil || keep == nil) || six == nil && (expanded == nil || keep == nil) {
		return HandPlan{}, fmt.Errorf("Full Deal and Six-Card Hand cannot combine with other hand modifiers")
	}
	if full == nil {
		plan.DealSize = 52 / seatCount
	}
	if six == nil {
		plan.DealSize = 6
	}
	if expanded == nil {
		plan.DealSize += 2
	}
	if plan.DealSize*seatCount > 52 {
		return HandPlan{}, fmt.Errorf("effective hand size %d cannot be dealt to %d players from one standard deck", plan.DealSize, seatCount)
	}
	if keep == nil {
		plan.KeepOne = true
		if plan.DealSize < 2 {
			return HandPlan{}, fmt.Errorf("Keep One Card requires an effective hand size of at least 2")
		}
	}
	plan.Tricks = plan.DealSize
	if plan.KeepOne {
		plan.Tricks--
	}
	if ownership, err := rules.Effect(MechanicScoreOwnership); err == nil && ownership.Value == ValuePartnership && seatCount != 4 {
		return HandPlan{}, fmt.Errorf("partnerships require exactly four seats")
	}
	if _, err := rules.Effect(MechanicAuction); err == nil {
		probe := MatchState{Seats: make([]Seat, seatCount), TricksPerHand: plan.Tricks}
		if _, _, _, err := AuctionBounds(probe, rules); err != nil {
			return HandPlan{}, err
		}
	}
	return plan, nil
}

// ValidateInitialSetup uses the actual room seat count, including bots.
func ValidateInitialSetup(config Config, seatCount int, catalog Catalog) error {
	if err := config.Validate(seatCount); err != nil {
		return err
	}
	if err := ValidateInitialRules(config, catalog); err != nil {
		return err
	}
	rules, err := ResolveRules(StartingFieldWithConfig(catalog, config), catalog, config)
	if err != nil {
		return err
	}
	if err := rules.ValidateMatchEffects(config); err != nil {
		return err
	}
	_, err = HandPlanForRules(config, seatCount, rules)
	return err
}
