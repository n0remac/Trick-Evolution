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
	if _, err := rules.Effect(MechanicExpandedHand); err == nil {
		plan.DealSize += 2
	}
	if plan.DealSize*seatCount > 52 {
		return HandPlan{}, fmt.Errorf("effective hand size %d cannot be dealt to %d players from one standard deck", plan.DealSize, seatCount)
	}
	if _, err := rules.Effect(MechanicKeepOne); err == nil {
		plan.KeepOne = true
		if plan.DealSize < 2 {
			return HandPlan{}, fmt.Errorf("Keep One Card requires an effective hand size of at least 2")
		}
	}
	plan.Tricks = plan.DealSize
	if plan.KeepOne {
		plan.Tricks--
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
