package benchmark

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexCostUsesExclusiveInputBucketsAndInclusiveOutput(t *testing.T) {
	prices := &TokenPrices{InputPerMillion: 2, CachedInputPerMillion: 0.5, OutputPerMillion: 8}
	for _, raw := range []string{
		`{"input_tokens":1000,"cached_input_tokens":800,"output_tokens":100,"reasoning_output_tokens":40}`,
		`{"input_tokens":1000,"cached_input_tokens":800,"output_tokens":100,"output_tokens_details":{"reasoning_tokens":40}}`,
	} {
		cost := codexCost(json.RawMessage(raw), prices)
		if cost.ActualBilledAPIUSD != nil || cost.Tokens == nil || cost.Tokens.InputTotal != 1000 || cost.Tokens.InputCached != 800 || cost.Tokens.InputUncached != 200 || cost.Tokens.OutputTotal != 100 || cost.Tokens.OutputReasoning == nil || *cost.Tokens.OutputReasoning != 40 || cost.EstimatedAPIUSD == nil || math.Abs(*cost.EstimatedAPIUSD-0.0016) > 1e-12 {
			t.Fatalf("cost: %+v", cost)
		}
	}
}

func TestCodexCostDoesNotInventPricesOrUsage(t *testing.T) {
	usage := json.RawMessage(`{"input_tokens":100,"cached_input_tokens":25,"output_tokens":10}`)
	withoutReasoning := codexCost(usage, &TokenPrices{InputPerMillion: 1})
	if withoutReasoning.Tokens == nil || withoutReasoning.Tokens.OutputReasoning != nil || withoutReasoning.EstimatedAPIUSD == nil || math.Abs(*withoutReasoning.EstimatedAPIUSD-0.000075) > 1e-12 {
		t.Fatalf("reasoning or estimate invented: %+v", withoutReasoning)
	}
	for _, tc := range []struct {
		raw    json.RawMessage
		prices *TokenPrices
		status string
		tokens bool
	}{
		{usage, nil, "prices_not_configured", true},
		{nil, &TokenPrices{}, "usage_unavailable", false},
		{json.RawMessage(`{"input_tokens":10,"output_tokens":1}`), &TokenPrices{}, "usage_invalid", false},
		{json.RawMessage(`{"input_tokens":10,"cached_input_tokens":11,"output_tokens":1}`), &TokenPrices{}, "usage_invalid", false},
		{json.RawMessage(`{"input_tokens":10,"output_tokens":1,"reasoning_output_tokens":2}`), &TokenPrices{}, "usage_invalid", false},
	} {
		cost := codexCost(tc.raw, tc.prices)
		if cost.ActualBilledAPIUSD != nil || cost.EstimatedAPIUSD != nil || cost.EstimateStatus != tc.status || (cost.Tokens != nil) != tc.tokens {
			t.Fatalf("cost: %+v", cost)
		}
	}
	for _, p := range []TokenPrices{{InputPerMillion: -1}, {OutputPerMillion: math.NaN()}, {CachedInputPerMillion: math.Inf(1)}} {
		if p.validate() == nil {
			t.Fatalf("accepted invalid price: %+v", p)
		}
	}
}

func TestCostReportJSONAndMarkdownShowUnknownActual(t *testing.T) {
	cost := codexCost(json.RawMessage(`{"input_tokens":1000,"cached_input_tokens":800,"output_tokens":100,"reasoning_output_tokens":40}`), &TokenPrices{InputPerMillion: 2, CachedInputPerMillion: 0.5, OutputPerMillion: 8})
	out := t.TempDir()
	if err := WriteReports(out, Result{Task: "cost", Adapter: "codex", Status: "passed", Cost: cost}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(out, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"actual_billed_api_usd": null`) || !strings.Contains(string(b), `"input_uncached": 200`) || !strings.Contains(string(b), `"estimated_api_usd": 0.0016`) {
		t.Fatalf("JSON cost: %s", b)
	}
	md, err := os.ReadFile(filepath.Join(out, "result.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "Actual billed API USD: unknown") || !strings.Contains(string(md), "Pricing-based API estimate: $0.001600") || !strings.Contains(string(md), "reasoning included in output") {
		t.Fatalf("markdown cost: %s", md)
	}
}

func TestAttachSavedCostChecksIdentityAndKeepsOriginal(t *testing.T) {
	saved := Result{Task: "invite", Baseline: strings.Repeat("a", 40), Adapter: "codex", CandidateSHA256: "patch-hash", Agent: &AgentRun{Usage: json.RawMessage(`{"input_tokens":30,"cached_input_tokens":20,"output_tokens":5}`)}}
	data, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "original.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	regrade := Result{Task: saved.Task, Baseline: saved.Baseline, Adapter: "fake", CandidateSHA256: saved.CandidateSHA256}
	if err := AttachSavedCost(&regrade, path, nil); err != nil {
		t.Fatal(err)
	}
	if regrade.Cost == nil || regrade.Cost.Source != "saved_codex_turn_completed_usage" || regrade.Cost.ActualBilledAPIUSD != nil || regrade.Cost.Tokens == nil || regrade.Cost.Tokens.InputUncached != 10 || regrade.RegradedFrom != path {
		t.Fatalf("regrade: %+v", regrade)
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, current) {
		t.Fatal("original result changed")
	}
	regrade.CandidateSHA256 = "different"
	if err := AttachSavedCost(&regrade, path, nil); err == nil {
		t.Fatal("accepted mismatched saved patch")
	}
}
