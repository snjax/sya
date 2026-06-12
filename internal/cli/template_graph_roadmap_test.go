package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snjax/sya/internal/syaerr"
)

func TestTemplateApplyE2E(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	templateFixtureProject(t, root)

	stdout, stderr, code := runCLI(t, root, []string{"a00001", "b00001", "c00001"}, nil, []string{"template", "apply", "feature-set", "-p", "name=Streaming", "--parent", "e00001"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("template apply stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	for _, want := range []string{"created spec -> a00001", "created impl -> b00001", "created follow -> c00001"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("apply output missing %q:\n%s", want, stdout)
		}
	}
	show, stderr, code := runCLI(t, root, nil, nil, []string{"show", "b00001"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("show stdout=%q stderr=%q code=%d", show, stderr, code)
	}
	for _, want := range []string{"parent: e00001", "depends_on: a00001, d00001", "created from template feature-set"} {
		if !strings.Contains(show, want) {
			t.Fatalf("created task missing %q:\n%s", want, show)
		}
	}
}

func TestTemplateDryRunGolden(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	templateFixtureProject(t, root)
	stdout, stderr, code := runCLI(t, root, []string{"a00001", "b00001", "c00001"}, nil, []string{"template", "apply", "feature-set", "-p", "name=Streaming", "--parent", "e00001", "--dry-run"})
	assertGolden(t, root, "testdata/template/dry_run_human.golden", stdout, stderr, code)
	if _, err := os.Stat(filepath.Join(root, ".sya", "tasks", "a00001-spec-streaming.md")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote task file, err=%v", err)
	}
}

func TestTemplatePreflightErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "unknown type",
			body: `
name: bad
tasks:
  - key: a
    type: ghost
    title: Bad
`,
			want: "unknown task type: ghost",
		},
		{
			name: "unknown relation",
			body: `
name: bad
tasks:
  - key: a
    title: Bad
    relations:
      ghost: [d00001]
`,
			want: "unknown relation: ghost",
		},
		{
			name: "unknown field",
			body: `
name: bad
tasks:
  - key: a
    title: Bad
    fields:
      ghost: true
`,
			want: "field is not declared for type: ghost",
		},
		{
			name: "missing target",
			body: `
name: bad
tasks:
  - key: a
    title: Bad
    relations:
      depends_on: [missing]
`,
			want: "not found: missing",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			initProject(t, root)
			createSeedTask(t, root, "d00001", "Dependency")
			writeTemplate(t, root, "bad", tt.body)
			_, stderr, code := runCLI(t, root, []string{"a00001"}, nil, []string{"template", "apply", "bad"})
			if code == syaerr.ExitOK || !strings.Contains(stderr, tt.want) {
				t.Fatalf("stderr=%q code=%d, want %q", stderr, code, tt.want)
			}
			if _, err := os.Stat(filepath.Join(root, ".sya", "tasks", "a00001-bad.md")); !os.IsNotExist(err) {
				t.Fatalf("preflight wrote task file, err=%v", err)
			}
		})
	}
}

func TestGraphGolden(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	graphRoadmapFixture(t, root)
	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"graph", "--all-relations"})
	assertGolden(t, root, "testdata/graph/mermaid_human.golden", stdout, stderr, code)
}

func TestRoadmapGolden(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	graphRoadmapFixture(t, root)
	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"roadmap"})
	assertGolden(t, root, "testdata/roadmap/human.golden", stdout, stderr, code)
}

func TestRoadmapWritesFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	graphRoadmapFixture(t, root)
	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"roadmap", "-o", "ROADMAP.md"})
	if code != syaerr.ExitOK || stderr != "" || strings.TrimSpace(stdout) != "wrote ROADMAP.md" {
		t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	data, err := os.ReadFile(filepath.Join(root, "ROADMAP.md"))
	if err != nil {
		t.Fatalf("read roadmap: %v", err)
	}
	if !strings.Contains(string(data), "# ") || !strings.Contains(string(data), "Platform Epic") {
		t.Fatalf("unexpected roadmap file:\n%s", data)
	}
}

func templateFixtureProject(t *testing.T, root string) {
	t.Helper()
	initProject(t, root)
	createSeedTask(t, root, "e00001", "Platform Epic", "-t", "epic")
	createSeedTask(t, root, "d00001", "Dependency")
	writeTemplate(t, root, "feature-set", `
name: feature-set
description: Create a feature with spec and follow-up.
params:
  - name: name
    required: true
tasks:
  - key: spec
    type: docs
    title: "Spec {{name}}"
    sections:
      Description: "Spec for {{name}}"
  - key: impl
    type: feature
    title: "Build {{name}}"
    priority: high
    fields:
      spec_approved: true
    sections:
      Description: "Feature {{name}}"
      Design: "Approved design"
      Acceptance: "- [ ] works"
    relations:
      depends_on: [spec, d00001]
  - key: follow
    title: "Follow {{name}}"
    relations:
      discovered_from: [impl]
`)
}

func graphRoadmapFixture(t *testing.T, root string) {
	t.Helper()
	initProject(t, root)
	setProjectName(t, root, "Roadmap")
	createSeedTask(t, root, "e00001", "Platform Epic", "-t", "epic", "-d", "Roadmap epic description\nsecond line")
	createSeedTask(t, root, "a00001", "Design Alpha", "--parent", "e00001")
	createSeedTask(t, root, "b00001", "Build Beta", "--parent", "e00001")
	mustRun(t, root, nil, []string{"move", "b00001", "in_progress"})
	mustRun(t, root, nil, []string{"--actor", "codex", "claim", "b00001"})
	createSeedTask(t, root, "x00001", "Blocked Work", "--parent", "e00001", "--depends-on", "a00001")
	createSeedTask(t, root, "d00001", "Top Docs", "-t", "docs")
	createSeedTask(t, root, "c00001", "Archived Task")
	mustRun(t, root, nil, []string{"move", "c00001", "in_progress"})
	mustRun(t, root, nil, []string{"close", "c00001"})
	mustRun(t, root, nil, []string{"archive", "c00001"})
}

func setProjectName(t *testing.T, root, name string) {
	t.Helper()
	path := filepath.Join(root, ".sya", "config.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "project: ") {
			lines[i] = "project: " + name
			break
		}
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func writeTemplate(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, ".sya", "templates")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".yml"), []byte(strings.TrimSpace(body)+"\n"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
}

func assertGolden(t *testing.T, root, path, stdout, stderr string, code int) {
	t.Helper()
	got := normalizeCommandOutput(root, stdout, stderr, code)
	wantBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v\nactual:\n%s", err, got)
	}
	want := strings.TrimSpace(string(wantBytes))
	if strings.TrimSpace(got) != want {
		t.Fatalf("golden mismatch\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}
