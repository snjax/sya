package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/snjax/sya/internal/task"
)

var (
	syaBin   string
	repoRoot string
)

type runResult struct {
	code   int
	stdout string
	stderr string
}

func TestMain(m *testing.M) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "cannot locate test file")
		os.Exit(2)
	}
	repoRoot = filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if env := os.Getenv("SYA_BIN"); env != "" {
		syaBin = env
		os.Exit(m.Run())
	}
	dir, err := os.MkdirTemp("", "sya-integration-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer os.RemoveAll(dir)
	syaBin = filepath.Join(dir, "sya")
	cmd := exec.Command("/usr/bin/go", "build", "-trimpath", "-o", syaBin, "./cmd/sya")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "build sya: %v\n%s", err, out)
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func TestParallelCreationMergeDoctorClean(t *testing.T) {
	t.Parallel()
	repo := initGitProject(t)

	gitOK(t, repo, "checkout", "-b", "branch-a")
	createTask(t, repo, "Branch A", "-t", "feature")
	commitAll(t, repo, "branch a task")

	gitOK(t, repo, "checkout", "master")
	gitOK(t, repo, "checkout", "-b", "branch-b")
	createTask(t, repo, "Branch B", "-t", "feature")
	commitAll(t, repo, "branch b task")

	gitOK(t, repo, "checkout", "master")
	gitOK(t, repo, "merge", "--no-edit", "branch-a")
	gitOK(t, repo, "merge", "--no-edit", "branch-b")
	doctorClean(t, repo)
}

func TestLogOnlyMergeConflictFixMerge(t *testing.T) {
	t.Parallel()
	repo := initGitProject(t)
	path := filepath.Join(repo, ".sya", "tasks", "log001-log.md")
	writeTask(t, path, taskFile("log001", "Log conflict", "todo", "", "## Log\n- 2026-06-11T12:00:00Z @test: created\n"))
	commitAll(t, repo, "seed log task")

	gitOK(t, repo, "checkout", "-b", "log-ours")
	appendFile(t, path, "- 2026-06-11T12:01:00Z @ours: appended\n")
	commitAll(t, repo, "ours log")

	gitOK(t, repo, "checkout", "master")
	gitOK(t, repo, "checkout", "-b", "log-theirs")
	appendFile(t, path, "- 2026-06-11T12:02:00Z @theirs: appended\n")
	commitAll(t, repo, "theirs log")

	gitOK(t, repo, "checkout", "master")
	gitOK(t, repo, "merge", "--no-edit", "log-ours")
	merge := runGit(t, repo, "merge", "--no-edit", "log-theirs")
	if merge.code == 0 {
		t.Fatalf("expected log merge conflict, got success\nstdout=%s\nstderr=%s", merge.stdout, merge.stderr)
	}

	result := runSya(t, repo, nil, "doctor", "--fix-merge")
	if result.code != 0 {
		t.Fatalf("doctor --fix-merge failed: code=%d stdout=%s stderr=%s", result.code, result.stdout, result.stderr)
	}
	data := readFile(t, path)
	assertNotContains(t, data, "<<<<<<<")
	assertContains(t, data, "id: log001")
	assertOrder(t, data, "@ours: appended", "@theirs: appended")
	doctorClean(t, repo)
}

func TestFrontmatterConflictFixMergeRefusedAndListWorks(t *testing.T) {
	t.Parallel()
	repo := initGitProject(t)
	path := filepath.Join(repo, ".sya", "tasks", "fm001-frontmatter.md")
	writeTask(t, path, taskFile("fm001", "Frontmatter conflict", "todo", "", ""))
	writeTask(t, filepath.Join(repo, ".sya", "tasks", "ok000-unaffected.md"), taskFile("ok000", "Unaffected", "todo", "", ""))
	commitAll(t, repo, "seed frontmatter task")

	gitOK(t, repo, "checkout", "-b", "status-a")
	replaceInFile(t, path, "status: todo\n", "status: in_progress\n")
	commitAll(t, repo, "status in progress")

	gitOK(t, repo, "checkout", "master")
	gitOK(t, repo, "checkout", "-b", "status-b")
	replaceInFile(t, path, "status: todo\n", "status: scrapped\n")
	commitAll(t, repo, "status scrapped")

	gitOK(t, repo, "checkout", "master")
	gitOK(t, repo, "merge", "--no-edit", "status-a")
	merge := runGit(t, repo, "merge", "--no-edit", "status-b")
	if merge.code == 0 {
		t.Fatalf("expected frontmatter merge conflict, got success")
	}

	result := runSya(t, repo, nil, "doctor", "--fix-merge")
	if result.code != 4 {
		t.Fatalf("expected doctor refusal exit 4, got code=%d stdout=%s stderr=%s", result.code, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "conflict_markers")
	assertContains(t, readFile(t, path), "<<<<<<<")

	list := runSya(t, repo, nil, "list")
	if list.code != 0 {
		t.Fatalf("list should work with conflicted task quarantined: code=%d stdout=%s stderr=%s", list.code, list.stdout, list.stderr)
	}
	assertContains(t, list.stdout, "ok000")
}

func TestDanglingRefAfterMergeDoctorReports(t *testing.T) {
	t.Parallel()
	repo := initGitProject(t)
	source := createTask(t, repo, "Feature F")
	target := createTask(t, repo, "Task X")
	commitAll(t, repo, "seed relation tasks")

	gitOK(t, repo, "checkout", "-b", "link-target")
	mustOK(t, runSya(t, repo, nil, "link", source, "depends_on", target))
	commitAll(t, repo, "link to target")

	gitOK(t, repo, "checkout", "master")
	gitOK(t, repo, "checkout", "-b", "delete-target")
	removeTaskFile(t, repo, target)
	commitAll(t, repo, "delete target")

	gitOK(t, repo, "checkout", "master")
	gitOK(t, repo, "merge", "--no-edit", "link-target")
	gitOK(t, repo, "merge", "--no-edit", "delete-target")

	result := runSya(t, repo, nil, "doctor")
	if result.code != 4 {
		t.Fatalf("expected doctor exit 4, got code=%d stdout=%s stderr=%s", result.code, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "dangling_relation")
}

func TestDuplicateIDReassignAfterMergeUpdatesInboundRefs(t *testing.T) {
	t.Parallel()
	repo := initGitProject(t)
	writeTask(t, filepath.Join(repo, ".sya", "tasks", "bbbbbb-ref.md"), taskFile("bbbbbb", "Ref", "todo", "relations:\n  depends_on: [aaaaaa]\n  discovered_from: [aaaaaa]\n  relates: [aaaaaa]\n", ""))
	commitAll(t, repo, "seed refs")

	gitOK(t, repo, "checkout", "-b", "dup-a")
	writeTask(t, filepath.Join(repo, ".sya", "tasks", "aaaaaa-one.md"), taskFile("aaaaaa", "One", "todo", "", ""))
	commitAll(t, repo, "duplicate one")

	gitOK(t, repo, "checkout", "master")
	gitOK(t, repo, "checkout", "-b", "dup-b")
	writeTask(t, filepath.Join(repo, ".sya", "tasks", "aaaaaa-two.md"), taskFile("aaaaaa", "Two", "todo", "", ""))
	commitAll(t, repo, "duplicate two")

	gitOK(t, repo, "checkout", "master")
	gitOK(t, repo, "merge", "--no-edit", "dup-a")
	gitOK(t, repo, "merge", "--no-edit", "dup-b")

	dirty := runSya(t, repo, nil, "doctor")
	if dirty.code != 4 {
		t.Fatalf("expected duplicate-id doctor exit 4, got code=%d stdout=%s stderr=%s", dirty.code, dirty.stdout, dirty.stderr)
	}
	assertContains(t, dirty.stdout, "duplicate_id")

	fixed := runSya(t, repo, nil, "doctor", "--reassign-id", "aaaaaa")
	if fixed.code != 0 {
		t.Fatalf("doctor --reassign-id failed: code=%d stdout=%s stderr=%s", fixed.code, fixed.stdout, fixed.stderr)
	}
	ref := readFile(t, filepath.Join(repo, ".sya", "tasks", "bbbbbb-ref.md"))
	if strings.Count(ref, "aaaaaa") != 0 {
		t.Fatalf("inbound refs still contain old id:\n%s", ref)
	}
	assertContains(t, ref, "depends_on:")
	assertContains(t, ref, "discovered_from:")
	assertContains(t, ref, "relates:")
	doctorClean(t, repo)
}

func TestSchemaDriftAfterMergeDoctorReportsConformance(t *testing.T) {
	t.Parallel()
	repo := initGitProject(t)
	id := createTask(t, repo, "Needs implementation")
	commitAll(t, repo, "seed task")

	gitOK(t, repo, "checkout", "-b", "tight-schema")
	schemaPath := filepath.Join(repo, ".sya", "schema.yml")
	replaceInFile(t, schemaPath, "pipeline: [todo, in_progress, done, scrapped]", "pipeline: [todo, done, scrapped]")
	commitAll(t, repo, "remove in progress status")

	gitOK(t, repo, "checkout", "master")
	gitOK(t, repo, "checkout", "-b", "task-progress")
	mustOK(t, runSya(t, repo, nil, "claim", id))
	commitAll(t, repo, "claim task")

	gitOK(t, repo, "checkout", "master")
	gitOK(t, repo, "merge", "--no-edit", "tight-schema")
	gitOK(t, repo, "merge", "--no-edit", "task-progress")

	result := runSya(t, repo, nil, "doctor")
	if result.code != 4 {
		t.Fatalf("expected schema conformance error, got code=%d stdout=%s stderr=%s", result.code, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "task_status_unknown")
	assertContains(t, result.stdout, id)
}

func TestConcurrentClaimRace(t *testing.T) {
	t.Parallel()
	for round := 0; round < 50; round++ {
		repo := initGitProject(t)
		id := createTask(t, repo, fmt.Sprintf("Race %d", round))

		results := runConcurrentClaims(t, repo, id)
		var winners []claimResult
		var losers []claimResult
		for _, result := range results {
			switch result.code {
			case 0:
				winners = append(winners, result)
			case 3:
				losers = append(losers, result)
			}
		}
		if len(winners) != 1 || len(losers) != 1 {
			t.Fatalf("round %d: expected one winner and one loser, got %+v", round, results)
		}
		assertContains(t, losers[0].stdout+losers[0].stderr, "already claimed")

		claimed := parseTaskByID(t, repo, id)
		if claimed.Assignee != winners[0].actor {
			t.Fatalf("round %d: assignee=%q, want winning actor %q\nwinner=%+v\nloser=%+v", round, claimed.Assignee, winners[0].actor, winners[0], losers[0])
		}
	}
}

func TestLogOnlyRebaseConflictFixMerge(t *testing.T) {
	t.Parallel()
	repo := initGitProject(t)
	path := filepath.Join(repo, ".sya", "tasks", "reb001-log.md")
	writeTask(t, path, taskFile("reb001", "Rebase log conflict", "todo", "", "## Log\n- 2026-06-11T12:00:00Z @test: created\n"))
	commitAll(t, repo, "seed rebase task")

	gitOK(t, repo, "checkout", "-b", "rebase-ours")
	appendFile(t, path, "- 2026-06-11T12:01:00Z @ours: rebase\n")
	commitAll(t, repo, "ours rebase log")

	gitOK(t, repo, "checkout", "master")
	gitOK(t, repo, "checkout", "-b", "rebase-theirs")
	appendFile(t, path, "- 2026-06-11T12:02:00Z @theirs: rebase\n")
	commitAll(t, repo, "theirs rebase log")

	rebase := runGit(t, repo, "rebase", "rebase-ours")
	if rebase.code == 0 {
		t.Fatalf("expected rebase conflict, got success")
	}
	result := runSya(t, repo, nil, "doctor", "--fix-merge")
	if result.code != 0 {
		t.Fatalf("doctor --fix-merge failed during rebase: code=%d stdout=%s stderr=%s", result.code, result.stdout, result.stderr)
	}
	data := readFile(t, path)
	assertNotContains(t, data, "<<<<<<<")
	assertOrder(t, data, "@ours: rebase", "@theirs: rebase")
	doctorClean(t, repo)
}

func initGitProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitOK(t, dir, "init")
	gitOK(t, dir, "config", "user.email", "integration@example.test")
	gitOK(t, dir, "config", "user.name", "Integration Test")
	gitOK(t, dir, "config", "commit.gpgsign", "false")
	mustOK(t, runSya(t, dir, nil, "init"))
	commitAll(t, dir, "init sya")
	return dir
}

type claimResult struct {
	actor string
	runResult
}

func runConcurrentClaims(t *testing.T, repo, id string) []claimResult {
	t.Helper()
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]claimResult, 2)
	for i, actor := range []string{"claim-a", "claim-b"} {
		i, actor := i, actor
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results[i] = claimResult{
				actor:     actor,
				runResult: runSyaWithEnv(t, repo, nil, []string{"SYA_ACTOR=" + actor}, "claim", id),
			}
		}()
	}
	close(start)
	wg.Wait()
	return results
}

func createTask(t *testing.T, repo, title string, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"--json", "create", title}, args...)
	result := runSya(t, repo, nil, cmdArgs...)
	mustOK(t, result)
	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.stdout), &envelope); err != nil {
		t.Fatalf("parse create json: %v\n%s", err, result.stdout)
	}
	if !envelope.OK || envelope.Data.ID == "" {
		t.Fatalf("bad create envelope: %s", result.stdout)
	}
	return envelope.Data.ID
}

func doctorClean(t *testing.T, repo string) {
	t.Helper()
	result := runSya(t, repo, nil, "doctor")
	if result.code != 0 {
		t.Fatalf("doctor not clean: code=%d stdout=%s stderr=%s", result.code, result.stdout, result.stderr)
	}
	assertContains(t, result.stdout, "doctor: clean")
}

func runSya(t *testing.T, dir string, stdin io.Reader, args ...string) runResult {
	t.Helper()
	return runSyaWithEnv(t, dir, stdin, nil, args...)
}

func runSyaWithEnv(t *testing.T, dir string, stdin io.Reader, extraEnv []string, args ...string) runResult {
	t.Helper()
	cmd := exec.Command(syaBin, args...)
	cmd.Dir = dir
	cmd.Stdin = stdin
	cmd.Env = append(baseEnv(), "NO_COLOR=1", "SYA_ACTOR=integration")
	cmd.Env = append(cmd.Env, extraEnv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("run sya %v: %v", args, err)
		}
	}
	return runResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func runGit(t *testing.T, dir string, args ...string) runResult {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(baseEnv(),
		"GIT_AUTHOR_NAME=Integration Test",
		"GIT_AUTHOR_EMAIL=integration@example.test",
		"GIT_COMMITTER_NAME=Integration Test",
		"GIT_COMMITTER_EMAIL=integration@example.test",
		"GIT_AUTHOR_DATE=2026-06-11T12:00:00Z",
		"GIT_COMMITTER_DATE=2026-06-11T12:00:00Z",
		"GIT_MERGE_AUTOEDIT=no",
		"GIT_EDITOR=true",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("run git %v: %v", args, err)
		}
	}
	return runResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func gitOK(t *testing.T, dir string, args ...string) {
	t.Helper()
	result := runGit(t, dir, args...)
	if result.code != 0 {
		t.Fatalf("git %v failed: code=%d stdout=%s stderr=%s", args, result.code, result.stdout, result.stderr)
	}
}

func commitAll(t *testing.T, repo, message string) {
	t.Helper()
	gitOK(t, repo, "add", ".")
	gitOK(t, repo, "commit", "-m", message)
}

func mustOK(t *testing.T, result runResult) {
	t.Helper()
	if result.code != 0 {
		t.Fatalf("command failed: code=%d stdout=%s stderr=%s", result.code, result.stdout, result.stderr)
	}
}

func baseEnv() []string {
	env := os.Environ()
	out := make([]string, 0, len(env)+1)
	for _, item := range env {
		if strings.HasPrefix(item, "GIT_DIR=") || strings.HasPrefix(item, "GIT_WORK_TREE=") {
			continue
		}
		out = append(out, item)
	}
	return out
}

func taskFile(id, title, status, extraFrontmatter, body string) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", id)
	b.WriteString("type: task\n")
	fmt.Fprintf(&b, "title: %s\n", title)
	fmt.Fprintf(&b, "status: %s\n", status)
	if extraFrontmatter != "" {
		b.WriteString(extraFrontmatter)
	}
	b.WriteString("schema_version: 1\n")
	b.WriteString("---\n")
	b.WriteString(body)
	return b.String()
}

func removeTaskFile(t *testing.T, repo, id string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(repo, ".sya", "tasks", id+"-*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one task file for %s, got %v", id, matches)
	}
	if err := os.Remove(matches[0]); err != nil {
		t.Fatal(err)
	}
}

func parseTaskByID(t *testing.T, repo, id string) *task.Task {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(repo, ".sya", "tasks", id+"-*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("expected one task file for %s, got %v", id, matches)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := task.ParseBytes(data)
	if err != nil {
		t.Fatalf("task %s did not parse after race: %v\n%s", id, err, data)
	}
	return parsed
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeTask(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendFile(t *testing.T, path, data string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString(data); err != nil {
		t.Fatal(err)
	}
}

func replaceInFile(t *testing.T, path, old, new string) {
	t.Helper()
	data := readFile(t, path)
	if !strings.Contains(data, old) {
		t.Fatalf("%s does not contain %q", path, old)
	}
	data = strings.Replace(data, old, new, 1)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertContains(t *testing.T, text, needle string) {
	t.Helper()
	if !strings.Contains(text, needle) {
		t.Fatalf("expected %q to contain %q", text, needle)
	}
}

func assertNotContains(t *testing.T, text, needle string) {
	t.Helper()
	if strings.Contains(text, needle) {
		t.Fatalf("expected %q not to contain %q", text, needle)
	}
}

func assertOrder(t *testing.T, text, first, second string) {
	t.Helper()
	firstIdx := strings.Index(text, first)
	secondIdx := strings.Index(text, second)
	if firstIdx < 0 || secondIdx < 0 || firstIdx > secondIdx {
		t.Fatalf("expected %q before %q in:\n%s", first, second, text)
	}
}
