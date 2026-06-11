package doctor

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/task"
)

func TestRunCleanFixture(t *testing.T) {
	t.Parallel()

	_, _, report := runFixture(t, "testdata/clean")
	if len(report.Findings) != 0 {
		t.Fatalf("clean fixture findings = %#v, want none", report.Findings)
	}
}

func TestRunDirtyFixture(t *testing.T) {
	t.Parallel()

	_, _, report := runFixture(t, "testdata/dirty")
	kinds := findingKinds(report)
	for _, want := range []string{
		"conflict_markers",
		"duplicate_id",
		"symmetric_duplicate_edge",
		"task_status_unknown",
		"schema_version_drift",
		"schema_version_future",
		"field_unknown",
		"field_type_invalid",
		"parent_not_container",
		"relation_unknown",
		"dangling_relation",
		"section_unknown",
		"relation_cycle",
		"parent_cycle",
	} {
		if !kinds[want] {
			t.Fatalf("missing finding %q in %#v", want, report.Findings)
		}
	}
	if got := findingByKind(report, "duplicate_id"); got == nil || len(got.Paths) != 2 {
		t.Fatalf("duplicate_id finding = %#v, want both paths", got)
	}
	if got := findingByKind(report, "symmetric_duplicate_edge"); got == nil || !got.Fixable {
		t.Fatalf("symmetric duplicate finding = %#v, want fixable", got)
	}
}

func TestFixMerge(t *testing.T) {
	t.Parallel()

	path := copyFixture(t, "testdata/fixmerge/log-conflict.md")
	changes, err := FixMerge(path)
	if err != nil {
		t.Fatalf("FixMerge() error = %v", err)
	}
	if len(changes) != 1 || changes[0].Action != "fix_merge" {
		t.Fatalf("changes = %#v", changes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("<<<<<<<")) {
		t.Fatalf("conflict markers remain:\n%s", data)
	}
	for _, want := range []string{"@test: created", "@codex: ours", "@claude: theirs"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Fatalf("merged log missing %q:\n%s", want, data)
		}
	}
	if count := strings.Count(string(data), "@codex: ours"); count != 1 {
		t.Fatalf("duplicate log line count = %d, want 1\n%s", count, data)
	}
}

func TestFixMergeRefusesFrontmatterConflict(t *testing.T) {
	t.Parallel()

	path := copyFixture(t, "testdata/fixmerge/frontmatter-conflict.md")
	if _, err := FixMerge(path); err == nil {
		t.Fatal("FixMerge() error = nil, want refusal")
	}
}

func TestReassignIDUpdatesInboundReferences(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeSchema(t, dir)
	writeTaskFile(t, dir, "tasks/old001-target.md", &task.Task{ID: "old001", Type: "task", Title: "Target", Status: "todo", SchemaVersion: 1, Body: body("Description", "target\n")})
	writeTaskFile(t, dir, "tasks/ref001-dep.md", &task.Task{ID: "ref001", Type: "task", Title: "Ref dep", Status: "todo", Relations: map[string][]string{"depends_on": {"old001"}}, SchemaVersion: 1, Body: body("Description", "dep\n")})
	writeTaskFile(t, dir, "tasks/ref002-disc.md", &task.Task{ID: "ref002", Type: "task", Title: "Ref disc", Status: "todo", Relations: map[string][]string{"discovered_from": {"old001"}}, SchemaVersion: 1, Body: body("Description", "disc\n")})
	writeTaskFile(t, dir, "tasks/ref003-rel.md", &task.Task{ID: "ref003", Type: "task", Title: "Ref rel", Status: "todo", Relations: map[string][]string{"relates": {"old001"}}, SchemaVersion: 1, Body: body("Description", "rel\n")})
	writeTaskFile(t, dir, "tasks/ref004-parent.md", &task.Task{ID: "ref004", Type: "task", Title: "Ref parent", Status: "todo", Parent: "old001", SchemaVersion: 1, Body: body("Description", "parent\n")})

	sch, idx := loadProject(t, os.DirFS(dir), ".")
	_ = sch
	changes, err := ReassignIDInDir(dir, idx, "old001")
	if err != nil {
		t.Fatalf("ReassignIDInDir() error = %v", err)
	}
	newID := changeTo(t, changes, "reassign_id")
	reloadedSchema, reloadedIndex := loadProject(t, os.DirFS(dir), ".")
	report, err := Run(os.DirFS(dir), ".", reloadedSchema, reloadedIndex, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if hasKind(report, "dangling_relation") || hasKind(report, "dangling_parent") {
		t.Fatalf("reassign left dangling references: %#v", report.Findings)
	}
	if _, err := reloadedIndex.Get("old001"); err == nil {
		t.Fatal("old id still resolves")
	}
	if _, err := reloadedIndex.Get(newID); err != nil {
		t.Fatalf("new id %q does not resolve: %v", newID, err)
	}
	assertReference(t, reloadedIndex, "ref001", "depends_on", newID)
	assertReference(t, reloadedIndex, "ref002", "discovered_from", newID)
	assertReference(t, reloadedIndex, "ref003", "relates", newID)
	parentTask, _ := reloadedIndex.Get("ref004")
	if parentTask.Parent != newID {
		t.Fatalf("parent = %q, want %q", parentTask.Parent, newID)
	}
}

func TestFixSymmetricDup(t *testing.T) {
	t.Parallel()

	dir := copyProject(t, "testdata/dirty")
	sch, idx := loadProject(t, os.DirFS(dir), ".")
	if !hasKind(mustReport(t, sch, idx, dir), "symmetric_duplicate_edge") {
		t.Fatal("dirty fixture missing symmetric duplicate before fix")
	}
	changes, err := FixSymmetricDup(dir, idx, sch)
	if err != nil {
		t.Fatalf("FixSymmetricDup() error = %v", err)
	}
	if len(changes) == 0 {
		t.Fatal("FixSymmetricDup() returned no changes")
	}
	sch, idx = loadProject(t, os.DirFS(dir), ".")
	if hasKind(mustReport(t, sch, idx, dir), "symmetric_duplicate_edge") {
		t.Fatal("symmetric duplicate remains after fix")
	}
}

func TestFixRelationListsWriterFakeMatchesReal(t *testing.T) {
	t.Parallel()

	realDir := t.TempDir()
	fakeDir := t.TempDir()
	for _, dir := range []string{realDir, fakeDir} {
		writeSchema(t, dir)
		writeTaskFile(t, dir, "tasks/a00001-target.md", &task.Task{ID: "a00001", Type: "task", Title: "Target", Status: "todo", SchemaVersion: 1, Body: body("Description", "target\n")})
		writeTaskFile(t, dir, "tasks/z00001-target.md", &task.Task{ID: "z00001", Type: "task", Title: "Target Z", Status: "todo", SchemaVersion: 1, Body: body("Description", "target z\n")})
		writeTaskFile(t, dir, "tasks/ref001-ref.md", &task.Task{ID: "ref001", Type: "task", Title: "Ref", Status: "todo", Relations: map[string][]string{"depends_on": {"z00001", "a00001", "a00001"}}, SchemaVersion: 1, Body: body("Description", "ref\n")})
	}

	_, realIndex := loadProject(t, os.DirFS(realDir), ".")
	realChanges, err := FixRelationListsWith(OSWriter{}, realDir, realIndex)
	if err != nil {
		t.Fatalf("FixRelationListsWith(OSWriter) error = %v", err)
	}
	realData, err := os.ReadFile(filepath.Join(realDir, "tasks", "ref001-ref.md"))
	if err != nil {
		t.Fatalf("read real fixed task: %v", err)
	}

	_, fakeIndex := loadProject(t, os.DirFS(fakeDir), ".")
	fakeWriter := newFakeDoctorWriter()
	fakeChanges, err := FixRelationListsWith(fakeWriter, fakeDir, fakeIndex)
	if err != nil {
		t.Fatalf("FixRelationListsWith(fake) error = %v", err)
	}
	fakePath := filepath.Join(fakeDir, "tasks", "ref001-ref.md")
	if !bytes.Equal(fakeWriter.files[fakePath], realData) {
		t.Fatalf("fake write differs\n--- got ---\n%s\n--- want ---\n%s", fakeWriter.files[fakePath], realData)
	}
	if fmt.Sprint(fakeChanges) != fmt.Sprint(realChanges) {
		t.Fatalf("fake changes = %#v, want %#v", fakeChanges, realChanges)
	}
}

func TestGeneratedValidProjectsHaveNoFindings(t *testing.T) {
	t.Parallel()

	for i := 0; i < 100; i++ {
		i := i
		t.Run(fmt.Sprintf("project_%03d", i), func(t *testing.T) {
			t.Parallel()
			fsys := generatedProject(i)
			sch, idx := loadProject(t, fsys, ".")
			report, err := Run(fsys, ".", sch, idx, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if len(report.Findings) != 0 {
				t.Fatalf("generated valid project findings = %#v", report.Findings)
			}
		})
	}
}

func runFixture(t *testing.T, dir string) (*schema.Schema, *index.Index, Report) {
	t.Helper()
	sch, idx := loadProject(t, os.DirFS(dir), ".")
	report, err := Run(os.DirFS(dir), ".", sch, idx, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return sch, idx, report
}

func loadProject(t *testing.T, fsys fs.FS, projectDir string) (*schema.Schema, *index.Index) {
	t.Helper()
	data, err := fs.ReadFile(fsys, "schema.yml")
	if err != nil {
		t.Fatal(err)
	}
	sch, err := schema.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := index.Load(fsys, projectDir, sch)
	if err != nil {
		t.Fatal(err)
	}
	return sch, idx
}

func mustReport(t *testing.T, sch *schema.Schema, idx *index.Index, dir string) Report {
	t.Helper()
	report, err := Run(os.DirFS(dir), ".", sch, idx, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func findingKinds(report Report) map[string]bool {
	kinds := make(map[string]bool)
	for _, finding := range report.Findings {
		kinds[finding.Kind] = true
	}
	return kinds
}

func findingByKind(report Report, kind string) *Finding {
	for i := range report.Findings {
		if report.Findings[i].Kind == kind {
			return &report.Findings[i]
		}
	}
	return nil
}

func hasKind(report Report, kind string) bool {
	return findingKinds(report)[kind]
}

func copyFixture(t *testing.T, source string) string {
	t.Helper()
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), filepath.Base(source))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func copyProject(t *testing.T, source string) string {
	t.Helper()
	dest := t.TempDir()
	if err := filepath.WalkDir(source, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil || rel == "." {
			return err
		}
		out := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	}); err != nil {
		t.Fatal(err)
	}
	return dest
}

func writeSchema(t *testing.T, dir string) {
	t.Helper()
	data, err := os.ReadFile("testdata/clean/schema.yml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "schema.yml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTaskFile(t *testing.T, dir, rel string, task *task.Task) {
	t.Helper()
	out, err := taskpkgSerialize(task)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func taskpkgSerialize(t *task.Task) ([]byte, error) {
	return task.Serialize(t)
}

func body(name, text string) task.Body {
	raw := []byte("## " + name + "\n" + text)
	return task.Body{Raw: raw, Sections: []task.Section{{Name: name, Raw: raw}}}
}

func changeTo(t *testing.T, changes []Change, action string) string {
	t.Helper()
	for _, change := range changes {
		if change.Action == action {
			if change.To == "" {
				t.Fatalf("change %q has empty To: %#v", action, change)
			}
			return change.To
		}
	}
	t.Fatalf("missing change action %q in %#v", action, changes)
	return ""
}

func assertReference(t *testing.T, idx *index.Index, id, relation, target string) {
	t.Helper()
	task, err := idx.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(task.Relations[relation], target) {
		t.Fatalf("%s.%s = %#v, want %q", id, relation, task.Relations[relation], target)
	}
}

type fakeDoctorWriter struct {
	files   map[string][]byte
	removed []string
}

func newFakeDoctorWriter() *fakeDoctorWriter {
	return &fakeDoctorWriter{files: make(map[string][]byte)}
}

func (w *fakeDoctorWriter) WriteFile(name string, data []byte, _ fs.FileMode) error {
	w.files[name] = append([]byte(nil), data...)
	return nil
}

func (w *fakeDoctorWriter) Remove(name string) error {
	w.removed = append(w.removed, name)
	return nil
}

func generatedProject(seed int) fstest.MapFS {
	files := fstest.MapFS{
		"schema.yml": {Data: []byte(generatedSchemaYAML())},
	}
	count := 3 + seed%5
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("t%05x", seed*16+i)
		status := "todo"
		if i%3 == 1 {
			status = "doing"
		}
		if i%3 == 2 {
			status = "done"
		}
		relations := ""
		if i > 0 {
			prev := fmt.Sprintf("t%05x", seed*16+i-1)
			relations = fmt.Sprintf("relations:\n  discovered_from: [%s]\n", prev)
		}
		files[fmt.Sprintf("tasks/%s-generated.md", id)] = &fstest.MapFile{Data: []byte(fmt.Sprintf(`---
id: %s
type: task
title: Generated %d
status: %s
%sfields:
  ready: %t
  size: small
schema_version: 2
---
## Description
Generated valid task.
`, id, i, status, relations, i%2 == 0))}
	}
	return files
}

func generatedSchemaYAML() string {
	data, err := os.ReadFile("testdata/clean/schema.yml")
	if err != nil {
		panic(err)
	}
	return string(data)
}

func findingKindList(report Report) []string {
	var kinds []string
	for _, finding := range report.Findings {
		kinds = append(kinds, finding.Kind)
	}
	sort.Strings(kinds)
	return kinds
}
