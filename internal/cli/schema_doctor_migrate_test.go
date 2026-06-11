package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/snjax/sya/internal/syaerr"
)

func TestSchemaMigrateGoldens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(t *testing.T, root string) (stdout, stderr string, code int)
	}{
		{name: "migrate_human", run: func(t *testing.T, root string) (string, string, int) {
			migrateFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"schema", "migrate", "--rename-status", "scrapped=done"})
		}},
		{name: "migrate_json", run: func(t *testing.T, root string) (string, string, int) {
			migrateFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"--json", "schema", "migrate", "--rename-status", "scrapped=done"})
		}},
		{name: "migrate_dry_run_human", run: func(t *testing.T, root string) (string, string, int) {
			migrateFixtureProject(t, root)
			return runCLI(t, root, nil, nil, []string{"schema", "migrate", "--rename-status", "scrapped=done", "--dry-run"})
		}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			stdout, stderr, code := tt.run(t, root)
			got := normalizeCommandOutput(root, stdout, stderr, code)
			wantBytes, err := os.ReadFile(filepath.Join("testdata", "schema", tt.name+".golden"))
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

func TestSchemaMigrateEffectsAndErrors(t *testing.T) {
	t.Parallel()

	t.Run("rewrites archived and bumps schema version", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		migrateFixtureProject(t, root)
		stdout, stderr, code := runCLI(t, root, nil, nil, []string{"schema", "migrate", "--rename-status", "scrapped=done"})
		if code != syaerr.ExitOK || stderr != "" {
			t.Fatalf("migrate stdout=%q stderr=%q code=%d", stdout, stderr, code)
		}
		data := readTaskByID(t, root, "a00001")
		for _, want := range []string{"status: done", "schema_version: 2", "migrated: status scrapped->done", "archived: true"} {
			if !strings.Contains(data, want) {
				t.Fatalf("archived migrated task missing %q:\n%s", want, data)
			}
		}
	})

	t.Run("dry run leaves files untouched", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		migrateFixtureProject(t, root)
		before := readTaskByID(t, root, "s00001")
		stdout, stderr, code := runCLI(t, root, nil, nil, []string{"schema", "migrate", "--rename-status", "scrapped=done", "--dry-run"})
		if code != syaerr.ExitOK || stderr != "" || !strings.Contains(stdout, "would migrate s00001") {
			t.Fatalf("dry run stdout=%q stderr=%q code=%d", stdout, stderr, code)
		}
		after := readTaskByID(t, root, "s00001")
		if after != before {
			t.Fatalf("dry-run changed task\nbefore:\n%s\nafter:\n%s", before, after)
		}
	})

	t.Run("new status must be in pipeline", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		migrateFixtureProject(t, root)
		_, stderr, code := runCLI(t, root, nil, nil, []string{"schema", "migrate", "--rename-status", "scrapped=missing"})
		if code != syaerr.ExitUsage || !strings.Contains(stderr, "new status is not in pipeline") {
			t.Fatalf("stderr=%q code=%d", stderr, code)
		}
	})
}

func TestDoctorFixGoldensAndIdempotence(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	doctorFixFixtureProject(t, root)
	stdout, stderr, code := runCLI(t, root, nil, nil, []string{"doctor", "--fix"})
	got := normalizeCommandOutput(root, stdout, stderr, code)
	wantBytes, err := os.ReadFile(filepath.Join("testdata", "schema", "doctor_fix_human.golden"))
	if err != nil {
		t.Fatalf("read golden: %v\n\ngot:\n%s", err, got)
	}
	if strings.TrimSpace(got) != strings.TrimSpace(string(wantBytes)) {
		t.Fatalf("golden mismatch\nwant:\n%s\n\ngot:\n%s", strings.TrimSpace(string(wantBytes)), got)
	}
	if code != syaerr.ExitSchemaInvalid || stderr != "" {
		t.Fatalf("doctor --fix stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
	if data := readTaskByID(t, root, "bad001"); !strings.Contains(data, "status: ghost") || !strings.Contains(data, "schema_version: 1") {
		t.Fatalf("unsafe task was changed:\n%s", data)
	}

	stdout, stderr, code = runCLI(t, root, nil, nil, []string{"doctor", "--fix"})
	got = normalizeCommandOutput(root, stdout, stderr, code)
	wantBytes, err = os.ReadFile(filepath.Join("testdata", "schema", "doctor_fix_second_human.golden"))
	if err != nil {
		t.Fatalf("read golden: %v\n\ngot:\n%s", err, got)
	}
	if strings.TrimSpace(got) != strings.TrimSpace(string(wantBytes)) {
		t.Fatalf("second golden mismatch\nwant:\n%s\n\ngot:\n%s", strings.TrimSpace(string(wantBytes)), got)
	}
	if code != syaerr.ExitSchemaInvalid || stderr != "" {
		t.Fatalf("second doctor --fix stdout=%q stderr=%q code=%d", stdout, stderr, code)
	}
}

func migrateFixtureProject(t *testing.T, root string) {
	t.Helper()
	initProject(t, root)
	createSeedTask(t, root, "s00001", "Scrapped Task")
	createSeedTask(t, root, "f00001", "Scrapped Feature", "-t", "feature")
	createSeedTask(t, root, "a00001", "Archived Scrapped Task")
	mustRun(t, root, nil, []string{"move", "s00001", "scrapped"})
	mustRun(t, root, nil, []string{"move", "f00001", "scrapped"})
	mustRun(t, root, nil, []string{"move", "a00001", "scrapped"})
	markTaskArchived(t, root, "a00001")
	setFixtureSchemaVersion(t, root, 2)
}

func doctorFixFixtureProject(t *testing.T, root string) {
	t.Helper()
	initProject(t, root)
	createSeedTask(t, root, "a00001", "A")
	createSeedTask(t, root, "b00001", "B")
	createSeedTask(t, root, "c00001", "C")
	createSeedTask(t, root, "d00001", "D")
	createSeedTask(t, root, "bad001", "Bad")
	setFixtureSchemaVersion(t, root, 2)
	addTaskDescriptionSection(t, root)
	replaceTaskLine(t, root, "bad001", "status:", "status: ghost")
	addRelationsBlock(t, root, "a00001", "relations:\n  relates: [b00001]\n  depends_on: [d00001, b00001, b00001]\n")
	addRelationsBlock(t, root, "b00001", "relations:\n  relates: [a00001]\n")
}

func setFixtureSchemaVersion(t *testing.T, root string, version int) {
	t.Helper()
	path := filepath.Join(root, ".sya", "schema.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	updated := strings.Replace(string(data), "schema_version: 1", "schema_version: 2", 1)
	if version != 2 {
		updated = strings.Replace(string(data), "schema_version: 1", "schema_version: "+strconv.Itoa(version), 1)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
}

func addTaskDescriptionSection(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, ".sya", "schema.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	updated := strings.Replace(string(data), "  task:\n    pipeline:", "  task:\n    sections: [Description]\n    pipeline:", 1)
	if updated == string(data) {
		t.Fatalf("failed to add task sections")
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
}

func addRelationsBlock(t *testing.T, root, id, block string) {
	t.Helper()
	path := taskPathByID(t, root, id)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read task: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "relations:\n") {
		t.Fatalf("task %s already has relations:\n%s", id, text)
	}
	updated := strings.Replace(text, "fields:\n", block+"fields:\n", 1)
	if updated == text {
		updated = strings.Replace(text, "created:", block+"created:", 1)
	}
	if updated == text {
		t.Fatalf("failed to insert relations in %s:\n%s", id, text)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}
}

func replaceTaskLine(t *testing.T, root, id, prefix, replacement string) {
	t.Helper()
	path := taskPathByID(t, root, id)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read task: %v", err)
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), strings.TrimSpace(prefix)) {
			lines[i] = replacement
			if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
				t.Fatalf("write task: %v", err)
			}
			return
		}
	}
	t.Fatalf("prefix %q not found in %s", prefix, id)
}

func readTaskByID(t *testing.T, root, id string) string {
	t.Helper()
	data, err := os.ReadFile(taskPathByID(t, root, id))
	if err != nil {
		t.Fatalf("read task: %v", err)
	}
	return string(data)
}

func taskPathByID(t *testing.T, root, id string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, ".sya", "tasks", id+"*.md"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("find task %s matches=%v err=%v", id, matches, err)
	}
	return matches[0]
}
