package cli

import (
	"strings"
	"testing"

	"github.com/snjax/sya/internal/syaerr"
)

func TestQueryCommandUsesEngineAndAge(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "f00001", "Feature", "-t", "feature")
	createSeedTask(t, root, "b00001", "Bug", "-t", "bug")
	feature := findTaskFile(t, root, "f00001")
	replaceInFile(t, feature, "created: \"2026-01-02T03:04:05Z\"\n", "created: \"2025-12-20T03:04:05Z\"\n")
	mustRun(t, root, nil, []string{"move", "f00001", "spec"})

	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"query", "blocked and age>7d"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "f00001") || strings.Contains(stdout, "b00001") {
		t.Fatalf("blocked age query stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, root, nil, nil, []string{"query", "ready and type=bug"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "b00001") || strings.Contains(stdout, "f00001") {
		t.Fatalf("ready query stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}

func TestQueryParseErrorIsUsage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	_, stderr, code := runCLI(t, root, nil, nil, []string{"query", "type="})
	if code != syaerr.ExitUsage || !strings.Contains(stderr, "query parse error") || !strings.Contains(stderr, "expected value") {
		t.Fatalf("stderr=%q code=%d", stderr, code)
	}
}

func TestStatsCommand(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "A")
	createSeedTask(t, root, "b00001", "B", "-t", "bug")
	mustRun(t, root, nil, []string{"move", "a00001", "in_progress"})

	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"--json", "stats"})
	if code != syaerr.ExitOK || stderr != "" ||
		!strings.Contains(stdout, `"active":2`) ||
		!strings.Contains(stdout, `"ready":2`) ||
		!strings.Contains(stdout, `"wisps":0`) {
		t.Fatalf("stats stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}

func TestStaleCommand(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "o00001", "Old")
	createSeedTask(t, root, "n00001", "New")
	oldPath := findTaskFile(t, root, "o00001")
	replaceInFile(t, oldPath, "2026-01-02T03:04:05Z @unknown: created", "2025-12-01T03:04:05Z @unknown: created")

	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"stale", "--days", "14"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "o00001") || strings.Contains(stdout, "n00001") {
		t.Fatalf("stale stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}
