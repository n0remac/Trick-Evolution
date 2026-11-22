package benchmark

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// prepareCodex validates the outer isolation mechanism before any agent starts.
// The CLI binary and one auth file are the only host paths mounted beyond
// system binaries, certificates, and the candidate checkout.
func prepareCodex(r *Result, opts *RunOptions) error {
	if strings.TrimSpace(opts.Model) == "" || strings.ContainsAny(opts.Model, "\n\r") {
		return errors.New("Codex requires -model with a single-line model name")
	}
	if opts.ReasoningEffort == "" {
		opts.ReasoningEffort = "medium"
	}
	switch opts.ReasoningEffort {
	case "low", "medium", "high", "xhigh", "max", "ultra":
	default:
		return fmt.Errorf("unsupported reasoning effort %q", opts.ReasoningEffort)
	}
	if opts.CodexBinary == "" {
		opts.CodexBinary = "codex"
	}
	bin, err := exec.LookPath(opts.CodexBinary)
	if err != nil {
		return fmt.Errorf("Codex executable: %w", err)
	}
	if opts.CodexBinary, err = filepath.EvalSymlinks(bin); err != nil {
		return fmt.Errorf("Codex executable: %w", err)
	}
	if opts.CodexBinary, err = filepath.Abs(opts.CodexBinary); err != nil {
		return err
	}
	if opts.AuthFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		opts.AuthFile = filepath.Join(home, ".codex", "auth.json")
	}
	if opts.AuthFile, err = filepath.Abs(opts.AuthFile); err != nil {
		return err
	}
	info, err := os.Stat(opts.AuthFile)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("Codex auth file unavailable at %s (set -codex-auth-file): %v", opts.AuthFile, err)
	}
	if opts.GoModCache != "" {
		if opts.GoModCache, err = filepath.Abs(opts.GoModCache); err != nil {
			return err
		}
		if opts.GoModCache, err = filepath.EvalSymlinks(opts.GoModCache); err != nil {
			return fmt.Errorf("prepared Go module cache unavailable: %w", err)
		}
		info, err := os.Stat(filepath.Join(opts.GoModCache, "cache", "download"))
		if err != nil || !info.IsDir() {
			return fmt.Errorf("prepared Go module cache at %s must contain cache/download: %v", opts.GoModCache, err)
		}
	}
	if opts.OutDir, err = filepath.Abs(opts.OutDir); err != nil {
		return err
	}
	if err := os.MkdirAll(opts.OutDir, 0700); err != nil {
		return err
	}
	if _, err := exec.LookPath("bwrap"); err != nil {
		return fmt.Errorf("required bubblewrap isolation unavailable: %w", err)
	}
	probe := exec.Command("bwrap", "--unshare-all", "--share-net", "--die-with-parent", "--ro-bind", "/usr", "/usr", "--symlink", "usr/bin", "/bin", "--symlink", "usr/lib", "/lib", "--dev", "/dev", "--proc", "/proc", "--tmpfs", "/tmp", "--dir", "/workspace", "--chdir", "/workspace", "/usr/bin/true")
	if output, err := probe.CombinedOutput(); err != nil {
		return fmt.Errorf("required bubblewrap isolation unavailable: %w: %s", err, strings.TrimSpace(string(output)))
	}
	r.Agent = &AgentRun{
		Model: opts.Model, ReasoningEffort: opts.ReasoningEffort, CLI: opts.CodexBinary,
		Isolation: "bubblewrap: private mount and PID namespaces; single-commit checkout",
		Sandbox:   "workspace-write", Approval: "never", ToolNetwork: "disabled", WebSearch: "disabled",
		Settings: map[string]string{
			"session":                          "new ephemeral noninteractive",
			"user_config":                      "ignored",
			"exec_rules":                       "ignored",
			"strict_config":                    "true",
			"shell_environment_policy.inherit": "all (curated outer environment only)",
			"backend_network":                  "host namespace for authenticated API requests",
			"goproxy":                          "off",
		},
	}
	if opts.GoModCache == "" {
		r.Agent.Settings["go_module_cache"] = "empty private tmpfs"
	} else {
		r.Agent.Settings["go_module_cache"] = opts.GoModCache + " (read-only mount)"
	}
	return nil
}

func codexBwrapArgs(checkout, authFile, messageDir, goModCache string) []string {
	args := []string{
		"--unshare-all", "--share-net", "--die-with-parent", "--clearenv",
		"--ro-bind", "/usr", "/usr", "--symlink", "usr/bin", "/bin", "--symlink", "usr/lib", "/lib",
		"--ro-bind", "/usr/local/go", "/usr/local/go",
		"--dir", "/etc", "--dir", "/etc/ssl", "--dir", "/etc/ssl/certs",
		"--ro-bind", "/etc/ssl/certs/ca-certificates.crt", "/etc/ssl/certs/ca-certificates.crt",
		"--ro-bind", "/etc/resolv.conf", "/etc/resolv.conf",
		"--dev", "/dev", "--proc", "/proc", "--tmpfs", "/tmp",
		"--dir", "/home", "--tmpfs", "/home/agent", "--dir", "/home/agent/.codex",
		"--bind", checkout, "/workspace", "--ro-bind", filepath.Join(checkout, ".git"), "/workspace/.git",
		"--bind", messageDir, "/artifacts", "--ro-bind", authFile, "/home/agent/.codex/auth.json",
		"--setenv", "HOME", "/home/agent", "--setenv", "CODEX_HOME", "/home/agent/.codex",
		"--setenv", "PATH", "/usr/local/go/bin:/usr/bin:/bin", "--setenv", "TMPDIR", "/tmp",
		"--setenv", "GOCACHE", "/tmp/gocache",
		"--setenv", "GOTOOLCHAIN", "local", "--setenv", "GOPROXY", "off", "--setenv", "GOSUMDB", "off", "--setenv", "GOWORK", "off",
		"--setenv", "GIT_CONFIG_NOSYSTEM", "1", "--setenv", "GIT_CONFIG_GLOBAL", "/dev/null",
		"--setenv", "GIT_TERMINAL_PROMPT", "0", "--setenv", "SSL_CERT_FILE", "/etc/ssl/certs/ca-certificates.crt",
		"--chdir", "/workspace",
	}
	if goModCache == "" {
		args = append(args, "--setenv", "GOMODCACHE", "/tmp/gomodcache")
	} else {
		args = append(args, "--ro-bind", goModCache, "/gomodcache", "--setenv", "GOMODCACHE", "/gomodcache")
	}
	return args
}

func runCodex(ctx context.Context, checkout, prompt string, opts RunOptions, r *Result) error {
	messageDir, err := os.MkdirTemp("", "agent-benchmark-message-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(messageDir)
	events := filepath.Join(opts.OutDir, "agent-events.jsonl")
	stderrPath := filepath.Join(opts.OutDir, "agent-stderr.log")
	lastPath := filepath.Join(opts.OutDir, "agent-last-message.txt")
	stdoutFile, err := os.OpenFile(events, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer stdoutFile.Close()
	stderrFile, err := os.OpenFile(stderrPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer stderrFile.Close()
	r.Artifacts = &Artifacts{Events: events, Stderr: stderrPath, LastMessage: lastPath}

	args := codexBwrapArgs(checkout, opts.AuthFile, messageDir, opts.GoModCache)
	// Standalone Codex spawns the code-mode host by absolute path. It must be
	// mounted explicitly: the CLI binary alone is not enough for shell tools.
	if filepath.Base(opts.CodexBinary) == "codex" {
		host := filepath.Join(filepath.Dir(opts.CodexBinary), "codex-code-mode-host")
		if info, err := os.Stat(host); err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
			return fmt.Errorf("Codex code-mode host unavailable at %s: %v", host, err)
		}
		args = append(args, "--ro-bind", host, "/codex-code-mode-host")
	}
	args = append(args, "--ro-bind", opts.CodexBinary, "/runner-codex", "--", "/runner-codex", "exec",
		"--ignore-user-config", "--ignore-rules", "--strict-config", "--ephemeral", "--json",
		"--sandbox", "workspace-write", "--model", opts.Model,
		"--config", "approval_policy=\"never\"",
		"--config", "sandbox_workspace_write.network_access=false",
		"--config", "web_search=\"disabled\"",
		"--config", "shell_environment_policy.inherit=\"all\"",
		"--config", "model_reasoning_effort=\""+opts.ReasoningEffort+"\"",
		"--output-last-message", "/artifacts/last-message", "-",
	)
	cmd := exec.Command("bwrap", args...)
	cmd.Dir = checkout
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	cmd.Stdin = strings.NewReader(prompt)
	cmd.Stdout, cmd.Stderr = stdoutFile, stderrFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start isolated Codex: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var runErr error
	select {
	case runErr = <-done:
	case <-ctx.Done():
		r.Agent.TimedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		select {
		case runErr = <-done:
		case <-time.After(time.Second):
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			runErr = <-done
		}
	}
	lastInSandbox := filepath.Join(messageDir, "last-message")
	// Never follow a link created by the backend when collecting its message.
	if source, err := os.OpenFile(lastInSandbox, os.O_RDONLY|syscall.O_NOFOLLOW, 0); err == nil {
		if info, err := source.Stat(); err == nil && info.Mode().IsRegular() {
			if dest, err := os.OpenFile(lastPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600); err == nil {
				_, _ = io.CopyN(dest, source, 1<<20)
				_ = dest.Close()
			}
		}
		_ = source.Close()
	}
	if _, err := os.Stat(lastPath); err != nil {
		r.Artifacts.LastMessage = ""
	}
	r.Agent.EventTail = fileTail(events)
	r.Agent.StderrTail = fileTail(stderrPath)
	r.Agent.Usage = lastUsage(events)
	r.Cost = codexCost(r.Agent.Usage, opts.Prices)
	if runErr == nil {
		code := 0
		r.Agent.ExitCode = &code
	} else {
		var exit *exec.ExitError
		if errors.As(runErr, &exit) {
			code := exit.ExitCode()
			r.Agent.ExitCode = &code
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if runErr != nil {
		return fmt.Errorf("Codex exited: %w: %s", runErr, r.Agent.StderrTail)
	}
	completed, failed := eventOutcome(events)
	if failed {
		return fmt.Errorf("Codex reported turn.failed: %s", r.Agent.EventTail)
	}
	if !completed {
		return fmt.Errorf("Codex exited without a turn.completed event: %s", r.Agent.EventTail)
	}
	return nil
}

func fileTail(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	if info.Size() > maxOutput {
		if _, err := f.Seek(-maxOutput, 2); err != nil {
			return ""
		}
		b := make([]byte, maxOutput)
		n, _ := f.Read(b)
		return "[earlier output omitted]\n" + string(b[:n])
	}
	b := make([]byte, info.Size())
	n, _ := f.Read(b)
	return string(b[:n])
}

func lastUsage(path string) json.RawMessage {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var usage json.RawMessage
	for s.Scan() {
		var event struct {
			Type  string          `json:"type"`
			Usage json.RawMessage `json:"usage"`
		}
		if json.Unmarshal(s.Bytes(), &event) == nil && event.Type == "turn.completed" && len(event.Usage) > 0 && !bytes.Equal(event.Usage, []byte("null")) {
			usage = append(json.RawMessage(nil), event.Usage...)
		}
	}
	return usage
}

func eventOutcome(path string) (completed, failed bool) {
	f, err := os.Open(path)
	if err != nil {
		return false, false
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for s.Scan() {
		var event struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(s.Bytes(), &event) == nil {
			if event.Type == "turn.failed" {
				failed = true
			}
			if event.Type == "turn.completed" {
				completed = true
			}
		}
	}
	return completed, failed
}
