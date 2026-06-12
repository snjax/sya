package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
)

func TestMutationCommandGoldens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T, root string) (stdout, stderr string, code int)
	}{
		{name: "move_json", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "a00001", "Move Me")
			return runCLI(t, root, nil, nil, []string{"--json", "move", "a00001", "in_progress"})
		}},
		{name: "move_blocked_json", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "f00001", "Feature", "-t", "feature")
			mustRun(t, root, nil, []string{"move", "f00001", "spec"})
			return runCLI(t, root, nil, nil, []string{"--json", "move", "f00001", "impl"})
		}},
		{name: "move_blocked_human", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "f00001", "Feature", "-t", "feature")
			mustRun(t, root, nil, []string{"move", "f00001", "spec"})
			return runCLI(t, root, nil, nil, []string{"move", "f00001", "impl"})
		}},
		{name: "move_offending_json", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "a00001", "Blocked")
			createSeedTask(t, root, "b00001", "Dependency")
			mustRun(t, root, nil, []string{"link", "a00001", "depends_on", "b00001"})
			return runCLI(t, root, nil, nil, []string{"--json", "move", "a00001", "in_progress"})
		}},
		{name: "move_offending_human", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "a00001", "Blocked")
			createSeedTask(t, root, "b00001", "Dependency")
			mustRun(t, root, nil, []string{"link", "a00001", "depends_on", "b00001"})
			return runCLI(t, root, nil, nil, []string{"move", "a00001", "in_progress"})
		}},
		{name: "move_human", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "a00001", "Move Me")
			return runCLI(t, root, nil, nil, []string{"move", "a00001", "in_progress"})
		}},
		{name: "update_json", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "b00001", "Bug", "-t", "bug")
			return runCLI(t, root, nil, nil, []string{"--json", "update", "b00001", "--priority", "critical", "--assignee", "codex", "--field", "severity=critical"})
		}},
		{name: "update_rel_json", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "a00001", "A")
			createSeedTask(t, root, "b00001", "B")
			return runCLI(t, root, nil, nil, []string{"--json", "update", "b00001", "--rel", "blocks=a00001"})
		}},
		{name: "update_human", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "b00001", "Bug", "-t", "bug")
			return runCLI(t, root, nil, nil, []string{"update", "b00001", "--priority", "critical", "--assignee", "codex", "--field", "severity=critical"})
		}},
		{name: "update_rel_human", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "a00001", "A")
			createSeedTask(t, root, "b00001", "B")
			return runCLI(t, root, nil, nil, []string{"update", "b00001", "--rel", "blocks=a00001"})
		}},
		{name: "edit_json", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "f00001", "Feature", "-t", "feature")
			return runCLI(t, root, nil, strings.NewReader("Design text\n"), []string{"--json", "edit", "f00001", "--section", "Design", "--file", "-"})
		}},
		{name: "edit_human", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "f00001", "Feature", "-t", "feature")
			return runCLI(t, root, nil, strings.NewReader("Design text\n"), []string{"edit", "f00001", "--section", "Design", "--file", "-"})
		}},
		{name: "claim_json", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "c00001", "Claim Me")
			return runCLI(t, root, nil, nil, []string{"--json", "--actor", "codex", "claim", "c00001"})
		}},
		{name: "claim_human", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "c00001", "Claim Me")
			return runCLI(t, root, nil, nil, []string{"--actor", "codex", "claim", "c00001"})
		}},
		{name: "claim_not_reachable_json", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "f00001", "Draft Feature", "-t", "feature")
			return runCLI(t, root, nil, nil, []string{"--json", "--actor", "codex", "claim", "f00001"})
		}},
		{name: "claim_not_reachable_human", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "f00001", "Draft Feature", "-t", "feature")
			return runCLI(t, root, nil, nil, []string{"--actor", "codex", "claim", "f00001"})
		}},
		{name: "close_json", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "d00001", "Close Me")
			mustRun(t, root, nil, []string{"move", "d00001", "in_progress"})
			return runCLI(t, root, nil, nil, []string{"--json", "close", "d00001", "--reason", "done"})
		}},
		{name: "close_human", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "d00001", "Close Me")
			mustRun(t, root, nil, []string{"move", "d00001", "in_progress"})
			return runCLI(t, root, nil, nil, []string{"close", "d00001", "--reason", "done"})
		}},
		{name: "close_ambiguous_json", run: func(t *testing.T, root string) (string, string, int) {
			closeAmbiguousFixture(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "close", "f00001"})
		}},
		{name: "close_ambiguous_human", run: func(t *testing.T, root string) (string, string, int) {
			closeAmbiguousFixture(t, root)
			return runCLI(t, root, nil, nil, []string{"close", "f00001"})
		}},
		{name: "reopen_json", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "e00001", "Reopen Me")
			mustRun(t, root, nil, []string{"move", "e00001", "in_progress"})
			mustRun(t, root, nil, []string{"close", "e00001"})
			return runCLI(t, root, nil, nil, []string{"--json", "reopen", "e00001"})
		}},
		{name: "reopen_human", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "e00001", "Reopen Me")
			mustRun(t, root, nil, []string{"move", "e00001", "in_progress"})
			mustRun(t, root, nil, []string{"close", "e00001"})
			return runCLI(t, root, nil, nil, []string{"reopen", "e00001"})
		}},
		{name: "link_json", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "a00001", "A")
			createSeedTask(t, root, "b00001", "B")
			return runCLI(t, root, nil, nil, []string{"--json", "link", "b00001", "blocks", "a00001"})
		}},
		{name: "link_human", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "a00001", "A")
			createSeedTask(t, root, "b00001", "B")
			return runCLI(t, root, nil, nil, []string{"link", "b00001", "blocks", "a00001"})
		}},
		{name: "unlink_json", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "a00001", "A")
			createSeedTask(t, root, "b00001", "B")
			mustRun(t, root, nil, []string{"link", "a00001", "depends_on", "b00001"})
			return runCLI(t, root, nil, nil, []string{"--json", "unlink", "a00001", "depends_on", "b00001"})
		}},
		{name: "unlink_human", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "a00001", "A")
			createSeedTask(t, root, "b00001", "B")
			mustRun(t, root, nil, []string{"link", "a00001", "depends_on", "b00001"})
			return runCLI(t, root, nil, nil, []string{"unlink", "a00001", "depends_on", "b00001"})
		}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			stdout, stderr, code := tt.run(t, root)
			got := normalizeCommandOutput(root, stdout, stderr, code)
			wantBytes, err := os.ReadFile(filepath.Join("testdata", "mutations", tt.name+".golden"))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			want := strings.TrimSpace(string(wantBytes))
			if strings.TrimSpace(got) != want {
				t.Fatalf("golden mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
			}
		})
	}
}

func TestUpdateRejectsParentCycle(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	addRecursiveNodeType(t, root)
	createSeedTask(t, root, "a00001", "A", "-t", "node")
	createSeedTask(t, root, "b00001", "B", "-t", "node", "--parent", "a00001")

	_, stderr, code := runCLI(t, root, nil, nil, []string{"update", "a00001", "--parent", "b00001"})
	if code != syaerr.ExitUsage || !strings.Contains(stderr, "parent cycle") {
		t.Fatalf("stderr=%q code=%d", stderr, code)
	}
}

func TestFreshFeatureBlocksImplOnEmptyDesign(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "f00001", "Feature", "-t", "feature")
	mustRun(t, root, nil, []string{"move", "f00001", "spec"})

	_, stderr, code := runCLI(t, root, nil, nil, []string{"move", "f00001", "impl"})
	if code != syaerr.ExitTransitionRejected || !strings.Contains(stderr, "Секция Design пуста") || !strings.Contains(stderr, ".sya/tasks/f00001-feature.md") {
		t.Fatalf("stderr=%q code=%d", stderr, code)
	}
}

func TestClaimWorkingStatusOnlyAssigns(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "Working Task")
	mustRun(t, root, nil, []string{"move", "a00001", "in_progress"})

	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"--actor", "codex", "claim", "a00001"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "a00001: ok") {
		t.Fatalf("claim stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	parsed, err := parseTaskFile(t, findTaskFile(t, root, "a00001"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Status != "in_progress" || parsed.Assignee != "codex" || !strings.Contains(string(parsed.Body.Raw), "@codex: claimed") {
		t.Fatalf("claim changed wrong fields: status=%s assignee=%s\n%s", parsed.Status, parsed.Assignee, parsed.Body.Raw)
	}
}

func TestClaimWorkingStatusStealsAssignee(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "Working Task")
	mustRun(t, root, nil, []string{"move", "a00001", "in_progress"})
	mustRun(t, root, nil, []string{"update", "a00001", "--assignee", "alice"})

	_, stderr, code := runCLI(t, root, nil, nil, []string{"--actor", "codex", "claim", "a00001"})
	if code != syaerr.ExitTransitionRejected || !strings.Contains(stderr, "already claimed by alice") {
		t.Fatalf("claim occupied stderr=%q code=%d", stderr, code)
	}
	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"--actor", "codex", "claim", "a00001", "--steal"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "a00001: ok") {
		t.Fatalf("steal stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	parsed, err := parseTaskFile(t, findTaskFile(t, root, "a00001"))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Assignee != "codex" || !strings.Contains(string(parsed.Body.Raw), "claim stolen from alice") {
		t.Fatalf("steal did not update assignee/log: %#v\n%s", parsed.Assignee, parsed.Body.Raw)
	}
}

func TestUnlinkMissingRelationIsNoop(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "A")
	createSeedTask(t, root, "b00001", "B")
	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"unlink", "a00001", "depends_on", "b00001"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "noop a00001 depends_on b00001") {
		t.Fatalf("unlink stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}

func TestUpdateRejectsDuplicateFieldFlags(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "b00001", "Bug", "-t", "bug")

	_, stderr, code := runCLI(t, root, nil, nil, []string{"update", "b00001", "--field", "severity=critical", "--field", "severity=minor"})
	if code != syaerr.ExitUsage || !strings.Contains(stderr, "duplicate field flag: severity") {
		t.Fatalf("stderr=%q code=%d", stderr, code)
	}
}

func TestUpdateRelIsRepeatable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "A")
	createSeedTask(t, root, "b00001", "B")
	createSeedTask(t, root, "c00001", "C")

	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"update", "a00001", "--rel", "depends_on=b00001", "--rel", "depends_on=c00001"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("update stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, root, nil, nil, []string{"show", "a00001"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "depends_on: b00001, c00001") {
		t.Fatalf("show stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}

func TestUpdateFieldIsRepeatable(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	addRecordType(t, root)
	createSeedTask(t, root, "r00001", "Record", "-t", "record")

	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"update", "r00001", "--field", "ready=true", "--field", "size=large"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("update stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, root, nil, nil, []string{"--json", "show", "r00001"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, `"fields":{"ready":true,"size":"large"}`) {
		t.Fatalf("show stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}

func TestShowDeduplicatesSymmetricRelationView(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "A")
	createSeedTask(t, root, "b00001", "B")
	mustRun(t, root, nil, []string{"link", "a00001", "relates", "b00001"})

	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"show", "a00001"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	if got := strings.Count(stdout, "relates: b00001"); got != 1 {
		t.Fatalf("relates rendered %d times, want 1\n%s", got, stdout)
	}
	if strings.Contains(stdout, "relates: b00001, b00001") {
		t.Fatalf("duplicate relation rendered:\n%s", stdout)
	}
}

func TestMutationWriteRejectsSchemaConformanceViolation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "Bad Section")
	path := findTaskFile(t, root, "a00001")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n## Surprise\nnot declared\n"); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"--json", "comment", "a00001", "-m", "hello"})
	if code != syaerr.ExitSchemaInvalid || !strings.Contains(stdout, `"type":"schema_conformance"`) || !strings.Contains(stdout, `"section_unknown"`) {
		t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}

func TestMutationWriteBumpsSchemaVersion(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	setFixtureSchemaVersion(t, root, 2)
	createSeedTask(t, root, "a00001", "Old Version")
	path := findTaskFile(t, root, "a00001")
	replaceInFile(t, path, "schema_version: 2\n", "schema_version: 1\n")

	_, stderr, code := runCLI(t, root, nil, nil, []string{"comment", "a00001", "-m", "bump"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("stderr=%q code=%d", stderr, code)
	}
	parsed := parseTaskFileForTest(t, path)
	if parsed.SchemaVersion != 2 {
		t.Fatalf("schema_version = %d, want 2", parsed.SchemaVersion)
	}
}

func TestSchemaMigratePreflightPreventsPartialWrites(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "Good")
	bad := &task.Task{ID: "b00001", Type: "ghost", Title: "Bad", Status: "todo", SchemaVersion: 1, Body: task.NewBody([]byte("## Description\nbad\n"), nil)}
	out, err := task.Serialize(bad)
	if err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(root, ".sya", "tasks", "b00001-bad.md")
	if err := os.WriteFile(badPath, out, 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, root, nil, nil, []string{"schema", "migrate", "--rename-status", "todo=in_progress"})
	if code != syaerr.ExitSchemaInvalid || !strings.Contains(stderr, "unknown task type: ghost") {
		t.Fatalf("stderr=%q code=%d", stderr, code)
	}
	parsed := parseTaskFileForTest(t, findTaskFile(t, root, "a00001"))
	if parsed.Status != "todo" {
		t.Fatalf("valid task was partially migrated to %q", parsed.Status)
	}
}

func addRecordType(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, ".sya", "schema.yml")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open schema: %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Fatalf("close schema: %v", err)
		}
	}()
	if _, err := f.WriteString(`
  record:
    pipeline: [open, done]
    terminal: [done]
    sections: [Description]
    fields:
      ready: {type: bool}
      size: {type: enum, values: [small, large]}
    transitions:
      open -> done: {}
`); err != nil {
		t.Fatalf("append schema type: %v", err)
	}
}

func addRecursiveNodeType(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, ".sya", "schema.yml")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open schema: %v", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Fatalf("close schema: %v", err)
		}
	}()
	if _, err := f.WriteString(`
  node:
    container: true
    children: [node]
    pipeline: [open, done]
    terminal: [done]
    sections: [Description]
    transitions:
      open -> done: {}
`); err != nil {
		t.Fatalf("append schema type: %v", err)
	}
}

func replaceInFile(t *testing.T, path, old, new string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, old) {
		t.Fatalf("%s does not contain %q", path, old)
	}
	text = strings.Replace(text, old, new, 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func parseTaskFileForTest(t *testing.T, path string) *task.Task {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := task.ParseBytes(data)
	if err != nil {
		t.Fatalf("parse task: %v\n%s", err, data)
	}
	return parsed
}

func TestMutationErrorPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(t *testing.T, root string)
		args     []string
		stdin    *strings.Reader
		wantCode int
		wantText string
	}{
		{
			name: "move guard violation partial",
			setup: func(t *testing.T, root string) {
				initProject(t, root)
				createSeedTask(t, root, "f00001", "Feature", "-t", "feature")
				mustRun(t, root, nil, []string{"move", "f00001", "spec"})
			},
			args:     []string{"--json", "move", "f00001", "impl"},
			wantCode: syaerr.ExitTransitionRejected,
			wantText: "transition_blocked",
		},
		{
			name: "move guard violation human has details",
			setup: func(t *testing.T, root string) {
				initProject(t, root)
				createSeedTask(t, root, "f00001", "Feature", "-t", "feature")
				mustRun(t, root, nil, []string{"move", "f00001", "spec"})
			},
			args:     []string{"move", "f00001", "impl"},
			wantCode: syaerr.ExitTransitionRejected,
			wantText: "hint: После ревью спеки",
		},
		{
			name: "move unknown status human lists allowed",
			setup: func(t *testing.T, root string) {
				initProject(t, root)
				createSeedTask(t, root, "f00001", "Feature", "-t", "feature")
			},
			args:     []string{"move", "f00001", "working"},
			wantCode: syaerr.ExitTransitionRejected,
			wantText: `allowed: spec (advance - Требования созрели`,
		},
		{
			name: "move unknown status json allowed targets",
			setup: func(t *testing.T, root string) {
				initProject(t, root)
				createSeedTask(t, root, "f00001", "Feature", "-t", "feature")
			},
			args:     []string{"--json", "move", "f00001", "working"},
			wantCode: syaerr.ExitTransitionRejected,
			wantText: `"allowed":[{"to":"spec","kind":"advance","description":"Требования созрели — пишем спецификацию"},{"to":"scrapped","kind":"setback","terminal":true}]`,
		},
		{
			name: "move offending includes task detail",
			setup: func(t *testing.T, root string) {
				initProject(t, root)
				createSeedTask(t, root, "a00001", "Blocked")
				createSeedTask(t, root, "b00001", "Dependency")
				mustRun(t, root, nil, []string{"link", "a00001", "depends_on", "b00001"})
			},
			args:     []string{"--json", "move", "a00001", "in_progress"},
			wantCode: syaerr.ExitTransitionRejected,
			wantText: `"title":"Dependency"`,
		},
		{
			name: "ambiguous prefix",
			setup: func(t *testing.T, root string) {
				initProject(t, root)
				createSeedTask(t, root, "abc111", "One")
				createSeedTask(t, root, "abc222", "Two")
			},
			args:     []string{"move", "abc", "in_progress"},
			wantCode: syaerr.ExitLookup,
			wantText: "ambiguous prefix",
		},
		{
			name: "not found",
			setup: func(t *testing.T, root string) {
				initProject(t, root)
			},
			args:     []string{"move", "missing", "in_progress"},
			wantCode: syaerr.ExitLookup,
			wantText: "not found",
		},
		{
			name: "already claimed",
			setup: func(t *testing.T, root string) {
				initProject(t, root)
				createSeedTask(t, root, "c00001", "Claimed")
				mustRun(t, root, nil, []string{"update", "c00001", "--assignee", "alice"})
			},
			args:     []string{"--json", "--actor", "bob", "claim", "c00001"},
			wantCode: syaerr.ExitTransitionRejected,
			wantText: "already_claimed",
		},
		{
			name: "close ambiguous from working feature",
			setup: func(t *testing.T, root string) {
				closeAmbiguousFixture(t, root)
			},
			args:     []string{"close", "f00001"},
			wantCode: syaerr.ExitTransitionRejected,
			wantText: "sya close f00001 --to scrapped",
		},
		{
			name: "claim feature draft not reachable",
			setup: func(t *testing.T, root string) {
				initProject(t, root)
				createSeedTask(t, root, "f00001", "Draft Feature", "-t", "feature")
			},
			args:     []string{"claim", "f00001"},
			wantCode: syaerr.ExitTransitionRejected,
			wantText: "cannot claim: working statuses for feature are impl, review; no transition from draft; advance first: sya move f00001 spec",
		},
		{
			name: "update rel unknown relation",
			setup: func(t *testing.T, root string) {
				initProject(t, root)
				createSeedTask(t, root, "a00001", "A")
				createSeedTask(t, root, "b00001", "B")
			},
			args:     []string{"update", "a00001", "--rel", "ghost=b00001"},
			wantCode: syaerr.ExitUsage,
			wantText: "unknown relation: ghost",
		},
		{
			name: "update rel bad target",
			setup: func(t *testing.T, root string) {
				initProject(t, root)
				createSeedTask(t, root, "a00001", "A")
			},
			args:     []string{"update", "a00001", "--rel", "depends_on=missing"},
			wantCode: syaerr.ExitLookup,
			wantText: "not found: missing",
		},
		{
			name: "undeclared field",
			setup: func(t *testing.T, root string) {
				initProject(t, root)
				createSeedTask(t, root, "d00001", "Task")
			},
			args:     []string{"update", "d00001", "--field", "ghost=true"},
			wantCode: syaerr.ExitUsage,
			wantText: "field is not declared",
		},
		{
			name: "unknown section",
			setup: func(t *testing.T, root string) {
				initProject(t, root)
				createSeedTask(t, root, "e00001", "Task")
			},
			args:     []string{"edit", "e00001", "--section", "Design", "--file", "-"},
			stdin:    strings.NewReader("text\n"),
			wantCode: syaerr.ExitUsage,
			wantText: "section is not declared",
		},
		{
			name: "link cycle",
			setup: func(t *testing.T, root string) {
				initProject(t, root)
				createSeedTask(t, root, "f00001", "F")
				createSeedTask(t, root, "g00001", "G")
				mustRun(t, root, nil, []string{"link", "f00001", "depends_on", "g00001"})
			},
			args:     []string{"link", "g00001", "depends_on", "f00001"},
			wantCode: syaerr.ExitUsage,
			wantText: "cycle",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			tt.setup(t, root)
			stdout, stderr, code := runCLI(t, root, nil, tt.stdin, tt.args)
			if code != tt.wantCode {
				t.Fatalf("code=%d want=%d stdout=%q stderr=%q", code, tt.wantCode, stdout, stderr)
			}
			if !strings.Contains(stdout+stderr, tt.wantText) {
				t.Fatalf("output does not contain %q\nstdout=%q\nstderr=%q", tt.wantText, stdout, stderr)
			}
		})
	}
}

func TestUpdateRelCanonicalizesLikeLink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "A")
	createSeedTask(t, root, "b00001", "B")
	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"update", "b00001", "--rel", "blocks=a00001"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("update --rel stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, root, nil, nil, []string{"show", "a00001"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("show stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	if !strings.Contains(stdout, "depends_on: b00001") {
		t.Fatalf("canonical relation missing from source task:\n%s", stdout)
	}
}

func closeAmbiguousFixture(t *testing.T, root string) {
	t.Helper()
	initProject(t, root)
	createSeedTask(t, root, "f00001", "Feature", "-t", "feature")
	mustRun(t, root, nil, []string{"move", "f00001", "spec"})
	mustRun(t, root, nil, []string{"update", "f00001", "--field", "spec_approved=true"})
	mustRun(t, root, strings.NewReader("Design text\n"), []string{"edit", "f00001", "--section", "Design", "--file", "-"})
	mustRun(t, root, nil, []string{"move", "f00001", "impl"})
}

func mustRun(t *testing.T, root string, stdin *strings.Reader, args []string) {
	t.Helper()
	stdout, stderr, code := runCLI(t, root, nil, stdin, args)
	if code != syaerr.ExitOK {
		t.Fatalf("command %v failed code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
	}
}
