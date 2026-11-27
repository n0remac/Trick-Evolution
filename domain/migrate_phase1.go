package domain

import "fmt"

// MigratePhase1Snapshot upgrades a version-eight live snapshot without
// rewriting its personal scores, team ledger, award history, or dealt cards.
// Old rule-draw orders gain the new cards at the end, in stable catalog order.
func MigratePhase1Snapshot(old MatchState) (MatchState, error) {
	if old.CatalogVersion != 8 {
		return old, fmt.Errorf("expected a phase-one catalog version 8 snapshot")
	}
	next := cloneState(old)
	next.CatalogVersion = 9
	next.Field = append(next.Field, RuleInstance{InstanceID: fmt.Sprintf("rule:migration:bid_metric:%d", len(next.Field)), DefinitionID: RuleDefaultBidPoints, Sequence: len(next.Field), Starting: true})
	if next.Config.Mode == ModeEndless {
		for _, id := range []RuleID{RuleAuction, RuleBidPoints, RuleBidTricks, RuleSetBidder} {
			next.RuleDraw = append(next.RuleDraw, id)
		}
		if len(next.Shuffles) < 2 || next.Shuffles[1].Kind != RuleDeckKind {
			return old, fmt.Errorf("phase-one rule shuffle is missing")
		}
		for _, id := range []RuleID{RuleAuction, RuleBidPoints, RuleBidTricks, RuleSetBidder} {
			next.Shuffles[1].Order = append(next.Shuffles[1].Order, string(id))
		}
	}
	next, err := MigratePhase2Snapshot(next)
	if err != nil {
		return old, fmt.Errorf("phase-one snapshot cannot be migrated: %w", err)
	}
	return next, nil
}

// MigratePhase2Snapshot retains the v9 deal, scores and scorecard. New target
// cards enter only Endless draws; finite v9 games keep their selected deck.
func MigratePhase2Snapshot(old MatchState) (MatchState, error) {
	if old.CatalogVersion != 9 {
		return old, fmt.Errorf("expected a phase-two catalog version 9 snapshot")
	}
	next := cloneState(old)
	next.CatalogVersion = CatalogVersion
	if next.GameNumber == 0 {
		next.GameNumber = 1
	}
	if next.Config.Mode == ModeEndless {
		for _, id := range []RuleID{RuleFirstTo5, RuleFirstTo21, RuleWinOnBid} {
			next.RuleDraw = append(next.RuleDraw, id)
		}
		if len(next.Shuffles) < 2 || next.Shuffles[1].Kind != RuleDeckKind {
			return old, fmt.Errorf("phase-two rule shuffle is missing")
		}
		for _, id := range []RuleID{RuleFirstTo5, RuleFirstTo21, RuleWinOnBid} {
			next.Shuffles[1].Order = append(next.Shuffles[1].Order, string(id))
		}
	}
	if _, err := ProjectPublic(next, DefaultCatalog()); err != nil {
		return old, fmt.Errorf("phase-two snapshot cannot be migrated: %w", err)
	}
	return next, nil
}
