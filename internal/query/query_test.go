package query

import (
	"strings"
	"testing"
	"time"
)

func TestParseTable(t *testing.T) {
	t.Parallel()

	valid := []string{
		`id=abc123`,
		`type in (task,bug)`,
		`status!=done`,
		`title~"api gateway"`,
		`priority>low`,
		`priority>=normal`,
		`assignee=""`,
		`parent!=epic1`,
		`label=backend`,
		`field.ready=true`,
		`rel.depends_on`,
		`rel.depends_on=abc123`,
		`age>=7d`,
		`age<1w`,
		`age<=2w`,
		`ready and not (blocked or archived)`,
		`terminal or working or parked or dead_end`,
	}
	for _, input := range valid {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			expr, err := Parse(input)
			if err != nil {
				t.Fatalf("Parse(%q): %v", input, err)
			}
			if _, err := Parse(expr.String()); err != nil {
				t.Fatalf("Parse(String()) for %q -> %q: %v", input, expr.String(), err)
			}
		})
	}

	invalid := []struct {
		input string
		want  string
	}{
		{``, "predicate key"},
		{`ghost=1`, "known key"},
		{`type`, "operator"},
		{`type=`, "value"},
		{`type in ()`, "value"},
		{`(type=task`, "')'"},
		{`type=task garbage`, "end of expression"},
	}
	for _, tt := range invalid {
		tt := tt
		t.Run("invalid "+tt.input, func(t *testing.T) {
			t.Parallel()
			_, err := Parse(tt.input)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Parse(%q) err=%v, want %q", tt.input, err, tt.want)
			}
		})
	}
}

func TestPrecedence(t *testing.T) {
	t.Parallel()

	expr, err := Parse(`not type=bug and status=todo or archived`)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		task fakeTask
		want bool
	}{
		{task: fakeTask{typ: "task", status: "todo"}, want: true},
		{task: fakeTask{typ: "bug", status: "todo"}, want: false},
		{task: fakeTask{typ: "bug", status: "done", archived: true}, want: true},
	}
	for _, tt := range tests {
		if got := expr.Eval(tt.task, Options{}); got != tt.want {
			t.Fatalf("Eval(%#v) = %v, want %v", tt.task, got, tt.want)
		}
	}
}

func TestPredicateSemantics(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	task := fakeTask{
		id:       "abc123",
		typ:      "feature",
		status:   "spec",
		priority: "high",
		title:    "API Gateway",
		assignee: "codex",
		parent:   "epic1",
		labels:   []string{"backend", "api"},
		fields:   map[string]any{"ready": true, "size": "large"},
		relations: map[string][]string{
			"depends_on": {"dep1"},
		},
		created: now.Add(-8 * 24 * time.Hour),
		ready:   true,
		working: true,
	}
	tests := []struct {
		expr string
		want bool
	}{
		{`id=abc123`, true},
		{`type=bug`, false},
		{`status in (draft,spec)`, true},
		{`priority>normal`, true},
		{`title~gateway`, true},
		{`assignee=codex`, true},
		{`parent=epic1`, true},
		{`label=api`, true},
		{`field.ready=true`, true},
		{`field.size=small`, false},
		{`rel.depends_on`, true},
		{`rel.depends_on=dep1`, true},
		{`age>7d`, true},
		{`ready and working and not archived`, true},
		{`terminal or blocked or dead_end`, false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.expr, func(t *testing.T) {
			t.Parallel()
			pred, _, err := Compile(tt.expr, Options{Now: now})
			if err != nil {
				t.Fatal(err)
			}
			if got := pred(task); got != tt.want {
				t.Fatalf("%s = %v, want %v", tt.expr, got, tt.want)
			}
		})
	}
}

func TestArchivedReference(t *testing.T) {
	t.Parallel()

	expr, err := Parse(`type=task and archived`)
	if err != nil {
		t.Fatal(err)
	}
	if !expr.References("archived") {
		t.Fatalf("References(archived) = false")
	}
}

type fakeTask struct {
	id        string
	typ       string
	status    string
	priority  string
	title     string
	assignee  string
	parent    string
	labels    []string
	fields    map[string]any
	relations map[string][]string
	created   time.Time
	archived  bool
	terminal  bool
	working   bool
	parked    bool
	ready     bool
	blocked   bool
	deadEnd   bool
}

func (t fakeTask) ID() string                    { return t.id }
func (t fakeTask) Type() string                  { return t.typ }
func (t fakeTask) Status() string                { return t.status }
func (t fakeTask) Priority() string              { return t.priority }
func (t fakeTask) Title() string                 { return t.title }
func (t fakeTask) Assignee() string              { return t.assignee }
func (t fakeTask) Parent() string                { return t.parent }
func (t fakeTask) Labels() []string              { return append([]string(nil), t.labels...) }
func (t fakeTask) Field(name string) (any, bool) { v, ok := t.fields[name]; return v, ok }
func (t fakeTask) Relation(name string) []string { return append([]string(nil), t.relations[name]...) }
func (t fakeTask) Created() time.Time            { return t.created }
func (t fakeTask) Archived() bool                { return t.archived }
func (t fakeTask) Terminal() bool                { return t.terminal }
func (t fakeTask) Working() bool                 { return t.working }
func (t fakeTask) Parked() bool                  { return t.parked }
func (t fakeTask) Ready() bool                   { return t.ready }
func (t fakeTask) Blocked() bool                 { return t.blocked }
func (t fakeTask) DeadEnd() bool                 { return t.deadEnd }
