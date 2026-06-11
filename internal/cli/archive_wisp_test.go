package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
)

func TestArchiveRequiresTerminalTask(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "Open Task")

	_, stderr, code := runCLI(t, root, nil, nil, []string{"archive", "a00001"})
	if code != syaerr.ExitUsage || !strings.Contains(stderr, "cannot archive non-terminal tasks: a00001(todo)") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	parsed, err := parseTaskFile(t, findTaskFile(t, root, "a00001"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Archived {
		t.Fatal("non-terminal task was archived")
	}
}

func TestArchiveTerminalTask(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "Done Task")
	mustRun(t, root, nil, []string{"move", "a00001", "in_progress"})
	mustRun(t, root, nil, []string{"close", "a00001"})

	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"archive", "a00001"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "a00001: ok") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	parsed, err := parseTaskFile(t, findTaskFile(t, root, "a00001"))
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.Archived || !strings.Contains(string(parsed.Body.Raw), ": archived") {
		t.Fatalf("archive did not update task:\n%s", parsed.Body.Raw)
	}
}

func TestRestoreRoundTripInGitRepo(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitOKCLI(t, root, "init")
	gitOKCLI(t, root, "config", "user.email", "cli@example.test")
	gitOKCLI(t, root, "config", "user.name", "CLI Test")
	gitOKCLI(t, root, "config", "commit.gpgsign", "false")
	initProject(t, root)
	createSeedTask(t, root, "a00001", "Restore Me", "-d", "Full text")
	mustRun(t, root, nil, []string{"move", "a00001", "in_progress"})
	mustRun(t, root, nil, []string{"close", "a00001"})
	gitOKCLI(t, root, "add", ".")
	gitOKCLI(t, root, "commit", "-m", "full task")

	path := findTaskFile(t, root, "a00001")
	parsed, err := parseTaskFile(t, path)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Body = task.NewBody([]byte("## Description\nCompacted\n"), []task.Section{{Name: "Description", Raw: []byte("## Description\nCompacted\n")}})
	parsed.Archived = true
	data, err := task.Serialize(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	gitOKCLI(t, root, "add", ".")
	gitOKCLI(t, root, "commit", "-m", "archived compact")

	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"restore", "a00001"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "Full text") || strings.Contains(stdout, "Compacted") {
		t.Fatalf("restore show code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}

	stdout, stderr, code = runCLI(t, root, nil, nil, []string{"restore", "a00001", "--apply"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "restored body") {
		t.Fatalf("restore apply code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	restored, err := parseTaskFile(t, path)
	if err != nil {
		t.Fatal(err)
	}
	if !restored.Archived || !strings.Contains(string(restored.Body.Raw), "Full text") || !strings.Contains(string(restored.Body.Raw), "restored from") {
		t.Fatalf("restore apply did not keep archived and restore body:\n%s", restored.Body.Raw)
	}
}

func TestWispLifecycleAndSquash(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	stdout, stderr, code := runCLI(t, root, []string{"abc123"}, nil, []string{"wisp", "create", "Loose Note", "-d", "Wisp body\n"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "created w-abc123") {
		t.Fatalf("create code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, root, nil, nil, []string{"wisp", "list"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "w-abc123") || !strings.Contains(stdout, "Loose Note") {
		t.Fatalf("list code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, root, nil, nil, []string{"wisp", "show", "w-abc"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "Wisp body") {
		t.Fatalf("show code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, root, []string{"t00001"}, nil, []string{"wisp", "squash", "w-abc123", "-t", "feature"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "squashed into t00001") {
		t.Fatalf("squash code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".sya", "wisps", "w-abc123-loose-note.md")); !os.IsNotExist(err) {
		t.Fatalf("wisp was not burned after squash: %v", err)
	}
	created, err := parseTaskFile(t, findTaskFile(t, root, "t00001"))
	if err != nil {
		t.Fatal(err)
	}
	if created.Type != "feature" || !strings.Contains(string(created.Body.Raw), "Wisp body") {
		t.Fatalf("squashed task invalid: %#v\n%s", created, created.Body.Raw)
	}

	stdout, stderr, code = runCLI(t, root, []string{"def456"}, nil, []string{"wisp", "create", "Burn Me"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("create burn target code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	stdout, stderr, code = runCLI(t, root, nil, nil, []string{"wisp", "burn", "w-def456"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "w-def456: burned") {
		t.Fatalf("burn code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestWispIDsDoNotResolveAsTaskRelations(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "Task")
	mustRun(t, root, nil, []string{"wisp", "create", "Loose", "-d", "body"})
	_, stderr, code := runCLI(t, root, nil, nil, []string{"link", "a00001", "depends_on", "w-z00000"})
	if code != syaerr.ExitLookup || !strings.Contains(stderr, "not found: w-z00000") {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
}

func findTaskFile(t *testing.T, root, id string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, ".sya", "tasks", id+"-*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		matches, err = filepath.Glob(filepath.Join(root, ".sya", "tasks", id+".md"))
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("expected one task file for %s, got %v", id, matches)
	}
	return matches[0]
}

func parseTaskFile(t *testing.T, path string) (*task.Task, error) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	parsed, err := task.ParseBytes(data)
	if err != nil {
		return nil, err
	}
	parsed.File = path
	return parsed, nil
}

func gitOKCLI(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=CLI Test",
		"GIT_AUTHOR_EMAIL=cli@example.test",
		"GIT_COMMITTER_NAME=CLI Test",
		"GIT_COMMITTER_EMAIL=cli@example.test",
		"GIT_AUTHOR_DATE=2026-01-02T03:04:05Z",
		"GIT_COMMITTER_DATE=2026-01-02T03:04:05Z",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
