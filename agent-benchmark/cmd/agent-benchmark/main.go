package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"

	"agent-benchmark/internal/benchmark"
)

func main() {
	os.Exit(run())
}

func run() int {
	pack := flag.String("pack", "", "path to a task pack JSON file")
	adapter := flag.String("adapter", "fake", "agent adapter: fake or codex")
	patch := flag.String("patch", "", "fixture patch supplied by the fake agent")
	savedResult := flag.String("saved-result", "", "saved Codex result.json to link to an offline fake-patch regrade")
	out := flag.String("out", "", "directory for result.json and result.md")
	codexBin := flag.String("codex-bin", "codex", "Codex CLI executable (also accepts a local offline stub)")
	authFile := flag.String("codex-auth-file", "", "Codex auth.json; defaults to ~/.codex/auth.json")
	goModCache := flag.String("go-mod-cache", "", "prepared Go module cache to mount read-only for Codex tools")
	model := flag.String("model", "", "Codex model name, required with -adapter codex")
	effort := flag.String("reasoning-effort", "medium", "Codex reasoning effort")
	var inputPrice, cachedPrice, outputPrice *float64
	priceFlag := func(target **float64) func(string) error {
		return func(value string) error {
			rate, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return err
			}
			*target = &rate
			return nil
		}
	}
	flag.Func("price-input-per-million", "explicit USD per million uncached input tokens", priceFlag(&inputPrice))
	flag.Func("price-cached-input-per-million", "explicit USD per million cached input tokens", priceFlag(&cachedPrice))
	flag.Func("price-output-per-million", "explicit USD per million output tokens", priceFlag(&outputPrice))
	flag.Parse()
	pricesProvided := inputPrice != nil || cachedPrice != nil || outputPrice != nil
	pricesComplete := inputPrice != nil && cachedPrice != nil && outputPrice != nil
	if flag.NArg() != 0 || *pack == "" || *out == "" || (*adapter == "fake" && *patch == "") || (*adapter == "codex" && (*patch != "" || *model == "" || *savedResult != "")) || (*adapter != "fake" && *adapter != "codex") || (pricesProvided && (!pricesComplete || (*adapter == "fake" && *savedResult == ""))) {
		fmt.Fprintln(os.Stderr, "usage: agent-benchmark -pack TASK.json -out RESULTS_DIR [-adapter fake -patch FIXTURE.patch [-saved-result ORIGINAL_RESULT.json] | -adapter codex -model MODEL [-reasoning-effort medium] [-codex-bin PATH] [-codex-auth-file PATH] [-go-mod-cache DIR]] [-price-input-per-million USD -price-cached-input-per-million USD -price-output-per-million USD]")
		return 2
	}
	var prices *benchmark.TokenPrices
	if pricesComplete {
		prices = &benchmark.TokenPrices{InputPerMillion: *inputPrice, CachedInputPerMillion: *cachedPrice, OutputPerMillion: *outputPrice}
	}

	result := benchmark.RunWithOptions(context.Background(), *pack, benchmark.RunOptions{Adapter: *adapter, PatchPath: *patch, OutDir: *out, CodexBinary: *codexBin, Model: *model, ReasoningEffort: *effort, AuthFile: *authFile, GoModCache: *goModCache, Prices: prices})
	if *savedResult != "" {
		if err := benchmark.AttachSavedCost(&result, *savedResult, prices); err != nil {
			fmt.Fprintf(os.Stderr, "link saved result: %v\n", err)
			return 2
		}
	}
	if err := benchmark.WriteReports(*out, result); err != nil {
		fmt.Fprintf(os.Stderr, "write reports: %v\n", err)
		return 2
	}
	fmt.Printf("%s (stage: %s); reports: %s/result.json, %s/result.md\n", result.Status, result.Stage, *out, *out)
	if !result.Passed {
		return 1
	}
	return 0
}
