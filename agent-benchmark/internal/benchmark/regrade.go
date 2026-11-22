package benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// AttachSavedCost links an offline fake-patch regrade to the matching saved
// Codex attempt. It never changes the original result or performs a model call.
func AttachSavedCost(regrade *Result, savedPath string, prices *TokenPrices) error {
	if regrade == nil || regrade.Adapter != "fake" || regrade.CandidateSHA256 == "" {
		return errors.New("saved cost requires an offline patch regrade with a captured candidate")
	}
	if prices != nil {
		if err := prices.validate(); err != nil {
			return err
		}
	}
	data, err := os.ReadFile(savedPath)
	if err != nil {
		return err
	}
	var saved Result
	if err := json.Unmarshal(data, &saved); err != nil {
		return fmt.Errorf("saved result: %w", err)
	}
	if saved.Adapter != "codex" || saved.Agent == nil || saved.CandidateSHA256 != regrade.CandidateSHA256 || saved.Task != regrade.Task || saved.Baseline != regrade.Baseline {
		return errors.New("saved Codex result does not match the regraded candidate, task, and baseline")
	}
	regrade.Cost = codexCost(saved.Agent.Usage, prices)
	regrade.Cost.Source = "saved_codex_turn_completed_usage"
	regrade.RegradedFrom = savedPath
	return nil
}
