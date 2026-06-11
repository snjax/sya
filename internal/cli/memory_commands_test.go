package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snjax/sya/internal/syaerr"
)

func TestPrimeMemoryGoldens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T, root string) (stdout, stderr string, code int)
	}{
		{name: "prime_human", run: func(t *testing.T, root string) (string, string, int) {
			primeFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"prime"})
		}},
		{name: "prime_json", run: func(t *testing.T, root string) (string, string, int) {
			primeFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "prime"})
		}},
		{name: "remember_human", run: func(t *testing.T, root string) (string, string, int) {
			fixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"remember", "Стриминг API uses SSE", "--task", "f000", "--task", "missing"})
		}},
		{name: "remember_json", run: func(t *testing.T, root string) (string, string, int) {
			fixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "remember", "Стриминг API uses SSE", "--task", "f000", "--task", "missing"})
		}},
		{name: "recall_list_human", run: func(t *testing.T, root string) (string, string, int) {
			memoryFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"recall"})
		}},
		{name: "recall_list_json", run: func(t *testing.T, root string) (string, string, int) {
			memoryFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "recall"})
		}},
		{name: "recall_one_human", run: func(t *testing.T, root string) (string, string, int) {
			memoryFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"recall", "streaming-api-uses-sse"})
		}},
		{name: "recall_one_json", run: func(t *testing.T, root string) (string, string, int) {
			memoryFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "recall", "streaming-api-uses-sse"})
		}},
		{name: "forget_human", run: func(t *testing.T, root string) (string, string, int) {
			memoryFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"forget", "streaming-api-uses-sse"})
		}},
		{name: "forget_json", run: func(t *testing.T, root string) (string, string, int) {
			memoryFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "forget", "streaming-api-uses-sse"})
		}},
		{name: "show_memory_human", run: func(t *testing.T, root string) (string, string, int) {
			memoryFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"show", "f000"})
		}},
		{name: "show_memory_json", run: func(t *testing.T, root string) (string, string, int) {
			memoryFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "show", "f000"})
		}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			stdout, stderr, code := tt.run(t, root)
			got := normalizeCommandOutput(root, stdout, stderr, code)
			wantBytes, err := os.ReadFile(filepath.Join("testdata", "memory", tt.name+".golden"))
			if err != nil {
				t.Fatalf("read golden: %v\n\ngot:\n%s", err, got)
			}
			want := strings.TrimSpace(string(wantBytes))
			if strings.TrimSpace(got) != want {
				t.Fatalf("golden mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
			}
		})
	}
}

func TestPrimeOutsideProjectSilent(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"prime"})
	if code != syaerr.ExitOK || stdout != "" || stderr != "" {
		t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}

func TestRememberTaskLinkRoundTrip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fixtureProject(t, root)
	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"--json", "remember", "Fact one", "--key", "deploy", "--task", "f000"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, `"tasks":["f00001"]`) {
		t.Fatalf("remember stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, root, nil, nil, []string{"--json", "recall", "deploy"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, `"tasks":["f00001"]`) || !strings.Contains(stdout, `"body":"Fact one\n"`) {
		t.Fatalf("recall stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}

func memoryFixtureProject(t *testing.T, root string) {
	t.Helper()
	fixtureProject(t, root)
	if err := os.WriteFile(filepath.Join(root, ".sya", "config.yml"), []byte("project: fixture\nprefix: sya\nid_length: 6\narchive:\n  after_days: 30\n"), 0o644); err != nil {
		t.Fatalf("write fixture config: %v", err)
	}
	mustRun(t, root, nil, []string{"remember", "Streaming API uses SSE", "--task", "f000"})
	mustRun(t, root, nil, []string{"remember", "Deploys require green doctor", "--key", "deploy-process", "--task", "d000"})
}

func primeFixtureProject(t *testing.T, root string) {
	t.Helper()
	memoryFixtureProject(t, root)
	mustRun(t, root, nil, []string{"move", "d00001", "in_progress"})
	mustRun(t, root, nil, []string{"update", "d00001", "--assignee", "codex"})
}
