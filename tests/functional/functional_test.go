package functional_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var (
	updateGoldens = flag.Bool("update", false, "update golden files")
	syaBin        string
	repoRoot      string
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
	dir, err := os.MkdirTemp("", "sya-functional-*")
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

func TestFunctionalGoldens(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, root string) runResult
	}{
		{"init_success", func(t *testing.T, root string) runResult {
			return runSya(t, root, nil, "init", "--prefix", "acme")
		}},
		{"init_idempotence_error", func(t *testing.T, root string) runResult {
			mustOK(t, runSya(t, root, nil, "init"))
			return runSya(t, root, nil, "init")
		}},
		{"create_all_flags", func(t *testing.T, root string) runResult {
			mustOK(t, runSya(t, root, nil, "init"))
			epic := createJSON(t, root, "Platform", "-t", "epic")
			dep := createJSON(t, root, "Dependency")
			return runSya(t, root, nil, "create", "Build API", "-t", "feature", "-p", "high", "--parent", epic, "--depends-on", dep, "--discovered-from", dep, "--field", "spec_approved=true", "-d", "Feature description")
		}},
		{"create_batch_stdin", func(t *testing.T, root string) runResult {
			mustOK(t, runSya(t, root, nil, "init"))
			stdin := strings.NewReader("- title: Batch One\n  type: task\n  priority: low\n  description: From stdin\n")
			return runSya(t, root, stdin, "create", "--from-file", "-")
		}},
		{"show", func(t *testing.T, root string) runResult {
			id := fixtureProject(t, root)
			return runSya(t, root, nil, "show", id[:3])
		}},
		{"list_filters", func(t *testing.T, root string) runResult {
			fixtureProject(t, root)
			return runSya(t, root, nil, "list", "-t", "feature", "--limit", "1")
		}},
		{"transitions", func(t *testing.T, root string) runResult {
			id := fixtureProject(t, root)
			return runSya(t, root, nil, "transitions", id[:3])
		}},
		{"doctor_clean", func(t *testing.T, root string) runResult {
			mustOK(t, runSya(t, root, nil, "init"))
			return runSya(t, root, nil, "doctor")
		}},
		{"doctor_broken", func(t *testing.T, root string) runResult {
			copyDoctorFixture(t, "dirty", root)
			return runSya(t, root, nil, "doctor")
		}},
		{"doctor_fix_merge", func(t *testing.T, root string) runResult {
			setupFixMergeProject(t, root)
			return runSya(t, root, nil, "doctor", "--fix-merge")
		}},
		{"doctor_reassign_id", func(t *testing.T, root string) runResult {
			setupDuplicateProject(t, root)
			return runSya(t, root, nil, "doctor", "--reassign-id", "aaaaaa")
		}},
		{"json_success_envelope", func(t *testing.T, root string) runResult {
			mustOK(t, runSya(t, root, nil, "init"))
			return runSya(t, root, nil, "--json", "doctor")
		}},
		{"json_not_found", func(t *testing.T, root string) runResult {
			mustOK(t, runSya(t, root, nil, "init"))
			return runSya(t, root, nil, "--json", "show", "abc123")
		}},
		{"json_ambiguous", func(t *testing.T, root string) runResult {
			setupAmbiguousProject(t, root)
			return runSya(t, root, nil, "--json", "show", "abc")
		}},
		{"json_usage", func(t *testing.T, root string) runResult {
			mustOK(t, runSya(t, root, nil, "init"))
			return runSya(t, root, nil, "--json", "create")
		}},
		{"json_schema_invalid", func(t *testing.T, root string) runResult {
			mustOK(t, runSya(t, root, nil, "init"))
			mustWrite(t, filepath.Join(root, ".sya", "schema.yml"), []byte("schema_version: [\n"))
			return runSya(t, root, nil, "--json", "list")
		}},
		{"claim_feature_draft", func(t *testing.T, root string) runResult {
			mustOK(t, runSya(t, root, nil, "init"))
			id := createJSON(t, root, "Draft Feature", "-t", "feature")
			return runSya(t, root, nil, "claim", id)
		}},
		{"human_transition_blocked_details", func(t *testing.T, root string) runResult {
			mustOK(t, runSya(t, root, nil, "init"))
			id := createJSON(t, root, "Blocked Feature", "-t", "feature")
			mustOK(t, runSya(t, root, nil, "move", id, "spec"))
			return runSya(t, root, nil, "move", id, "impl")
		}},
		{"close_ambiguous", func(t *testing.T, root string) runResult {
			mustOK(t, runSya(t, root, nil, "init"))
			id := createJSON(t, root, "Close Feature", "-t", "feature")
			mustOK(t, runSya(t, root, nil, "move", id, "spec"))
			mustOK(t, runSya(t, root, nil, "update", id, "--field", "spec_approved=true"))
			mustOK(t, runSya(t, root, strings.NewReader("Design text\n"), "edit", id, "--section", "Design", "--file", "-"))
			mustOK(t, runSya(t, root, nil, "move", id, "impl"))
			return runSya(t, root, nil, "close", id)
		}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			got := transcript(root, tt.run(t, root))
			assertGolden(t, tt.name, got)
		})
	}

	// TODO(sya-tym): add transition_not_allowed JSON envelope golden.
	// TODO(sya-tym): add transition_blocked JSON envelope golden.
}

func runSya(t *testing.T, dir string, stdin io.Reader, args ...string) runResult {
	t.Helper()
	cmd := exec.Command(syaBin, args...)
	cmd.Dir = dir
	cmd.Stdin = stdin
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "SYA_ACTOR=functional")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("run %v: %v", args, err)
		}
	}
	return runResult{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func mustOK(t *testing.T, result runResult) {
	t.Helper()
	if result.code != 0 {
		t.Fatalf("command failed: code=%d stdout=%q stderr=%q", result.code, result.stdout, result.stderr)
	}
}

func createJSON(t *testing.T, root, title string, args ...string) string {
	t.Helper()
	cmdArgs := append([]string{"--json", "create", title}, args...)
	result := runSya(t, root, nil, cmdArgs...)
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

func fixtureProject(t *testing.T, root string) string {
	t.Helper()
	mustOK(t, runSya(t, root, nil, "init"))
	epic := createJSON(t, root, "Platform", "-t", "epic")
	dep := createJSON(t, root, "Dependency")
	return createJSON(t, root, "Build API", "-t", "feature", "-p", "high", "--parent", epic, "--depends-on", dep, "-d", "Feature description")
}

func copyDoctorFixture(t *testing.T, name, root string) {
	t.Helper()
	source := filepath.Join(repoRoot, "internal", "doctor", "testdata", name)
	dest := filepath.Join(root, ".sya")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	copyTree(t, source, dest)
}

func setupFixMergeProject(t *testing.T, root string) {
	t.Helper()
	copyDoctorFixture(t, "clean", root)
	conflict, err := os.ReadFile(filepath.Join(repoRoot, "internal", "doctor", "testdata", "fixmerge", "log-conflict.md"))
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, ".sya", "tasks", "log001-log-conflict.md"), conflict)
}

func setupDuplicateProject(t *testing.T, root string) {
	t.Helper()
	copyDoctorFixture(t, "clean", root)
	tasksDir := filepath.Join(root, ".sya", "tasks")
	mustWrite(t, filepath.Join(tasksDir, "aaaaaa-one.md"), []byte(`---
id: aaaaaa
type: task
title: One
status: todo
schema_version: 2
---
## Description
One.
`))
	mustWrite(t, filepath.Join(tasksDir, "aaaaaa-two.md"), []byte(`---
id: aaaaaa
type: task
title: Two
status: todo
schema_version: 2
---
## Description
Two.
`))
	mustWrite(t, filepath.Join(tasksDir, "bbbbbb-ref.md"), []byte(`---
id: bbbbbb
type: task
title: Ref
status: todo
relations:
  depends_on: [aaaaaa]
  discovered_from: [aaaaaa]
schema_version: 2
---
## Description
Ref.
`))
}

func setupAmbiguousProject(t *testing.T, root string) {
	t.Helper()
	copyDoctorFixture(t, "clean", root)
	tasksDir := filepath.Join(root, ".sya", "tasks")
	mustWrite(t, filepath.Join(tasksDir, "abc111-one.md"), []byte(`---
id: abc111
type: task
title: One
status: todo
schema_version: 2
---
## Description
One.
`))
	mustWrite(t, filepath.Join(tasksDir, "abc222-two.md"), []byte(`---
id: abc222
type: task
title: Two
status: todo
schema_version: 2
---
## Description
Two.
`))
}

func copyTree(t *testing.T, source, dest string) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil || rel == "." {
			return err
		}
		out := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func transcript(root string, result runResult) string {
	out := fmt.Sprintf("code: %d\nstdout:\n%sstderr:\n%s", result.code, result.stdout, result.stderr)
	out = strings.ReplaceAll(out, filepath.ToSlash(root), "$ROOT")
	out = strings.ReplaceAll(out, root, "$ROOT")
	out = regexp.MustCompile(`\b[0-9a-f]{6}\b`).ReplaceAllString(out, "<id>")
	out = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z`).ReplaceAllString(out, "<time>")
	out = regexp.MustCompile(`sya [0-9a-f]{7,}(?:-dirty)?`).ReplaceAllString(out, "sya <version>")
	return strings.TrimSpace(out) + "\n"
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".golden")
	if *updateGoldens {
		mustWrite(t, path, []byte(got))
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", name, want, got)
	}
}
