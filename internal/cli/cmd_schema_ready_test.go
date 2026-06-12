package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snjax/sya/internal/syaerr"
)

func TestSchemaGraphDocsGoldens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		args   []string
		golden string
	}{
		{name: "task graph", args: []string{"schema", "graph", "--type", "task"}, golden: "task_graph.golden"},
		{name: "task docs", args: []string{"schema", "docs", "--type", "task"}, golden: "task_docs.golden"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			initProject(t, root)
			stdout, stderr, code := runCLI(t, root, nil, nil, tt.args)
			if code != syaerr.ExitOK || stderr != "" {
				t.Fatalf("stderr=%q code=%d", stderr, code)
			}
			wantBytes, err := os.ReadFile(filepath.Join("testdata", "schema", tt.golden))
			if err != nil {
				t.Fatalf("read golden: %v", err)
			}
			if strings.TrimSpace(stdout) != strings.TrimSpace(string(wantBytes)) {
				t.Fatalf("golden mismatch\nwant:\n%s\n\ngot:\n%s", strings.TrimSpace(string(wantBytes)), strings.TrimSpace(stdout))
			}
		})
	}
}

func TestSchemaValidateDiagnostics(t *testing.T) {
	t.Parallel()

	t.Run("violations exit 4", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		writeSchemaFile(t, root, `
schema_version: 1
defaults: {type: task}
types:
  task:
    pipeline: [todo, done]
    transitions:
      todo -> done: {}
`)
		stdout, stderr, code := runCLI(t, root, nil, nil, []string{"--json", "schema", "validate"})
		if code != syaerr.ExitSchemaInvalid || stderr != "" {
			t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
		}
		if !strings.Contains(stdout, `"type":"schema_invalid"`) || !strings.Contains(stdout, "terminal_required") {
			t.Fatalf("stdout = %s, want schema_invalid terminal_required", stdout)
		}
	})

	t.Run("human violations are listed", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		writeSchemaFile(t, root, `
schema_version: 1
defaults: {type: task}
types:
  task:
    pipeline: [todo, done]
    transitions:
      todo -> done: {}
`)
		stdout, stderr, code := runCLI(t, root, nil, nil, []string{"schema", "validate"})
		if code != syaerr.ExitSchemaInvalid || stdout != "" {
			t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
		}
		if !strings.Contains(stderr, "schema invalid") || !strings.Contains(stderr, "[terminal_required]") {
			t.Fatalf("stderr = %q, want schema violation details", stderr)
		}
	})

	t.Run("warnings exit 0", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		writeSchemaFile(t, root, `
schema_version: 1
defaults: {type: task}
relations:
  relates: {}
types:
  task:
    pipeline: [todo, done]
    terminal: [done]
    transitions:
      todo -> done:
        guards:
          - kind: relation_status
            relation: relates
            in: [ghost]
            message: ghost status
`)
		stdout, stderr, code := runCLI(t, root, nil, nil, []string{"--json", "schema", "validate"})
		if code != syaerr.ExitOK || stderr != "" {
			t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
		}
		if !strings.Contains(stdout, `"valid":true`) || !strings.Contains(stdout, "guard_status_unknown") {
			t.Fatalf("stdout = %s, want valid warning", stdout)
		}
	})
}

func TestReadyBlockedCommandsDependencyChain(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "b00001", "Dependency Task")
	createSeedTask(t, root, "a00001", "Blocked Task", "--depends-on", "b00001")

	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"ready"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("ready stderr=%q code=%d", stderr, code)
	}
	if !strings.Contains(stdout, "b00001") || strings.Contains(stdout, "a00001") {
		t.Fatalf("ready stdout=%q, want only dependency task ready", stdout)
	}

	stdout, stderr, code = runCLI(t, root, nil, nil, []string{"blocked"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("blocked stderr=%q code=%d", stderr, code)
	}
	if !strings.Contains(stdout, "a00001") || !strings.Contains(stdout, "blocking_relation") || !strings.Contains(stdout, "b00001") {
		t.Fatalf("blocked stdout=%q, want blocked task with dependency reason", stdout)
	}

	stdout, stderr, code = runCLI(t, root, nil, nil, []string{"blocked", "--limit", "1"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("blocked --limit stderr=%q code=%d", stderr, code)
	}
	if strings.Count(stdout, "\n\n") > 0 {
		t.Fatalf("blocked --limit stdout=%q, want one task", stdout)
	}
}

func writeSchemaFile(t *testing.T, root, contents string) {
	t.Helper()
	path := filepath.Join(root, ".sya", "schema.yml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
}
