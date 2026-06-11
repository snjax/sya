package index

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
)

func TestLoadGoldens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files map[string]string
	}{
		{
			name: "valid_mix",
			files: map[string]string{
				"a-epic.md": taskDoc(taskFields{
					ID:       "a00001",
					Type:     "epic",
					Title:    "Epic",
					Status:   "active",
					Priority: "normal",
					Created:  "2026-01-01T09:00:00Z",
				}),
				"b-feature.md": taskDoc(taskFields{
					ID:       "b00001",
					Type:     "feature",
					Title:    "Feature",
					Status:   "impl",
					Priority: "high",
					Parent:   "a00001",
					Labels:   []string{"backend", "cli"},
					Relations: map[string][]string{
						"depends_on": {"c00001"},
						"relates":    {"d00001"},
					},
					Created: "2026-01-01T10:00:00Z",
				}),
				"c-task.md": taskDoc(taskFields{
					ID:       "c00001",
					Type:     "task",
					Title:    "Task",
					Status:   "done",
					Priority: "low",
					Created:  "2026-01-01T08:00:00Z",
				}),
				"d-note.md": taskDoc(taskFields{
					ID:       "d00001",
					Type:     "note",
					Title:    "Note",
					Status:   "active",
					Priority: "deferred",
					Created:  "2026-01-01T11:00:00Z",
					Archived: true,
				}),
			},
		},
		{
			name: "quarantine",
			files: map[string]string{
				"good.md": taskDoc(taskFields{
					ID:       "a00001",
					Type:     "task",
					Title:    "Good",
					Status:   "todo",
					Priority: "normal",
					Created:  "2026-01-01T09:00:00Z",
				}),
				"bad-frontmatter.md": "---\ntitle: missing id\n---\n",
				"conflict.md":        "<<<<<<< HEAD\n---\nid: badbad\ntype: task\nstatus: todo\n---\n",
			},
		},
		{
			name: "duplicate_ids",
			files: map[string]string{
				"a-first.md": taskDoc(taskFields{
					ID:       "dup001",
					Type:     "task",
					Title:    "First",
					Status:   "todo",
					Priority: "normal",
					Created:  "2026-01-01T09:00:00Z",
				}),
				"b-second.md": taskDoc(taskFields{
					ID:       "dup001",
					Type:     "task",
					Title:    "Second",
					Status:   "todo",
					Priority: "critical",
					Created:  "2026-01-01T08:00:00Z",
				}),
			},
		},
		{
			name: "symmetric_dupes",
			files: map[string]string{
				"a.md": taskDoc(taskFields{
					ID:       "a00001",
					Type:     "task",
					Title:    "A",
					Status:   "todo",
					Priority: "normal",
					Relations: map[string][]string{
						"relates": {"b00001"},
					},
					Created: "2026-01-01T09:00:00Z",
				}),
				"b.md": taskDoc(taskFields{
					ID:       "b00001",
					Type:     "task",
					Title:    "B",
					Status:   "todo",
					Priority: "normal",
					Relations: map[string][]string{
						"relates": {"a00001"},
					},
					Created: "2026-01-01T10:00:00Z",
				}),
				"c.md": taskDoc(taskFields{
					ID:       "c00001",
					Type:     "task",
					Title:    "C",
					Status:   "todo",
					Priority: "normal",
					Relations: map[string][]string{
						"blocks": {"d00001"},
					},
					Created: "2026-01-01T11:00:00Z",
				}),
				"d.md": taskDoc(taskFields{
					ID:       "d00001",
					Type:     "task",
					Title:    "D",
					Status:   "todo",
					Priority: "normal",
					Relations: map[string][]string{
						"depends_on": {"c00001"},
					},
					Created: "2026-01-01T12:00:00Z",
				}),
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			idx := loadFixture(t, tt.files)
			assertGolden(t, tt.name+".golden.json", snapshotIndex(idx))
		})
	}
}

func TestQueryFiltersSortAndLimit(t *testing.T) {
	t.Parallel()

	idx := loadFixture(t, map[string]string{
		"a.md": taskDoc(taskFields{ID: "a00001", Type: "task", Title: "A", Status: "todo", Priority: "low", Labels: []string{"cli"}, Assignee: "codex", Parent: "epic01", Created: "2026-01-01T09:00:00Z"}),
		"b.md": taskDoc(taskFields{ID: "b00001", Type: "bug", Title: "B", Status: "todo", Priority: "critical", Labels: []string{"backend"}, Assignee: "codex", Parent: "epic01", Created: "2026-01-01T11:00:00Z"}),
		"c.md": taskDoc(taskFields{ID: "c00001", Type: "task", Title: "C", Status: "done", Priority: "high", Labels: []string{"cli", "backend"}, Assignee: "claude", Parent: "epic02", Created: "2026-01-01T08:00:00Z", Archived: true}),
		"d.md": taskDoc(taskFields{ID: "d00001", Type: "task", Title: "D", Status: "todo", Priority: "high", Labels: []string{"cli"}, Assignee: "codex", Parent: "epic01", Created: "2026-01-01T07:00:00Z"}),
	})

	tests := []struct {
		name string
		got  []*taskView
		want []string
	}{
		{name: "all sorted", got: viewTasks(idx.All()), want: []string{"b00001", "d00001", "c00001", "a00001"}},
		{name: "by type", got: viewTasks(idx.ByType("task")), want: []string{"d00001", "c00001", "a00001"}},
		{name: "by status", got: viewTasks(idx.ByStatus("todo")), want: []string{"b00001", "d00001", "a00001"}},
		{name: "by label", got: viewTasks(idx.ByLabel("cli")), want: []string{"d00001", "c00001", "a00001"}},
		{name: "by parent", got: viewTasks(idx.ByParent("epic01")), want: []string{"b00001", "d00001", "a00001"}},
		{name: "by assignee", got: viewTasks(idx.ByAssignee("codex")), want: []string{"b00001", "d00001", "a00001"}},
		{name: "archived", got: viewTasks(idx.Archived(true)), want: []string{"c00001"}},
		{name: "composed limit", got: viewTasks(idx.Query(Query{Types: []string{"task"}, Labels: []string{"cli"}, Archived: boolPtr(false), Limit: 1})), want: []string{"d00001"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := taskViewIDs(tt.got); strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("ids = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()

	idx := loadFixture(t, map[string]string{
		"a.md": taskDoc(taskFields{ID: "abc123", Type: "task", Title: "ABC", Status: "todo", Priority: "normal", Created: "2026-01-01T09:00:00Z"}),
		"b.md": taskDoc(taskFields{ID: "abc999", Type: "bug", Title: "ABZ", Status: "impl", Priority: "high", Created: "2026-01-01T10:00:00Z"}),
		"c.md": taskDoc(taskFields{ID: "def123", Type: "task", Title: "DEF", Status: "done", Priority: "low", Created: "2026-01-01T11:00:00Z"}),
	})

	t.Run("exact wins", func(t *testing.T) {
		t.Parallel()
		got, err := idx.Resolve("abc123")
		if err != nil {
			t.Fatalf("Resolve exact: %v", err)
		}
		if got.ID != "abc123" {
			t.Fatalf("id = %q", got.ID)
		}
	})
	t.Run("unique prefix", func(t *testing.T) {
		t.Parallel()
		got, err := idx.Resolve("def")
		if err != nil {
			t.Fatalf("Resolve prefix: %v", err)
		}
		if got.ID != "def123" {
			t.Fatalf("id = %q", got.ID)
		}
	})
	t.Run("ambiguous sorted candidates", func(t *testing.T) {
		t.Parallel()
		_, err := idx.Resolve("abc")
		ambiguous, ok := err.(syaerr.Ambiguous)
		if !ok {
			t.Fatalf("error = %T %v, want Ambiguous", err, err)
		}
		got := []string{ambiguous.Candidates[0].ID, ambiguous.Candidates[1].ID}
		want := []string{"abc123", "abc999"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("candidates = %v, want %v", got, want)
		}
	})
	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		_, err := idx.Resolve("zzz")
		if _, ok := err.(syaerr.NotFound); !ok {
			t.Fatalf("error = %T %v, want NotFound", err, err)
		}
	})
}

func TestReverseEdgesHaveCanonicalOrigins(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewSource(42))
	files := make(map[string]string)
	for n := 0; n < 350; n++ {
		id := fmt.Sprintf("%06x", n+1)
		fields := taskFields{
			ID:       id,
			Type:     "task",
			Title:    "Task " + id,
			Status:   "todo",
			Priority: "normal",
			Created:  time.Date(2026, 1, 1, 0, n%60, 0, 0, time.UTC).Format(time.RFC3339),
		}
		if n > 0 && rng.Intn(5) == 0 {
			fields.Parent = fmt.Sprintf("%06x", rng.Intn(n)+1)
		}
		if n > 0 {
			relations := make(map[string][]string)
			for edge := 0; edge < rng.Intn(4); edge++ {
				target := fmt.Sprintf("%06x", rng.Intn(n)+1)
				switch rng.Intn(3) {
				case 0:
					relations["depends_on"] = append(relations["depends_on"], target)
				case 1:
					relations["duplicates"] = append(relations["duplicates"], target)
				case 2:
					relations["relates"] = append(relations["relates"], target)
				}
			}
			fields.Relations = relations
		}
		files[id+".md"] = taskDoc(fields)
	}

	idx := loadFixture(t, files)
	origins := idx.CanonicalOrigins()
	for edge, edgeOrigins := range origins {
		if len(edgeOrigins) != 1 {
			t.Fatalf("edge %+v has %d origins, want 1", edge, len(edgeOrigins))
		}
	}
	for id, relations := range idx.ReverseEdges() {
		for relation, targets := range relations {
			if relation == childrenRelation {
				continue
			}
			for _, target := range targets {
				edge := reverseCanonical(id, relation, target)
				if got := len(origins[edge]); got != 1 {
					t.Fatalf("reverse %s %s %s maps to %+v with %d origins, want 1", id, relation, target, edge, got)
				}
			}
		}
	}
}

func BenchmarkLoad5k(b *testing.B) {
	root := b.TempDir()
	for n := 0; n < 5000; n++ {
		id := fmt.Sprintf("%06x", n+1)
		fields := taskFields{
			ID:       id,
			Type:     "task",
			Title:    "Task " + id,
			Status:   "todo",
			Priority: "normal",
			Created:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(n) * time.Second).Format(time.RFC3339),
		}
		if n > 0 {
			fields.Relations = map[string][]string{"depends_on": {fmt.Sprintf("%06x", n)}}
		}
		writeTaskFile(b, root, id+".md", taskDoc(fields))
	}
	fsys := os.DirFS(root)
	sch := testSchema()

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		idx, err := Load(fsys, ".sya", sch)
		if err != nil {
			b.Fatalf("Load: %v", err)
		}
		if got := len(idx.All()); got != 5000 {
			b.Fatalf("loaded %d tasks, want 5000", got)
		}
	}
}

func loadFixture(t testing.TB, files map[string]string) *Index {
	t.Helper()
	root := t.TempDir()
	for name, contents := range files {
		writeTaskFile(t, root, name, contents)
	}
	idx, err := Load(os.DirFS(root), ".sya", testSchema())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return idx
}

func writeTaskFile(t testing.TB, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, ".sya", "tasks", filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write task: %v", err)
	}
}

func testSchema() *schema.Schema {
	return &schema.Schema{
		Relations: map[string]schema.RelationDef{
			"depends_on": {Reverse: "blocks", Blocking: true},
			"duplicates": {Reverse: "duplicated_by"},
			"relates":    {Symmetric: true},
		},
		Types: map[string]schema.TypeDef{
			"epic":    {Container: true, Children: []string{"feature", "task", "bug", "note"}},
			"feature": {},
			"task":    {},
			"bug":     {},
			"note":    {},
		},
	}
}

type taskFields struct {
	ID        string
	Type      string
	Title     string
	Status    string
	Priority  string
	Parent    string
	Assignee  string
	Labels    []string
	Relations map[string][]string
	Created   string
	Archived  bool
}

func taskDoc(fields taskFields) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", fields.ID)
	fmt.Fprintf(&b, "type: %s\n", fields.Type)
	fmt.Fprintf(&b, "title: %q\n", fields.Title)
	fmt.Fprintf(&b, "status: %s\n", fields.Status)
	fmt.Fprintf(&b, "priority: %s\n", fields.Priority)
	if fields.Parent != "" {
		fmt.Fprintf(&b, "parent: %s\n", fields.Parent)
	}
	if fields.Assignee != "" {
		fmt.Fprintf(&b, "assignee: %s\n", fields.Assignee)
	}
	if len(fields.Labels) > 0 {
		fmt.Fprintf(&b, "labels: [%s]\n", strings.Join(fields.Labels, ", "))
	}
	if len(fields.Relations) > 0 {
		b.WriteString("relations:\n")
		for _, relation := range sortedMapKeys(fields.Relations) {
			fmt.Fprintf(&b, "  %s: [%s]\n", relation, strings.Join(fields.Relations[relation], ", "))
		}
	}
	fmt.Fprintf(&b, "created: %s\n", fields.Created)
	b.WriteString("schema_version: 1\n")
	if fields.Archived {
		b.WriteString("archived: true\n")
	}
	b.WriteString("---\n\n## Description\nFixture task.\n")
	return b.String()
}

type indexSnapshot struct {
	Tasks      []string            `json:"tasks"`
	Reverse    ReverseEdges        `json:"reverse,omitempty"`
	Quarantine []QuarantinedFile   `json:"quarantine,omitempty"`
	Warnings   []Warning           `json:"warnings,omitempty"`
	Origins    []CanonicalEdge     `json:"origins,omitempty"`
	Related    map[string][]string `json:"related,omitempty"`
}

func snapshotIndex(idx *Index) indexSnapshot {
	tasks := idx.All()
	taskIDs := make([]string, 0, len(tasks))
	for _, t := range tasks {
		taskIDs = append(taskIDs, t.ID)
	}
	originMap := idx.CanonicalOrigins()
	origins := make([]CanonicalEdge, 0, len(originMap))
	for edge := range originMap {
		origins = append(origins, edge)
	}
	sort.Slice(origins, func(a, b int) bool {
		if origins[a].From != origins[b].From {
			return origins[a].From < origins[b].From
		}
		if origins[a].Relation != origins[b].Relation {
			return origins[a].Relation < origins[b].Relation
		}
		return origins[a].To < origins[b].To
	})
	related := make(map[string][]string)
	for _, id := range []string{"a00001", "b00001", "c00001", "d00001"} {
		for _, relation := range []string{"depends_on", "blocks", "relates"} {
			values := idx.Related(id, relation)
			if len(values) > 0 {
				related[id+"."+relation] = values
			}
		}
	}
	if len(related) == 0 {
		related = nil
	}
	return indexSnapshot{
		Tasks:      taskIDs,
		Reverse:    idx.ReverseEdges(),
		Quarantine: idx.Quarantined(),
		Warnings:   idx.Warnings(),
		Origins:    origins,
		Related:    related,
	}
}

func assertGolden(t *testing.T, name string, value any) {
	t.Helper()
	gotBytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	got := strings.TrimSpace(string(gotBytes))
	wantBytes, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	want := strings.TrimSpace(string(wantBytes))
	if got != want {
		t.Fatalf("golden mismatch for %s\nwant:\n%s\n\ngot:\n%s", name, want, got)
	}
}

type taskView struct {
	ID string
}

func viewTasks(tasks []*task.Task) []*taskView {
	views := make([]*taskView, 0, len(tasks))
	for _, t := range tasks {
		views = append(views, &taskView{ID: t.ID})
	}
	return views
}

func taskViewIDs(tasks []*taskView) []string {
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}
	return ids
}

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func boolPtr(value bool) *bool {
	return &value
}

func reverseCanonical(id, relation, target string) CanonicalEdge {
	switch relation {
	case "blocks":
		return CanonicalEdge{From: target, Relation: "depends_on", To: id}
	case "duplicated_by":
		return CanonicalEdge{From: target, Relation: "duplicates", To: id}
	case "relates":
		from, to := id, target
		if to < from {
			from, to = to, from
		}
		return CanonicalEdge{From: from, Relation: "relates", To: to}
	default:
		return CanonicalEdge{From: target, Relation: relation, To: id}
	}
}
