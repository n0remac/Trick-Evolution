package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runGitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeTest(t *testing.T, path, value string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), mode); err != nil {
		t.Fatal(err)
	}
}

func codexFixture(t *testing.T, seconds int) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	repo := filepath.Join(root, "source")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "init", "--quiet")
	runGitTest(t, repo, "config", "user.name", "Fixture")
	runGitTest(t, repo, "config", "user.email", "fixture@example.invalid")
	writeTest(t, filepath.Join(repo, "answer.txt"), "pending\n", 0600)
	writeTest(t, filepath.Join(repo, "obsolete.txt"), "remove me\n", 0600)
	runGitTest(t, repo, "add", "-A")
	runGitTest(t, repo, "commit", "--quiet", "-m", "baseline")
	baseline := runGitTest(t, repo, "rev-parse", "HEAD")
	accept := filepath.Join(root, "accept.sh")
	writeTest(t, accept, "#!/bin/sh\nset -eu\ntest \"$(cat answer.txt)\" = correct\ntest -f added.txt\ntest ! -e obsolete.txt\ntest ! -e .benchmark-last-message\necho accepted-in-clean-checkout\n", 0700)
	pack := filepath.Join(root, "task.json")
	packJSON, _ := json.Marshal(Pack{Name: "codex-stub", Repository: repo, Baseline: baseline, Prompt: "Fix the tiny task.", BudgetSeconds: seconds, AcceptanceCommand: []string{"sh", "{pack_dir}/accept.sh"}})
	writeTest(t, pack, string(packJSON), 0600)
	return root, pack, repo
}

func stubCodex(t *testing.T, root, mode string) string {
	t.Helper()
	path := filepath.Join(root, "stub-codex")
	script := fmt.Sprintf(`#!/bin/sh
set -eu
case " $* " in *" exec "*) ;; *) exit 21;; esac
for flag in --ignore-user-config --ignore-rules --strict-config --ephemeral --json --sandbox --model --config --output-last-message; do
  case " $* " in *" $flag "*) ;; *) echo "missing $flag" >&2; exit 22;; esac
done
case " $* " in *" approval_policy=\"never\" "*) ;; *) echo "wrong approval" >&2; exit 23;; esac
case " $* " in *" sandbox_workspace_write.network_access=false "*) ;; *) exit 24;; esac
case " $* " in *" web_search=\"disabled\" "*) ;; *) exit 25;; esac
case " $* " in *" shell_environment_policy.inherit=\"all\" "*) ;; *) exit 30;; esac
case " $* " in *" --model stub-model "*) ;; *) exit 27;; esac
case " $* " in *" model_reasoning_effort=\"low\" "*|*" model_reasoning_effort=\"medium\" "*) ;; *) exit 28;; esac
test "$PWD" = /workspace
test "$(cat)" = "Fix the tiny task."
test -f .git/shallow
git status --short >/dev/null
if touch .git/write-probe 2>/dev/null; then echo '.git is writable' >&2; exit 29; fi
if git cat-file -e 'HEAD^' 2>/dev/null; then echo 'upstream history visible' >&2; exit 26; fi
test ! -e %q
test ! -e %q
test ! -e /home/pi/.codex/auth.json
test -z "${BENCH_INHERITED_SECRET:-}"
test -f /home/agent/.codex/auth.json
test "$GOPROXY" = off
if [ "${GOMODCACHE:-}" = /gomodcache ]; then
  test -f /gomodcache/cache/download/marker
  if touch /gomodcache/cache/download/write-probe 2>/dev/null; then echo 'module cache is writable' >&2; exit 31; fi
fi
printf 'correct\n' > answer.txt
printf 'new\n' > added.txt
rm obsolete.txt
case %q in
  pass) printf '%%s\n' '{"type":"turn.completed","usage":{"input_tokens":31,"cached_input_tokens":9,"output_tokens":4,"output_tokens_details":{"reasoning_tokens":2}}}' ;;
  missing) printf '%%s\n' '{"type":"turn.completed"}' ;;
  symlink) ln -s %q /artifacts/last-message; printf '%%s\n' '{"type":"turn.completed"}' ;;
  fail) printf 'backend failure detail\n' >&2; exit 17 ;;
  timeout) printf 'working before timeout\n' >&2; sleep 30 ;;
esac
`, filepath.Join(root, "accept.sh"), filepath.Join(root, "prior", "secret.txt"), mode, filepath.Join(root, "prior", "secret.txt"))
	writeTest(t, path, script, 0700)
	return path
}

func TestCodexFailsClosedWhenIsolationUnavailable(t *testing.T) {
	root, pack, _ := codexFixture(t, 5)
	auth := filepath.Join(root, "auth.json")
	writeTest(t, auth, "{}", 0600)
	stub := stubCodex(t, root, "pass")
	t.Setenv("PATH", "/no-such-binary-directory")
	result := RunWithOptions(context.Background(), pack, RunOptions{Adapter: "codex", OutDir: filepath.Join(root, "out"), CodexBinary: stub, AuthFile: auth, Model: "stub-model"})
	if result.Stage != "configure_agent" || result.Failure == nil || !strings.Contains(result.Failure.Details, "bubblewrap isolation unavailable") {
		t.Fatalf("isolation failure: %+v", result)
	}
}

func TestCodexRejectsMissingPreparedCache(t *testing.T) {
	root, pack, _ := codexFixture(t, 5)
	auth := filepath.Join(root, "auth.json")
	writeTest(t, auth, "{}", 0600)
	result := RunWithOptions(context.Background(), pack, RunOptions{
		Adapter: "codex", OutDir: filepath.Join(root, "out"),
		CodexBinary: stubCodex(t, root, "pass"), AuthFile: auth,
		Model: "stub-model", GoModCache: filepath.Join(root, "missing-cache"),
	})
	if result.Stage != "configure_agent" || result.Failure == nil || !strings.Contains(result.Failure.Details, "module cache") {
		t.Fatalf("missing cache: %+v", result)
	}
}

func TestCodexStubIsolationUsagePatchAndCleanGrading(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bubblewrap unavailable")
	}
	root, pack, repo := codexFixture(t, 5)
	if err := os.Mkdir(filepath.Join(root, "prior"), 0700); err != nil {
		t.Fatal(err)
	}
	writeTest(t, filepath.Join(root, "prior", "secret.txt"), "hidden", 0600)
	cache := filepath.Join(root, "module-cache")
	if err := os.MkdirAll(filepath.Join(cache, "cache", "download"), 0700); err != nil {
		t.Fatal(err)
	}
	writeTest(t, filepath.Join(cache, "cache", "download", "marker"), "prepared", 0600)
	auth := filepath.Join(root, "auth.json")
	writeTest(t, auth, "{}", 0600)
	t.Setenv("BENCH_INHERITED_SECRET", "must not leak")
	for _, mode := range []string{"pass", "missing", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			result := RunWithOptions(context.Background(), pack, RunOptions{Adapter: "codex", OutDir: filepath.Join(root, "out-"+mode), CodexBinary: stubCodex(t, root, mode), AuthFile: auth, Model: "stub-model", ReasoningEffort: "low", GoModCache: cache})
			if !result.Passed || result.Agent == nil || result.Agent.ExitCode == nil || *result.Agent.ExitCode != 0 || result.Acceptance == nil || !strings.Contains(result.Acceptance.Stdout, "accepted-in-clean-checkout") {
				t.Fatalf("result: %+v", result)
			}
			if mode == "pass" && !strings.Contains(string(result.Agent.Usage), `"cached_input_tokens":9`) {
				t.Fatalf("usage: %s", result.Agent.Usage)
			}
			if result.Cost == nil || result.Cost.ActualBilledAPIUSD != nil {
				t.Fatalf("actual billing incorrectly reported: %+v", result.Cost)
			}
			if mode == "pass" && (result.Cost.Tokens == nil || result.Cost.Tokens.InputUncached != 22 || result.Cost.Tokens.InputCached != 9 || result.Cost.Tokens.OutputTotal != 4 || result.Cost.Tokens.OutputReasoning == nil || *result.Cost.Tokens.OutputReasoning != 2 || result.Cost.EstimatedAPIUSD != nil) {
				t.Fatalf("token accounting: %+v", result.Cost)
			}
			if mode == "missing" && result.Agent.Usage != nil {
				t.Fatalf("missing usage became %s", result.Agent.Usage)
			}
			if mode == "symlink" && result.Artifacts.LastMessage != "" {
				t.Fatalf("followed candidate symlink: %+v", result.Artifacts)
			}
			if !strings.Contains(result.Agent.Settings["go_module_cache"], "read-only mount") {
				t.Fatalf("module cache policy omitted: %+v", result.Agent.Settings)
			}
			patch, err := os.ReadFile(result.Artifacts.Patch)
			if err != nil || !strings.Contains(string(patch), "new file mode") || !strings.Contains(string(patch), "deleted file mode") {
				t.Fatalf("patch: %v %s", err, patch)
			}
			if err := WriteReports(filepath.Join(root, "out-"+mode), result); err != nil {
				t.Fatal(err)
			}
			if b, err := os.ReadFile(filepath.Join(root, "out-"+mode, "result.md")); err != nil || !strings.Contains(string(b), "Agent usage:") {
				t.Fatalf("markdown: %v %s", err, b)
			}
		})
	}
	if b, err := os.ReadFile(filepath.Join(repo, "answer.txt")); err != nil || string(b) != "pending\n" {
		t.Fatalf("source changed: %s %v", b, err)
	}
}

func TestCodexStubFailureAndTimeoutRetainArtifacts(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("bubblewrap unavailable")
	}
	for _, tc := range []struct {
		mode    string
		seconds int
		stage   string
	}{{"fail", 5, "agent_run"}, {"timeout", 1, "agent_budget"}} {
		t.Run(tc.mode, func(t *testing.T) {
			root, pack, _ := codexFixture(t, tc.seconds)
			if err := os.Mkdir(filepath.Join(root, "prior"), 0700); err != nil {
				t.Fatal(err)
			}
			writeTest(t, filepath.Join(root, "prior", "secret.txt"), "hidden", 0600)
			auth := filepath.Join(root, "auth.json")
			writeTest(t, auth, "{}", 0600)
			result := RunWithOptions(context.Background(), pack, RunOptions{Adapter: "codex", OutDir: filepath.Join(root, "out"), CodexBinary: stubCodex(t, root, tc.mode), AuthFile: auth, Model: "stub-model"})
			if result.Stage != tc.stage || result.Failure == nil || result.Artifacts == nil || result.Artifacts.Patch == "" || result.Agent == nil || result.Agent.StderrTail == "" || result.Agent.ExitCode == nil {
				t.Fatalf("result: %+v", result)
			}
			if tc.mode == "timeout" && (!result.Agent.TimedOut || result.AgentElapsedMS > 15000) {
				t.Fatalf("timeout cleanup: %+v", result)
			}
			if err := WriteReports(filepath.Join(root, "out"), result); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"result.json", "result.md", "candidate.patch", "agent-events.jsonl", "agent-stderr.log"} {
				if _, err := os.Stat(filepath.Join(root, "out", name)); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
