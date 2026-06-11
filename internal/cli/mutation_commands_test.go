package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snjax/sya/internal/syaerr"
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
		{name: "move_offending_json", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "a00001", "Blocked")
			createSeedTask(t, root, "b00001", "Dependency")
			mustRun(t, root, nil, []string{"link", "a00001", "depends_on", "b00001"})
			return runCLI(t, root, nil, nil, []string{"--json", "move", "a00001", "in_progress"})
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
		{name: "update_human", run: func(t *testing.T, root string) (string, string, int) {
			initProject(t, root)
			createSeedTask(t, root, "b00001", "Bug", "-t", "bug")
			return runCLI(t, root, nil, nil, []string{"update", "b00001", "--priority", "critical", "--assignee", "codex", "--field", "severity=critical"})
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

func mustRun(t *testing.T, root string, stdin *strings.Reader, args []string) {
	t.Helper()
	stdout, stderr, code := runCLI(t, root, nil, stdin, args)
	if code != syaerr.ExitOK {
		t.Fatalf("command %v failed code=%d stdout=%q stderr=%q", args, code, stdout, stderr)
	}
}
