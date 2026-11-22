// Package benchmark runs a fixture patch against an evaluator-owned task pack.
package benchmark

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const maxOutput = 16 * 1024

// Pack describes one project-neutral benchmark task. Repository is relative to
// the pack file unless absolute. Acceptance runs from the grading checkout.
type Pack struct {
	Name                     string   `json:"name"`
	Repository               string   `json:"repository"`
	Baseline                 string   `json:"baseline"`
	Prompt                   string   `json:"prompt"`
	BudgetSeconds            int      `json:"budget_seconds"`
	AcceptanceCommand        []string `json:"acceptance_command"`
	AcceptanceTimeoutSeconds int      `json:"acceptance_timeout_seconds,omitempty"`
}

type Acceptance struct {
	Passed    bool   `json:"passed"`
	ExitCode  *int   `json:"exit_code,omitempty"`
	TimedOut  bool   `json:"timed_out"`
	ElapsedMS int64  `json:"elapsed_ms"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
}

type Failure struct {
	Stage   string `json:"stage"`
	Details string `json:"details"`
}

type Result struct {
	Task            string      `json:"task,omitempty"`
	Baseline        string      `json:"baseline,omitempty"`
	Adapter         string      `json:"adapter,omitempty"`
	Status          string      `json:"status"`
	Stage           string      `json:"stage"`
	Passed          bool        `json:"passed"`
	ElapsedMS       int64       `json:"elapsed_ms"`
	AgentElapsedMS  int64       `json:"agent_elapsed_ms"`
	Agent           *AgentRun   `json:"agent,omitempty"`
	Cost            *CostReport `json:"cost,omitempty"`
	RegradedFrom    string      `json:"regraded_from,omitempty"`
	Artifacts       *Artifacts  `json:"artifacts,omitempty"`
	CandidateSHA256 string      `json:"candidate_sha256,omitempty"`
	Acceptance      *Acceptance `json:"acceptance,omitempty"`
	Failure         *Failure    `json:"failure,omitempty"`
}

type AgentRun struct {
	Model           string            `json:"model,omitempty"`
	ReasoningEffort string            `json:"reasoning_effort,omitempty"`
	CLI             string            `json:"cli,omitempty"`
	Isolation       string            `json:"isolation,omitempty"`
	Settings        map[string]string `json:"settings,omitempty"`
	Sandbox         string            `json:"sandbox,omitempty"`
	Approval        string            `json:"approval,omitempty"`
	ToolNetwork     string            `json:"tool_network,omitempty"`
	WebSearch       string            `json:"web_search,omitempty"`
	ExitCode        *int              `json:"exit_code,omitempty"`
	TimedOut        bool              `json:"timed_out"`
	Usage           json.RawMessage   `json:"usage,omitempty"`
	StderrTail      string            `json:"stderr_tail,omitempty"`
	EventTail       string            `json:"event_tail,omitempty"`
}

type Artifacts struct {
	Patch       string `json:"patch,omitempty"`
	Events      string `json:"events,omitempty"`
	Stderr      string `json:"stderr,omitempty"`
	LastMessage string `json:"last_message,omitempty"`
}

type RunOptions struct {
	Adapter         string
	PatchPath       string
	OutDir          string
	CodexBinary     string
	Model           string
	ReasoningEffort string
	AuthFile        string
	GoModCache      string
	Prices          *TokenPrices
}

// Agent is the local attempt boundary. An implementation receives only the
// prompt and its isolated checkout, never the evaluator's acceptance command.
type Agent interface {
	Run(context.Context, string, string) error
}

// FixturePatchAgent is the milestone's deterministic stand-in for a coding agent.
type FixturePatchAgent struct {
	Patch []byte
}

func (a FixturePatchAgent) Run(ctx context.Context, checkout, prompt string) error {
	patchFile, err := os.CreateTemp("", "agent-benchmark-fixture-*.patch")
	if err != nil {
		return err
	}
	defer os.Remove(patchFile.Name())
	if _, err := patchFile.Write(a.Patch); err != nil {
		patchFile.Close()
		return err
	}
	if err := patchFile.Close(); err != nil {
		return err
	}
	_, err = git(ctx, checkout, "apply", "--", patchFile.Name())
	return err
}

func validCommit(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

func readPack(path string) (Pack, string, error) {
	var p Pack
	abs, err := filepath.Abs(path)
	if err != nil {
		return p, "", err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return p, "", err
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return p, "", err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return p, "", errors.New("task pack contains trailing data")
	}
	if p.Name == "" || p.Repository == "" || !validCommit(p.Baseline) || strings.TrimSpace(p.Prompt) == "" || p.BudgetSeconds <= 0 || len(p.AcceptanceCommand) == 0 || p.AcceptanceCommand[0] == "" || p.AcceptanceTimeoutSeconds < 0 {
		return p, "", errors.New("task pack requires name, repository, full baseline commit, prompt, positive budget_seconds, and acceptance_command")
	}
	for _, arg := range p.AcceptanceCommand {
		if arg == "" {
			return p, "", errors.New("acceptance_command cannot contain empty arguments")
		}
	}
	packDir := filepath.Dir(abs)
	if !filepath.IsAbs(p.Repository) {
		p.Repository = filepath.Join(packDir, p.Repository)
	}
	return p, packDir, nil
}

func truncate(b []byte) string {
	if len(b) <= maxOutput {
		return string(b)
	}
	return string(b[:maxOutput]) + "\n[output truncated]"
}

func command(ctx context.Context, dir string, argv []string, env []string) (string, string, error) {
	c := exec.CommandContext(ctx, argv[0], argv[1:]...)
	c.Dir = dir
	if env != nil {
		c.Env = append(os.Environ(), env...)
	}
	var out, errOut bytes.Buffer
	c.Stdout, c.Stderr = &out, &errOut
	err := c.Run()
	return truncate(out.Bytes()), truncate(errOut.Bytes()), err
}

func git(ctx context.Context, dir string, args ...string) (string, error) {
	argv := append([]string{"git"}, args...)
	out, stderr, err := command(ctx, dir, argv, nil)
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr))
	}
	return out, nil
}

func gitDiff(ctx context.Context, dir, baseline string) ([]byte, error) {
	c := exec.CommandContext(ctx, "git", "diff", "--cached", "--binary", "--full-index", "--no-ext-diff", baseline)
	c.Dir = dir
	var stderr bytes.Buffer
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("git diff: %w: %s", err, truncate(stderr.Bytes()))
	}
	return out, nil
}

// checkoutShallow carries only the pinned commit and tree into an agent checkout.
// A normal local clone would expose upstream history through .git/objects.
func checkoutShallow(ctx context.Context, repo, baseline, dest string) error {
	if _, err := git(ctx, "", "init", "--quiet", "--", dest); err != nil {
		return err
	}
	uri := "file://" + repo
	if _, err := git(ctx, dest, "fetch", "--quiet", "--depth=1", "--no-tags", "--no-write-fetch-head", "--", uri, baseline); err != nil {
		return err
	}
	_, err := git(ctx, dest, "checkout", "--quiet", "--detach", baseline)
	return err
}

func checkout(ctx context.Context, repo, baseline, dest string) error {
	if _, err := git(ctx, "", "clone", "--quiet", "--no-hardlinks", "--no-checkout", "--", repo, dest); err != nil {
		return err
	}
	_, err := git(ctx, dest, "checkout", "--quiet", "--detach", baseline)
	return err
}

// Run retains the original fixture-patch API.
func Run(ctx context.Context, packPath, patchPath string) Result {
	return RunWithOptions(ctx, packPath, RunOptions{Adapter: "fake", PatchPath: patchPath})
}

// RunWithOptions provisions a fresh attempt and grades its captured diff in a
// separate checkout. The Codex adapter never receives evaluator paths.
func RunWithOptions(ctx context.Context, packPath string, opts RunOptions) (r Result) {
	started := time.Now()
	r.Status = "failed"
	r.Stage = "load_pack"
	r.Adapter = opts.Adapter
	if r.Adapter == "" {
		r.Adapter = "fake"
	}
	if r.Adapter == "codex" {
		r.Cost = codexCost(nil, opts.Prices)
	}
	defer func() { r.ElapsedMS = time.Since(started).Milliseconds() }()
	fail := func(stage string, err error) Result {
		r.Stage = stage
		r.Failure = &Failure{Stage: stage, Details: err.Error()}
		return r
	}

	p, packDir, err := readPack(packPath)
	if err != nil {
		return fail("load_pack", err)
	}
	r.Task, r.Baseline = p.Name, p.Baseline
	var patch []byte
	switch r.Adapter {
	case "fake":
		patch, err = os.ReadFile(opts.PatchPath)
		if err != nil {
			return fail("read_patch", err)
		}
		if len(patch) == 0 {
			return fail("read_patch", errors.New("fixture patch is empty"))
		}
	case "codex":
		if opts.Prices != nil {
			if err := opts.Prices.validate(); err != nil {
				return fail("configure_agent", err)
			}
		}
		if opts.OutDir == "" {
			return fail("configure_agent", errors.New("Codex requires an output directory for durable artifacts"))
		}
		if err := prepareCodex(&r, &opts); err != nil {
			return fail("configure_agent", err)
		}
	default:
		return fail("configure_agent", fmt.Errorf("unknown adapter %q", r.Adapter))
	}
	verified, err := git(ctx, p.Repository, "rev-parse", "--verify", p.Baseline+"^{commit}")
	if err != nil || !strings.EqualFold(strings.TrimSpace(verified), p.Baseline) {
		if err == nil {
			err = errors.New("baseline is not the requested commit")
		}
		return fail("verify_baseline", err)
	}
	work, err := os.MkdirTemp("", "agent-benchmark-")
	if err != nil {
		return fail("create_workspace", err)
	}
	defer os.RemoveAll(work)
	attempt := filepath.Join(work, "attempt")
	grade := filepath.Join(work, "grade")
	if r.Adapter == "codex" {
		err = checkoutShallow(ctx, p.Repository, p.Baseline, attempt)
	} else {
		err = checkout(ctx, p.Repository, p.Baseline, attempt)
	}
	if err != nil {
		return fail("checkout_attempt", err)
	}

	agentStarted := time.Now()
	agentCtx, cancel := context.WithTimeout(ctx, time.Duration(p.BudgetSeconds)*time.Second)
	defer cancel()
	var agentErr error
	if r.Adapter == "fake" {
		agentErr = (FixturePatchAgent{Patch: patch}).Run(agentCtx, attempt, p.Prompt)
	} else {
		agentErr = runCodex(agentCtx, attempt, p.Prompt, opts, &r)
	}
	r.AgentElapsedMS = time.Since(agentStarted).Milliseconds()
	budgetErr := agentCtx.Err()
	cancel()
	// Capture after a timeout or backend error as well: a partial patch is useful
	// evidence. Do not use the expired agent context for this step.
	captureCtx, captureCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer captureCancel()
	if _, err := git(captureCtx, attempt, "add", "-A"); err != nil {
		if agentErr != nil {
			if budgetErr != nil {
				return fail("agent_budget", agentErr)
			}
			return fail("agent_run", agentErr)
		}
		return fail("capture_patch", err)
	}
	candidate, err := gitDiff(captureCtx, attempt, p.Baseline)
	if err != nil {
		return fail("capture_patch", err)
	}
	if len(candidate) > 0 {
		hash := sha256.Sum256(candidate)
		r.CandidateSHA256 = hex.EncodeToString(hash[:])
		if opts.OutDir != "" {
			artifact := filepath.Join(opts.OutDir, "candidate.patch")
			if err := os.MkdirAll(opts.OutDir, 0700); err != nil {
				return fail("capture_patch", err)
			}
			if err := os.WriteFile(artifact, candidate, 0600); err != nil {
				return fail("capture_patch", err)
			}
			if r.Artifacts == nil {
				r.Artifacts = &Artifacts{}
			}
			r.Artifacts.Patch = artifact
		}
	}
	if agentErr != nil {
		if budgetErr != nil {
			return fail("agent_budget", fmt.Errorf("agent deadline after %d seconds: %w", p.BudgetSeconds, agentErr))
		}
		return fail("agent_run", agentErr)
	}
	if budgetErr != nil {
		return fail("agent_budget", budgetErr)
	}
	if len(candidate) == 0 {
		return fail("capture_patch", errors.New("agent produced no changes"))
	}
	if err := checkout(ctx, p.Repository, p.Baseline, grade); err != nil {
		return fail("checkout_grade", err)
	}
	candidateFile := filepath.Join(work, "candidate.patch")
	if err := os.WriteFile(candidateFile, candidate, 0600); err != nil {
		return fail("grade_apply", err)
	}
	if _, err := git(ctx, grade, "apply", "--", candidateFile); err != nil {
		return fail("grade_apply", err)
	}

	r.Stage = "acceptance"
	timeout := p.AcceptanceTimeoutSeconds
	if timeout == 0 {
		timeout = 60
	}
	acceptCtx, acceptCancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer acceptCancel()
	args := make([]string, len(p.AcceptanceCommand))
	for i, arg := range p.AcceptanceCommand {
		arg = strings.ReplaceAll(arg, "{checkout}", grade)
		args[i] = strings.ReplaceAll(arg, "{pack_dir}", packDir)
	}
	acceptStarted := time.Now()
	out, stderr, err := command(acceptCtx, grade, args, []string{"BENCH_CHECKOUT=" + grade, "BENCH_PACK_DIR=" + packDir})
	a := &Acceptance{Passed: err == nil, TimedOut: acceptCtx.Err() == context.DeadlineExceeded, ElapsedMS: time.Since(acceptStarted).Milliseconds(), Stdout: out, Stderr: stderr}
	if err == nil {
		code := 0
		a.ExitCode = &code
	} else {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code := exit.ExitCode()
			a.ExitCode = &code
		}
	}
	r.Acceptance = a
	if err != nil {
		details := strings.TrimSpace(stderr)
		if details == "" {
			details = strings.TrimSpace(out)
		}
		if a.TimedOut {
			return fail("acceptance", fmt.Errorf("acceptance command timed out after %d seconds: %s", timeout, details))
		}
		return fail("acceptance", fmt.Errorf("acceptance command failed: %w: %s", err, details))
	}
	r.Status, r.Stage, r.Passed = "passed", "complete", true
	return r
}

func WriteReports(outDir string, r Result) error {
	if err := os.MkdirAll(outDir, 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "result.json"), append(b, '\n'), 0600); err != nil {
		return err
	}
	var md strings.Builder
	fmt.Fprintf(&md, "# Benchmark result: %s\n\n", r.Task)
	fmt.Fprintf(&md, "- Status: **%s**\n- Stage: `%s`\n- Baseline: `%s`\n- Elapsed: %d ms\n- Agent elapsed: %d ms\n", r.Status, r.Stage, r.Baseline, r.ElapsedMS, r.AgentElapsedMS)
	fmt.Fprintf(&md, "- Adapter: `%s`\n", r.Adapter)
	if r.Agent != nil {
		fmt.Fprintf(&md, "- Model: `%s`\n- Reasoning effort: `%s`\n- Isolation: `%s`\n- Sandbox: `%s`\n- Approval: `%s`\n- Tool network: `%s`\n- Web search: `%s`\n- Agent timed out: %t\n", r.Agent.Model, r.Agent.ReasoningEffort, r.Agent.Isolation, r.Agent.Sandbox, r.Agent.Approval, r.Agent.ToolNetwork, r.Agent.WebSearch, r.Agent.TimedOut)
		if len(r.Agent.Settings) > 0 {
			settings, _ := json.Marshal(r.Agent.Settings)
			fmt.Fprintf(&md, "- Settings: `%s`\n", settings)
		}
		if r.Agent.ExitCode != nil {
			fmt.Fprintf(&md, "- Agent exit code: %d\n", *r.Agent.ExitCode)
		}
		if len(r.Agent.Usage) > 0 {
			fmt.Fprintf(&md, "- Agent usage: `%s`\n", r.Agent.Usage)
		} else {
			fmt.Fprint(&md, "- Agent usage: unavailable\n")
		}
	}
	if r.Cost != nil {
		fmt.Fprintf(&md, "- Cost usage source: `%s`\n", r.Cost.Source)
		fmt.Fprintf(&md, "- Actual billed API USD: unknown (no authoritative billing data)\n")
		if r.Cost.Tokens != nil {
			t := r.Cost.Tokens
			fmt.Fprintf(&md, "- Tokens: %d total input (%d uncached, %d cached); %d output", t.InputTotal, t.InputUncached, t.InputCached, t.OutputTotal)
			if t.OutputReasoning != nil {
				fmt.Fprintf(&md, " (%d reasoning included in output)\n", *t.OutputReasoning)
			} else {
				fmt.Fprint(&md, " (reasoning breakdown unavailable)\n")
			}
		} else {
			fmt.Fprint(&md, "- Tokens: unavailable or invalid\n")
		}
		if r.Cost.EstimatedAPIUSD != nil {
			fmt.Fprintf(&md, "- Pricing-based API estimate: $%.6f USD (caller-supplied USD per million token rates)\n", *r.Cost.EstimatedAPIUSD)
		} else {
			fmt.Fprintf(&md, "- Pricing-based API estimate: unavailable (%s)\n", r.Cost.EstimateStatus)
		}
	}
	if r.RegradedFrom != "" {
		fmt.Fprintf(&md, "- Regraded from saved result: `%s`\n", r.RegradedFrom)
	}
	if r.Artifacts != nil {
		for label, path := range map[string]string{"Patch": r.Artifacts.Patch, "Events": r.Artifacts.Events, "Stderr": r.Artifacts.Stderr, "Last message": r.Artifacts.LastMessage} {
			if path != "" {
				fmt.Fprintf(&md, "- %s artifact: `%s`\n", label, path)
			}
		}
	}
	if r.CandidateSHA256 != "" {
		fmt.Fprintf(&md, "- Candidate SHA-256: `%s`\n", r.CandidateSHA256)
	}
	if r.Acceptance != nil {
		fmt.Fprintf(&md, "- Acceptance passed: %t\n- Acceptance elapsed: %d ms\n", r.Acceptance.Passed, r.Acceptance.ElapsedMS)
		if r.Acceptance.ExitCode != nil {
			fmt.Fprintf(&md, "- Acceptance exit code: %d\n", *r.Acceptance.ExitCode)
		}
		fmt.Fprintf(&md, "- Acceptance timed out: %t\n", r.Acceptance.TimedOut)
	}
	if r.Failure != nil {
		fmt.Fprintf(&md, "\n## Failure\n\n%s\n", r.Failure.Details)
	}
	if r.Agent != nil {
		if r.Agent.StderrTail != "" {
			fmt.Fprintf(&md, "\n## Agent stderr tail\n\n~~~text\n%s\n~~~\n", r.Agent.StderrTail)
		}
		if r.Agent.EventTail != "" {
			fmt.Fprintf(&md, "\n## Agent event tail\n\n~~~text\n%s\n~~~\n", r.Agent.EventTail)
		}
	}
	if r.Acceptance != nil {
		if r.Acceptance.Stdout != "" {
			fmt.Fprintf(&md, "\n## Acceptance stdout\n\n~~~text\n%s\n~~~\n", r.Acceptance.Stdout)
		}
		if r.Acceptance.Stderr != "" {
			fmt.Fprintf(&md, "\n## Acceptance stderr\n\n~~~text\n%s\n~~~\n", r.Acceptance.Stderr)
		}
	}
	return os.WriteFile(filepath.Join(outDir, "result.md"), []byte(md.String()), 0600)
}
