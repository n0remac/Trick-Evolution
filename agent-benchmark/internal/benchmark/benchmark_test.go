package benchmark

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestTinyFixturePassingAndFailing(t *testing.T) {
	root := t.TempDir()
	setup := filepath.Join("..", "..", "testdata", "tiny", "setup.sh")
	cmd := exec.Command("sh", setup, root)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("set up fixture: %v: %s", err, out)
	}
	pack := filepath.Join(root, "task.json")
	fixtures := filepath.Join("..", "..", "testdata", "tiny")

	passed := Run(context.Background(), pack, filepath.Join(fixtures, "passing.patch"))
	if !passed.Passed || passed.Stage != "complete" || passed.Acceptance == nil || !passed.Acceptance.Passed || passed.CandidateSHA256 == "" {
		t.Fatalf("passing patch: %+v", passed)
	}
	if err := WriteReports(filepath.Join(root, "passing"), passed); err != nil {
		t.Fatal(err)
	}

	failed := Run(context.Background(), pack, filepath.Join(fixtures, "failing.patch"))
	if failed.Passed || failed.Stage != "acceptance" || failed.Acceptance == nil || failed.Acceptance.ExitCode == nil || *failed.Acceptance.ExitCode != 1 || failed.Failure == nil || !strings.Contains(failed.Acceptance.Stderr, "expected correct, got incorrect") {
		t.Fatalf("failing patch: %+v", failed)
	}
	if err := WriteReports(filepath.Join(root, "failing"), failed); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"passing", "failing"} {
		for _, file := range []string{"result.json", "result.md"} {
			if _, err := os.Stat(filepath.Join(root, name, file)); err != nil {
				t.Fatal(err)
			}
		}
	}
	for name, want := range map[string]bool{"passing": true, "failing": false} {
		b, err := os.ReadFile(filepath.Join(root, name, "result.json"))
		if err != nil {
			t.Fatal(err)
		}
		var report Result
		if err := json.Unmarshal(b, &report); err != nil || report.Passed != want || report.Acceptance == nil || report.Acceptance.Passed != want {
			t.Fatalf("%s report: %+v, %v", name, report, err)
		}
	}
	b, err := os.ReadFile(filepath.Join(root, "repo", "answer.txt"))
	if err != nil || string(b) != "pending\n" {
		t.Fatalf("baseline repository changed: %q, %v", b, err)
	}
}
