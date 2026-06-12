package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/snjax/sya/internal/fsutil"
	"github.com/snjax/sya/internal/syaerr"
)

func TestDefaultSchemaMatchesReference(t *testing.T) {
	t.Parallel()

	reference, err := os.ReadFile(filepath.Join("..", "schema", "testdata", "reference.yml"))
	if err != nil {
		t.Fatalf("read reference schema: %v", err)
	}
	if string(DefaultSchemaBytes()) != string(reference) {
		t.Fatalf("embedded default schema differs from internal/schema/testdata/reference.yml")
	}
}

func TestInitCreatesSearchIgnoreFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"init"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("init stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	if info, err := os.Stat(filepath.Join(root, ".sya", "templates")); err != nil || !info.IsDir() {
		t.Fatalf("templates dir missing or not a dir: info=%v err=%v", info, err)
	}
	for _, name := range fsutil.SearchIgnoreFiles {
		path := filepath.Join(root, ".sya", name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if string(data) != fsutil.SearchIgnoreContent {
			t.Fatalf("%s content = %q, want %q", name, data, fsutil.SearchIgnoreContent)
		}
	}
}

func TestInitNoAgentsMD(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"init", "--no-agents-md"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("init stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md should not be created with --no-agents-md, err=%v", err)
	}
	if strings.Contains(stdout, "Agent docs:") {
		t.Fatalf("init output should not list agent docs with --no-agents-md:\n%s", stdout)
	}
}

func TestSlugify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		title string
		want  string
	}{
		{title: "Стриминг", want: "striming"},
		{title: "API Стриминг v2", want: "api-striming-v2"},
		{title: "🚀✨", want: ""},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.title, func(t *testing.T) {
			t.Parallel()
			if got := slugify(tt.title); got != tt.want {
				t.Fatalf("slugify(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestCreateSlugFilenames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		want  string
	}{
		{name: "russian", title: "Стриминг", want: ".sya/tasks/r00001-striming.md"},
		{name: "mixed", title: "API Стриминг v2", want: ".sya/tasks/m00001-api-striming-v2.md"},
		{name: "emoji fallback", title: "🚀✨", want: ".sya/tasks/e00001.md"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			initProject(t, root)
			id := strings.Split(tt.want, "/")[2]
			id = strings.TrimSuffix(strings.Split(id, "-")[0], ".md")
			stdout, stderr, code := runCLI(t, root, []string{id}, nil, []string{"--json", "create", tt.title})
			if code != syaerr.ExitOK {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
			}
			if !strings.Contains(stdout, `"file":"`+tt.want+`"`) {
				t.Fatalf("stdout=%q does not contain file %q", stdout, tt.want)
			}
		})
	}
}

func TestCommandGoldens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		json bool
		run  func(t *testing.T, root string) (stdout, stderr string, code int)
	}{
		{name: "init_human", run: func(t *testing.T, root string) (string, string, int) {
			return runCLI(t, root, nil, nil, []string{"init", "--prefix", "acme"})
		}},
		{name: "init_json", json: true, run: func(t *testing.T, root string) (string, string, int) {
			return runCLI(t, root, nil, nil, []string{"--json", "init", "--prefix", "acme"})
		}},
		{name: "create_human", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "e00001", "Create epic", "-t", "epic")
			return runCLI(t, root, []string{"f00001"}, nil, []string{"create", "Build API", "-t", "feature", "-p", "high", "--parent", "e000", "--depends-on", "e00001", "--discovered-from", "e00001", "--field", "spec_approved=true", "-d", "Feature description"})
		}},
		{name: "create_json", json: true, run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "e00001", "Create epic", "-t", "epic")
			return runCLI(t, root, []string{"f00001"}, nil, []string{"--json", "create", "Build API", "-t", "feature", "-p", "high", "--parent", "e000", "--depends-on", "e00001", "--discovered-from", "e00001", "--field", "spec_approved=true", "-d", "Feature description"})
		}},
		{name: "show_human", run: func(t *testing.T, root string) (string, string, int) {
			fixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"show", "f000"})
		}},
		{name: "show_json", json: true, run: func(t *testing.T, root string) (string, string, int) {
			fixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "show", "f000"})
		}},
		{name: "list_human", run: func(t *testing.T, root string) (string, string, int) {
			fixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"list", "-t", "feature", "--limit", "1"})
		}},
		{name: "list_json", json: true, run: func(t *testing.T, root string) (string, string, int) {
			fixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "list", "-t", "feature", "--limit", "1"})
		}},
		{name: "ready_human", run: func(t *testing.T, root string) (string, string, int) {
			fixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"ready"})
		}},
		{name: "ready_json", json: true, run: func(t *testing.T, root string) (string, string, int) {
			fixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "ready"})
		}},
		{name: "transitions_human", run: func(t *testing.T, root string) (string, string, int) {
			fixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"transitions", "f000"})
		}},
		{name: "transitions_json", json: true, run: func(t *testing.T, root string) (string, string, int) {
			fixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "transitions", "f000"})
		}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			stdout, stderr, code := tt.run(t, root)
			got := normalizeCommandOutput(root, stdout, stderr, code)
			wantBytes, err := os.ReadFile(filepath.Join("testdata", "commands", tt.name+".golden"))
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

func TestCreateFlagsAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("description file", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		desc := filepath.Join(root, "desc.md")
		if err := os.WriteFile(desc, []byte("From file\n"), 0o644); err != nil {
			t.Fatalf("write desc: %v", err)
		}
		stdout, stderr, code := runCLI(t, root, []string{"a00001"}, nil, []string{"create", "From File", "--description-file", desc})
		if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "created a00001") {
			t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
		}
	})
	t.Run("sectionless type doctor clean", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		addSectionlessNoteType(t, root)

		stdout, stderr, code := runCLI(t, root, []string{"n00001"}, nil, []string{"create", "No Sections", "-t", "note"})
		if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "created n00001") {
			t.Fatalf("create stdout=%q stderr=%q code=%d", stdout, stderr, code)
		}
		data, err := os.ReadFile(filepath.Join(root, ".sya", "tasks", "n00001-no-sections.md"))
		if err != nil {
			t.Fatalf("read created task: %v", err)
		}
		if strings.Contains(string(data), "## Description") {
			t.Fatalf("sectionless task contains Description section:\n%s", data)
		}
		if !strings.Contains(string(data), "## Log\n- 2026-01-02T03:04:05Z @unknown: created\n") {
			t.Fatalf("sectionless task missing creation Log:\n%s", data)
		}

		stdout, stderr, code = runCLI(t, root, nil, nil, []string{"doctor"})
		if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "doctor: clean") {
			t.Fatalf("doctor stdout=%q stderr=%q code=%d", stdout, stderr, code)
		}
	})
	t.Run("description rejected for sectionless type", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		addSectionlessNoteType(t, root)

		_, stderr, code := runCLI(t, root, []string{"n00001"}, nil, []string{"create", "Bad", "-t", "note", "-d", ""})
		if code != syaerr.ExitUsage || !strings.Contains(stderr, `type "note" does not declare section "Description"`) {
			t.Fatalf("stderr=%q code=%d", stderr, code)
		}
	})
	t.Run("description file rejected for sectionless type", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		addSectionlessNoteType(t, root)
		desc := filepath.Join(root, "desc.md")
		if err := os.WriteFile(desc, nil, 0o644); err != nil {
			t.Fatalf("write desc: %v", err)
		}

		_, stderr, code := runCLI(t, root, []string{"n00001"}, nil, []string{"create", "Bad", "-t", "note", "--description-file", desc})
		if code != syaerr.ExitUsage || !strings.Contains(stderr, `type "note" does not declare section "Description"`) {
			t.Fatalf("stderr=%q code=%d", stderr, code)
		}
	})
	t.Run("batch from stdin", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		batch := "- title: Batch One\n  type: bug\n  priority: low\n  fields:\n    severity: critical\n"
		stdout, stderr, code := runCLI(t, root, []string{"b00001"}, strings.NewReader(batch), []string{"create", "--from-file", "-"})
		if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "created b00001") {
			t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
		}
	})
	t.Run("field validation matches schema", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)

		stdout, stderr, code := runCLI(t, root, []string{"b00001"}, nil, []string{"create", "Bug", "-t", "bug", "--field", "severity=critical"})
		if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "created b00001") {
			t.Fatalf("valid field stdout=%q stderr=%q code=%d", stdout, stderr, code)
		}
		_, stderr, code = runCLI(t, root, []string{"x00001"}, nil, []string{"create", "Bad", "-t", "bug", "--field", "severity=urgent"})
		if code != syaerr.ExitUsage || !strings.Contains(stderr, "field value not in enum: urgent") {
			t.Fatalf("bad enum stderr=%q code=%d", stderr, code)
		}
		_, stderr, code = runCLI(t, root, []string{"x00001"}, nil, []string{"create", "Bad", "--field", "estimate=3"})
		if code != syaerr.ExitUsage || !strings.Contains(stderr, "field is not declared for type: estimate") {
			t.Fatalf("unknown field stderr=%q code=%d", stderr, code)
		}
	})
	t.Run("duplicate field flags rejected", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)

		_, stderr, code := runCLI(t, root, []string{"x00001"}, nil, []string{"create", "Bad", "-t", "bug", "--field", "severity=critical", "--field", "severity=minor"})
		if code != syaerr.ExitUsage || !strings.Contains(stderr, "duplicate field flag: severity") {
			t.Fatalf("create stderr=%q code=%d", stderr, code)
		}
	})
	t.Run("id length from config", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		config := filepath.Join(root, ".sya", "config.yml")
		data, err := os.ReadFile(config)
		if err != nil {
			t.Fatalf("read config: %v", err)
		}
		data = []byte(strings.Replace(string(data), "id_length: 6", "id_length: 8", 1))
		if err := os.WriteFile(config, data, 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		var gotLength int
		stdout, stderr, code := runCLIWithNewID(t, root, func(existing map[string]struct{}, length int) (string, error) {
			gotLength = length
			return "abcd1234", nil
		}, []string{"create", "Configured ID"})
		if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "created abcd1234") {
			t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
		}
		if gotLength != 8 {
			t.Fatalf("NewID length = %d, want 8", gotLength)
		}
	})
	t.Run("create rejects self parent", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		createSeedTask(t, root, "e00001", "Epic", "-t", "epic")

		_, stderr, code := runCLI(t, root, []string{"e00001"}, nil, []string{"create", "Child", "--parent", "e00001"})
		if code != syaerr.ExitUsage || !strings.Contains(stderr, "parent cycle") {
			t.Fatalf("stderr=%q code=%d", stderr, code)
		}
	})
	t.Run("batch updates in-memory index", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		batch := "- title: Batch One\n  type: task\n- title: Batch Two\n  type: task\n  relations:\n    depends_on: [c00001]\n"
		stdout, stderr, code := runCLI(t, root, []string{"c00001", "d00001"}, strings.NewReader(batch), []string{"create", "--from-file", "-"})
		if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "created c00001") || !strings.Contains(stdout, "created d00001") {
			t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
		}
		stdout, stderr, code = runCLI(t, root, nil, nil, []string{"show", "d00001"})
		if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "depends_on: c00001") {
			t.Fatalf("show stdout=%q stderr=%q code=%d", stdout, stderr, code)
		}
	})
	t.Run("bad type", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		_, stderr, code := runCLI(t, root, []string{"x00001"}, nil, []string{"create", "Bad", "-t", "ghost"})
		if code != syaerr.ExitUsage || !strings.Contains(stderr, "unknown task type") {
			t.Fatalf("stderr=%q code=%d", stderr, code)
		}
	})
	t.Run("bad parent", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		createSeedTask(t, root, "p00001", "Parent task")
		_, stderr, code := runCLI(t, root, []string{"x00001"}, nil, []string{"create", "Child", "--parent", "p000"})
		if code != syaerr.ExitUsage || !strings.Contains(stderr, "parent is not a container") {
			t.Fatalf("stderr=%q code=%d", stderr, code)
		}
	})
	t.Run("unknown relation", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		createSeedTask(t, root, "a00001", "Target")
		_, stderr, code := runCLI(t, root, []string{"x00001"}, nil, []string{"create", "Bad relation", "--rel", "ghost=a00001"})
		if code != syaerr.ExitUsage || !strings.Contains(stderr, "unknown relation") {
			t.Fatalf("stderr=%q code=%d", stderr, code)
		}
	})
	t.Run("ambiguous prefix", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		createSeedTask(t, root, "abc111", "One")
		createSeedTask(t, root, "abc222", "Two")
		_, stderr, code := runCLI(t, root, []string{"x00001"}, nil, []string{"create", "Ambiguous", "--depends-on", "abc"})
		if code != syaerr.ExitLookup || !strings.Contains(stderr, "ambiguous prefix") || !strings.Contains(stderr, "abc111 (One, todo)") {
			t.Fatalf("stderr=%q code=%d", stderr, code)
		}
	})
	t.Run("duplicate rel", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		createSeedTask(t, root, "a00001", "Target")
		_, stderr, code := runCLI(t, root, []string{"x00001"}, nil, []string{"create", "Dup", "--rel", "depends_on=a00001", "--depends-on", "a00001"})
		if code != syaerr.ExitUsage || !strings.Contains(stderr, "duplicate relation flag") {
			t.Fatalf("stderr=%q code=%d", stderr, code)
		}
	})
	t.Run("full reference form resolves", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		createSeedTask(t, root, "a00001", "Ref Task")
		stdout, stderr, code := runCLI(t, root, nil, nil, []string{"show", "sya-a00001"})
		if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "a00001 [todo] Ref Task") {
			t.Fatalf("show full ref stdout=%q stderr=%q code=%d", stdout, stderr, code)
		}
	})
	t.Run("json cobra usage envelope", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		stdout, stderr, code := runCLI(t, root, nil, nil, []string{"--json", "show", "--bad-flag"})
		if code != syaerr.ExitUsage || stderr != "" || !strings.Contains(stdout, `"ok":false`) || !strings.Contains(stdout, `"type":"usage"`) {
			t.Fatalf("json usage stdout=%q stderr=%q code=%d", stdout, stderr, code)
		}
	})
}

func TestShowRendersLinksAndThread(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "Origin")
	createSeedTask(t, root, "b00001", "Middle", "--discovered-from", "a00001")
	createSeedTask(t, root, "c00001", "Leaf", "--discovered-from", "b00001")
	createSeedTask(t, root, "d00001", "Follow Up", "--discovered-from", "b00001")
	path := taskPathByID(t, root, "b00001")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), "created:", "links:\n  - url: https://example.test/pr/1\n    title: PR 1\n  - path: docs/design.md\ncreated:", 1)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"show", "b00001"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "links:") || !strings.Contains(stdout, "PR 1: https://example.test/pr/1") || !strings.Contains(stdout, "docs/design.md") {
		t.Fatalf("show links stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, root, nil, nil, []string{"show", "b00001", "--thread"})
	for _, want := range []string{"thread:", "^ a00001 [todo] Origin", "* b00001 [todo] Middle", "v c00001 [todo] Leaf", "v d00001 [todo] Follow Up"} {
		if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, want) {
			t.Fatalf("show thread missing %q stdout=%q stderr=%q code=%d", want, stdout, stderr, code)
		}
	}
	stdout, stderr, code = runCLI(t, root, nil, nil, []string{"--json", "show", "b00001", "--thread"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, `"links":[`) || !strings.Contains(stdout, `"thread":[`) {
		t.Fatalf("show thread json stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}

func runCLI(t *testing.T, root string, ids []string, stdin *strings.Reader, args []string) (stdout, stderr string, code int) {
	t.Helper()
	var out, err bytes.Buffer
	idIndex := 0
	app := New(Options{
		Version: "test",
		Stdin:   stdin,
		Stdout:  &out,
		Stderr:  &err,
		WorkDir: root,
		Env: func(string) string {
			return ""
		},
		GitUser: func(context.Context) (string, error) {
			return "", errors.New("git user unset")
		},
		Now: func() time.Time {
			return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		},
		NewID: func(map[string]struct{}, int) (string, error) {
			if idIndex >= len(ids) {
				return "z00000", nil
			}
			id := ids[idIndex]
			idIndex++
			return id, nil
		},
	})
	code = app.Execute(args)
	return out.String(), err.String(), code
}

func runCLIWithNewID(t *testing.T, root string, newID func(map[string]struct{}, int) (string, error), args []string) (stdout, stderr string, code int) {
	t.Helper()
	var out, err bytes.Buffer
	app := New(Options{
		Version: "test",
		Stdout:  &out,
		Stderr:  &err,
		WorkDir: root,
		Env: func(string) string {
			return ""
		},
		GitUser: func(context.Context) (string, error) {
			return "", errors.New("git user unset")
		},
		Now: func() time.Time {
			return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		},
		NewID: newID,
	})
	code = app.Execute(args)
	return out.String(), err.String(), code
}

func initProject(t *testing.T, root string) {
	t.Helper()
	_, stderr, code := runCLI(t, root, nil, nil, []string{"init", "--prefix", "sya"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("init stderr=%q code=%d", stderr, code)
	}
}

func createSeedTask(t *testing.T, root, id, title string, args ...string) {
	t.Helper()
	createArgs := append([]string{"create", title}, args...)
	_, stderr, code := runCLI(t, root, []string{id}, nil, createArgs)
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("create seed stderr=%q code=%d", stderr, code)
	}
}

func addSectionlessNoteType(t *testing.T, root string) {
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
  note:
    pipeline: [open, done]
    terminal: [done]
    transitions:
      open -> done: {}
`); err != nil {
		t.Fatalf("append schema type: %v", err)
	}
}

func fixtureProject(t *testing.T, root string) {
	t.Helper()
	initProject(t, root)
	createSeedTask(t, root, "e00001", "Platform Epic", "-t", "epic")
	createSeedTask(t, root, "d00001", "Dependency Task")
	createSeedTask(t, root, "f00001", "Build API", "-t", "feature", "-p", "high", "--parent", "e00001", "--depends-on", "d00001", "-d", "Feature description")
}

func normalizeCommandOutput(root, stdout, stderr string, code int) string {
	normalized := "code: " + strconv.Itoa(code) + "\nstdout:\n" + stdout + "stderr:\n" + stderr
	normalized = strings.ReplaceAll(normalized, filepath.ToSlash(root), "$ROOT")
	normalized = strings.ReplaceAll(normalized, root, "$ROOT")
	normalized = strings.ReplaceAll(normalized, "\x1b[31merror:\x1b[0m", "error:")
	return strings.TrimSpace(normalized)
}
