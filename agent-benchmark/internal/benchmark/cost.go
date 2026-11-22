package benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
)

// TokenPrices are caller-supplied USD rates per million tokens. The runner has
// no built-in model price table; a model name alone never implies a rate.
type TokenPrices struct {
	InputPerMillion       float64 `json:"input_per_million_usd"`
	CachedInputPerMillion float64 `json:"cached_input_per_million_usd"`
	OutputPerMillion      float64 `json:"output_per_million_usd"`
}

func (p TokenPrices) validate() error {
	for _, rate := range []float64{p.InputPerMillion, p.CachedInputPerMillion, p.OutputPerMillion} {
		if math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 {
			return errors.New("token prices must be finite, nonnegative USD rates per million tokens")
		}
	}
	return nil
}

type TokenCounts struct {
	InputTotal      int64  `json:"input_total"`
	InputCached     int64  `json:"input_cached"`
	InputUncached   int64  `json:"input_uncached"`
	OutputTotal     int64  `json:"output_total"`
	OutputReasoning *int64 `json:"output_reasoning_included"`
}

// Actual billing requires an authoritative provider bill. Usage events only
// support an estimate, even if explicit prices are available.
type CostReport struct {
	Source             string       `json:"source"`
	ActualBilledAPIUSD *float64     `json:"actual_billed_api_usd"`
	ActualStatus       string       `json:"actual_status"`
	Tokens             *TokenCounts `json:"tokens"`
	Prices             *TokenPrices `json:"prices_per_million_usd"`
	EstimatedAPIUSD    *float64     `json:"estimated_api_usd"`
	EstimateStatus     string       `json:"estimate_status"`
}

func parseTokenCounts(raw json.RawMessage) (*TokenCounts, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errors.New("usage unavailable")
	}
	var usage struct {
		Input     *int64 `json:"input_tokens"`
		Cached    *int64 `json:"cached_input_tokens"`
		Output    *int64 `json:"output_tokens"`
		Reasoning *int64 `json:"reasoning_output_tokens"`
		Details   struct {
			Reasoning *int64 `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
	}
	if err := json.Unmarshal(raw, &usage); err != nil {
		return nil, fmt.Errorf("invalid usage: %w", err)
	}
	if usage.Input == nil || usage.Cached == nil || usage.Output == nil {
		return nil, errors.New("usage omitted input, cached input, or output tokens")
	}
	cached := *usage.Cached
	var reasoning *int64
	if usage.Reasoning != nil {
		reasoning = usage.Reasoning
	} else if usage.Details.Reasoning != nil {
		reasoning = usage.Details.Reasoning
	}
	if *usage.Input < 0 || cached < 0 || cached > *usage.Input || *usage.Output < 0 || (reasoning != nil && (*reasoning < 0 || *reasoning > *usage.Output)) {
		return nil, errors.New("usage contains inconsistent token counts")
	}
	return &TokenCounts{InputTotal: *usage.Input, InputCached: cached, InputUncached: *usage.Input - cached, OutputTotal: *usage.Output, OutputReasoning: reasoning}, nil
}

func codexCost(raw json.RawMessage, prices *TokenPrices) *CostReport {
	cost := &CostReport{Source: "codex_turn_completed_usage", ActualStatus: "unknown_without_authoritative_billing", Prices: prices}
	tokens, err := parseTokenCounts(raw)
	if err != nil {
		if len(raw) == 0 {
			cost.EstimateStatus = "usage_unavailable"
		} else {
			cost.EstimateStatus = "usage_invalid"
		}
		return cost
	}
	cost.Tokens = tokens
	if prices == nil {
		cost.EstimateStatus = "prices_not_configured"
		return cost
	}
	if err := prices.validate(); err != nil {
		cost.EstimateStatus = "prices_invalid"
		return cost
	}
	estimate := (float64(tokens.InputUncached)*prices.InputPerMillion + float64(tokens.InputCached)*prices.CachedInputPerMillion + float64(tokens.OutputTotal)*prices.OutputPerMillion) / 1_000_000
	cost.EstimatedAPIUSD = &estimate
	cost.EstimateStatus = "pricing_based_estimate"
	return cost
}
