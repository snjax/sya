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
	t.Run("batch from stdin", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		initProject(t, root)
		batch := "- title: Batch One\n  type: task\n  priority: low\n  fields:\n    estimate: 3\n"
		stdout, stderr, code := runCLI(t, root, []string{"b00001"}, strings.NewReader(batch), []string{"create", "--from-file", "-"})
		if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "created b00001") {
			t.Fatalf("stdout=%q stderr=%q code=%d", stdout, stderr, code)
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
		if code != syaerr.ExitLookup || !strings.Contains(stderr, "ambiguous prefix") {
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
