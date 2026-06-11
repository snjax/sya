package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snjax/sya/internal/syaerr"
)

func TestAgentCommandGoldens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T, root string) (stdout, stderr string, code int)
	}{
		{name: "comment_human", run: func(t *testing.T, root string) (string, string, int) {
			fixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--actor", "codex", "comment", "f000", "-m", "Looks good\nship it"})
		}},
		{name: "comment_json", run: func(t *testing.T, root string) (string, string, int) {
			fixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "--actor", "codex", "comment", "f000", "-m", "Looks good\nship it"})
		}},
		{name: "board_human", run: func(t *testing.T, root string) (string, string, int) {
			boardFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"board"})
		}},
		{name: "board_json", run: func(t *testing.T, root string) (string, string, int) {
			boardFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "board", "--type", "task"})
		}},
		{name: "epic_tree_human", run: func(t *testing.T, root string) (string, string, int) {
			epicFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"epic", "tree", "e00001"})
		}},
		{name: "epic_tree_json", run: func(t *testing.T, root string) (string, string, int) {
			epicFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "epic", "tree", "e00001"})
		}},
		{name: "epic_progress_human", run: func(t *testing.T, root string) (string, string, int) {
			epicFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"epic", "progress", "e00001"})
		}},
		{name: "epic_progress_json", run: func(t *testing.T, root string) (string, string, int) {
			epicFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "epic", "progress", "e00001"})
		}},
		{name: "search_human", run: func(t *testing.T, root string) (string, string, int) {
			searchFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"search", "api"})
		}},
		{name: "search_json", run: func(t *testing.T, root string) (string, string, int) {
			searchFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "search", "api", "--limit", "2"})
		}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			stdout, stderr, code := tt.run(t, root)
			got := normalizeCommandOutput(root, stdout, stderr, code)
			wantBytes, err := os.ReadFile(filepath.Join("testdata", "agent", tt.name+".golden"))
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

func TestAgentCommandErrorPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		setup    func(t *testing.T, root string)
		args     []string
		wantCode int
		wantText string
	}{
		{
			name: "comment missing message",
			setup: func(t *testing.T, root string) {
				fixtureProject(t, root)
			},
			args:     []string{"comment", "f000"},
			wantCode: syaerr.ExitUsage,
			wantText: "comment message is required",
		},
		{
			name: "board unknown type",
			setup: func(t *testing.T, root string) {
				initProject(t, root)
			},
			args:     []string{"board", "--type", "ghost"},
			wantCode: syaerr.ExitUsage,
			wantText: "unknown task type",
		},
		{
			name: "epic non container",
			setup: func(t *testing.T, root string) {
				fixtureProject(t, root)
			},
			args:     []string{"epic", "tree", "f000"},
			wantCode: syaerr.ExitUsage,
			wantText: "not an epic/container",
		},
		{
			name: "search empty",
			setup: func(t *testing.T, root string) {
				initProject(t, root)
			},
			args:     []string{"search", "   "},
			wantCode: syaerr.ExitUsage,
			wantText: "search query is required",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			tt.setup(t, root)
			stdout, stderr, code := runCLI(t, root, nil, nil, tt.args)
			if code != tt.wantCode {
				t.Fatalf("code=%d want=%d stdout=%q stderr=%q", code, tt.wantCode, stdout, stderr)
			}
			if !strings.Contains(stdout+stderr, tt.wantText) {
				t.Fatalf("output does not contain %q\nstdout=%q\nstderr=%q", tt.wantText, stdout, stderr)
			}
		})
	}
}

func TestCommentAppendsLog(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fixtureProject(t, root)
	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"--actor", "codex", "comment", "f000", "-m", "Line one\nLine two"})
	if code != syaerr.ExitOK || stderr != "" {
		t.Fatalf("comment stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	stdout, stderr, code = runCLI(t, root, nil, nil, []string{"show", "f000"})
	if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "- 2026-01-02T03:04:05Z @codex: Line one\nLine two") {
		t.Fatalf("show stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}

func boardFixtureProject(t *testing.T, root string) {
	t.Helper()
	initProject(t, root)
	appendNoteType(t, root)
	createSeedTask(t, root, "e00001", "Platform Epic", "-t", "epic")
	createSeedTask(t, root, "f00001", "Feature API", "-t", "feature", "-p", "high", "--parent", "e00001")
	createSeedTask(t, root, "b00001", "Bug Fix", "-t", "bug")
	createSeedTask(t, root, "t00001", "Worker Task")
	createSeedTask(t, root, "n00001", "Research Note", "-t", "note")
	createSeedTask(t, root, "a00001", "Archived Task")
	mustRun(t, root, nil, []string{"move", "t00001", "in_progress"})
	mustRun(t, root, nil, []string{"update", "t00001", "--assignee", "codex"})
	markTaskArchived(t, root, "a00001")
}

func epicFixtureProject(t *testing.T, root string) {
	t.Helper()
	initProject(t, root)
	allowNestedEpics(t, root)
	createSeedTask(t, root, "e00001", "Root Epic", "-t", "epic")
	createSeedTask(t, root, "e00002", "Child Epic", "-t", "epic", "--parent", "e00001")
	createSeedTask(t, root, "e00003", "Grandchild Epic", "-t", "epic", "--parent", "e00002")
	createSeedTask(t, root, "t00001", "Leaf Task", "--parent", "e00003")
	mustRun(t, root, nil, []string{"move", "t00001", "in_progress"})
	mustRun(t, root, nil, []string{"close", "t00001"})
}

func searchFixtureProject(t *testing.T, root string) {
	t.Helper()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "API Gateway", "-d", "Title match should rank first")
	createSeedTask(t, root, "b00001", "Worker", "-d", "Body mentions api details")
	createSeedTask(t, root, "c00001", "Unrelated", "-d", "No match")
	createSeedTask(t, root, "d00001", "Docs API", "-t", "docs")
}

func appendNoteType(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, ".sya", "schema.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	data = append(data, []byte("\n  note:\n    board: false\n    pipeline: [active, superseded]\n    terminal: [superseded]\n    parked: [active]\n    transitions:\n      active -> superseded: {}\n")...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
}

func allowNestedEpics(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, ".sya", "schema.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	updated := strings.Replace(string(data), "children: [feature, bug, docs, task]", "children: [epic, feature, bug, docs, task]", 1)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
}

func markTaskArchived(t *testing.T, root, id string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, ".sya", "tasks", id+"*.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("find task %s matches=%v err=%v", id, matches, err)
	}
	data, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatalf("read task: %v", err)
	}
	updated := strings.Replace(string(data), "---\n## Description", "archived: true\n---\n## Description", 1)
	if updated == string(data) {
		t.Fatalf("failed to mark archived in %s", matches[0])
	}
	if err := os.WriteFile(matches[0], []byte(updated), 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}
}
