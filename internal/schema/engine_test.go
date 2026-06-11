package schema

import (
	"fmt"
	"testing"
)

func TestEvaluateGuardTruthTables(t *testing.T) {
	t.Parallel()

	schema := engineSchema()
	cases := []struct {
		name      string
		task      engineTask
		tasks     map[string]engineTask
		guard     Guard
		wantFail  bool
		wantKind  string
		wantField string
	}{
		{
			name: "relation_status pass",
			task: engineTask{id: "a", typ: "task", status: "todo", relations: map[string][]string{
				"relates": {"b"},
			}},
			tasks: map[string]engineTask{"b": {id: "b", typ: "task", status: "done"}},
			guard: relationStatusGuard("relates", []string{"done"}),
		},
		{
			name: "relation_status fail",
			task: engineTask{id: "a", typ: "task", status: "todo", relations: map[string][]string{
				"relates": {"b"},
			}},
			tasks:    map[string]engineTask{"b": {id: "b", typ: "task", status: "todo"}},
			guard:    relationStatusGuard("relates", []string{"done"}),
			wantFail: true,
			wantKind: string(GuardRelationStatus),
		},
		{
			name:  "relation_status vacuous",
			task:  engineTask{id: "a", typ: "task", status: "todo"},
			guard: relationStatusGuard("relates", []string{"done"}),
		},
		{
			name: "relation_status cross-type terminal",
			task: engineTask{id: "a", typ: "task", status: "todo", relations: map[string][]string{
				"relates": {"d"},
			}},
			tasks: map[string]engineTask{"d": {id: "d", typ: "doc", status: "closed"}},
			guard: relationStatusGuard("relates", []string{"terminal"}),
		},
		{
			name: "relation_status archived target counts terminal",
			task: engineTask{id: "a", typ: "task", status: "todo", relations: map[string][]string{
				"relates": {"b"},
			}},
			tasks: map[string]engineTask{"b": {id: "b", typ: "task", status: "todo", archived: true}},
			guard: relationStatusGuard("relates", []string{"terminal"}),
		},
		{
			name: "relation_exists pass",
			task: engineTask{id: "a", typ: "task", status: "todo", relations: map[string][]string{
				"relates": {"b"},
			}},
			tasks: map[string]engineTask{"b": {id: "b", typ: "task", status: "todo"}},
			guard: relationExistsGuard("relates"),
		},
		{
			name:     "relation_exists fail",
			task:     engineTask{id: "a", typ: "task", status: "todo"},
			guard:    relationExistsGuard("relates"),
			wantFail: true,
			wantKind: string(GuardRelationExists),
		},
		{
			name:  "field equals pass",
			task:  engineTask{id: "a", typ: "task", status: "todo", fields: map[string]any{"ready": true}},
			guard: fieldEqualsGuard("ready", true),
		},
		{
			name:      "field equals fail",
			task:      engineTask{id: "a", typ: "task", status: "todo", fields: map[string]any{"ready": false}},
			guard:     fieldEqualsGuard("ready", true),
			wantFail:  true,
			wantKind:  string(GuardField),
			wantField: "ready",
		},
		{
			name:  "field in pass",
			task:  engineTask{id: "a", typ: "task", status: "todo", fields: map[string]any{"severity": "major"}},
			guard: fieldInGuard("severity", []any{"critical", "major"}),
		},
		{
			name:     "field in fail",
			task:     engineTask{id: "a", typ: "task", status: "todo", fields: map[string]any{"severity": "minor"}},
			guard:    fieldInGuard("severity", []any{"critical", "major"}),
			wantFail: true,
			wantKind: string(GuardField),
		},
		{
			name:  "children_status pass",
			task:  engineTask{id: "a", typ: "task", status: "todo", children: []string{"b"}},
			tasks: map[string]engineTask{"b": {id: "b", typ: "task", status: "done"}},
			guard: childrenStatusGuard([]string{"done"}),
		},
		{
			name:     "children_status fail",
			task:     engineTask{id: "a", typ: "task", status: "todo", children: []string{"b"}},
			tasks:    map[string]engineTask{"b": {id: "b", typ: "task", status: "todo"}},
			guard:    childrenStatusGuard([]string{"done"}),
			wantFail: true,
			wantKind: string(GuardChildrenStatus),
		},
		{
			name:  "children_status vacuous",
			task:  engineTask{id: "a", typ: "task", status: "todo"},
			guard: childrenStatusGuard([]string{"done"}),
		},
		{
			name:  "children_status cross-type terminal",
			task:  engineTask{id: "a", typ: "task", status: "todo", children: []string{"d"}},
			tasks: map[string]engineTask{"d": {id: "d", typ: "doc", status: "closed"}},
			guard: childrenStatusGuard([]string{"terminal"}),
		},
		{
			name:  "children_status archived target counts terminal",
			task:  engineTask{id: "a", typ: "task", status: "todo", children: []string{"b"}},
			tasks: map[string]engineTask{"b": {id: "b", typ: "task", status: "todo", archived: true}},
			guard: childrenStatusGuard([]string{"terminal"}),
		},
		{
			name:  "parent_status pass",
			task:  engineTask{id: "a", typ: "task", status: "todo", parent: "p", hasParent: true},
			tasks: map[string]engineTask{"p": {id: "p", typ: "task", status: "done"}},
			guard: parentStatusGuard([]string{"done"}),
		},
		{
			name:     "parent_status fail",
			task:     engineTask{id: "a", typ: "task", status: "todo", parent: "p", hasParent: true},
			tasks:    map[string]engineTask{"p": {id: "p", typ: "task", status: "todo"}},
			guard:    parentStatusGuard([]string{"done"}),
			wantFail: true,
			wantKind: string(GuardParentStatus),
		},
		{
			name:     "parent_status missing parent fails",
			task:     engineTask{id: "a", typ: "task", status: "todo"},
			guard:    parentStatusGuard([]string{"done"}),
			wantFail: true,
			wantKind: string(GuardParentStatus),
		},
		{
			name:  "parent_status cross-type terminal",
			task:  engineTask{id: "a", typ: "task", status: "todo", parent: "d", hasParent: true},
			tasks: map[string]engineTask{"d": {id: "d", typ: "doc", status: "closed"}},
			guard: parentStatusGuard([]string{"terminal"}),
		},
		{
			name:  "parent_status archived target counts terminal",
			task:  engineTask{id: "a", typ: "task", status: "todo", parent: "p", hasParent: true},
			tasks: map[string]engineTask{"p": {id: "p", typ: "task", status: "todo", archived: true}},
			guard: parentStatusGuard([]string{"terminal"}),
		},
		{
			name:  "section_nonempty pass",
			task:  engineTask{id: "a", typ: "task", status: "todo", sections: map[string]bool{"Design": true}},
			guard: sectionGuard("Design"),
		},
		{
			name:     "section_nonempty fail",
			task:     engineTask{id: "a", typ: "task", status: "todo"},
			guard:    sectionGuard("Design"),
			wantFail: true,
			wantKind: string(GuardSectionNonempty),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tasks := make(map[string]engineTask, len(tc.tasks)+1)
			for id, task := range tc.tasks {
				tasks[id] = task
			}
			resolver := engineResolver{tasks: tasks}
			resolver.tasks[tc.task.id] = tc.task
			transition := Transition{From: "todo", To: "in_progress", Guards: []Guard{tc.guard}}
			violations := Evaluate(schema, resolver, tc.task, transition)
			if gotFail := len(violations) > 0; gotFail != tc.wantFail {
				t.Fatalf("failed=%v, want %v; violations=%#v", gotFail, tc.wantFail, violations)
			}
			if !tc.wantFail {
				return
			}
			violation := violations[0]
			if violation.Kind != tc.wantKind {
				t.Fatalf("violation kind = %q, want %q", violation.Kind, tc.wantKind)
			}
			if violation.Message != "blocked message" || violation.Hint != "blocked hint" {
				t.Fatalf("message/hint = %q/%q", violation.Message, violation.Hint)
			}
			if tc.wantField != "" && violation.Field != tc.wantField {
				t.Fatalf("field = %q, want %q", violation.Field, tc.wantField)
			}
		})
	}
}

func TestImplicitBlockingMatrix(t *testing.T) {
	t.Parallel()

	schema := engineSchema()
	task := engineTask{
		id:     "a",
		typ:    "task",
		status: "todo",
		relations: map[string][]string{
			"depends_on": {"b"},
		},
	}
	resolver := engineResolver{tasks: map[string]engineTask{
		"a": task,
		"b": {id: "b", typ: "task", status: "todo"},
	}}
	cases := []struct {
		name        string
		to          string
		ignore      []string
		wantBlocked bool
	}{
		{name: "working target blocks", to: "in_progress", wantBlocked: true},
		{name: "terminal target blocks", to: "done", wantBlocked: true},
		{name: "other target does not block", to: "todo"},
		{name: "ignore blocking opt out", to: "in_progress", ignore: []string{"depends_on"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			transition := Transition{From: "todo", To: tc.to, IgnoreBlocking: tc.ignore}
			violations := Evaluate(schema, resolver, task, transition)
			if got := hasEngineViolation(violations, "blocking_relation"); got != tc.wantBlocked {
				t.Fatalf("blocking violation = %v, want %v; violations=%#v", got, tc.wantBlocked, violations)
			}
		})
	}

	t.Run("archived dependency counts terminal", func(t *testing.T) {
		t.Parallel()
		archivedResolver := engineResolver{tasks: map[string]engineTask{
			"a": task,
			"b": {id: "b", typ: "task", status: "todo", archived: true},
		}}
		violations := Evaluate(schema, archivedResolver, task, Transition{From: "todo", To: "done"})
		if hasEngineViolation(violations, "blocking_relation") {
			t.Fatalf("archived target should satisfy implicit blocking guard: %#v", violations)
		}
	})
}

func TestAvailableReadyBlockedSemantics(t *testing.T) {
	t.Parallel()

	schema := engineSchema()
	t.Run("available includes wildcard, ready candidates do not", func(t *testing.T) {
		t.Parallel()
		task := engineTask{id: "a", typ: "task", status: "todo"}
		resolver := engineResolver{tasks: map[string]engineTask{"a": task}}
		available := AvailableTransitions(schema, resolver, task)
		if got := transitionList(available); fmt.Sprint(got) != "[todo -> in_progress todo -> scrapped]" {
			t.Fatalf("available transitions = %v", got)
		}
		if !Ready(schema, resolver, task) {
			t.Fatal("task with passing explicit candidate should be ready")
		}
		blocked := Blocked(schema, resolver, task)
		if blocked.Blocked {
			t.Fatalf("ready task reported blocked: %#v", blocked)
		}
	})

	t.Run("blocked with candidate violations", func(t *testing.T) {
		t.Parallel()
		task := engineTask{id: "a", typ: "task", status: "in_progress", relations: map[string][]string{
			"depends_on": {"b"},
		}}
		resolver := engineResolver{tasks: map[string]engineTask{
			"a": task,
			"b": {id: "b", typ: "task", status: "todo"},
		}}
		if Ready(schema, resolver, task) {
			t.Fatal("task with failing candidate should not be ready")
		}
		blocked := Blocked(schema, resolver, task)
		if !blocked.Blocked || blocked.DeadEnd || len(blocked.Transitions) == 0 {
			t.Fatalf("blocked status = %#v, want blocked candidate reasons", blocked)
		}
	})

	t.Run("blocked dead end", func(t *testing.T) {
		t.Parallel()
		task := engineTask{id: "a", typ: "task", status: "review"}
		resolver := engineResolver{tasks: map[string]engineTask{"a": task}}
		blocked := Blocked(schema, resolver, task)
		if !blocked.Blocked || !blocked.DeadEnd {
			t.Fatalf("blocked status = %#v, want dead-end", blocked)
		}
	})

	t.Run("terminal and parked excluded", func(t *testing.T) {
		t.Parallel()
		for _, status := range []string{"done", "parked"} {
			task := engineTask{id: status, typ: "task", status: status}
			resolver := engineResolver{tasks: map[string]engineTask{status: task}}
			if Ready(schema, resolver, task) {
				t.Fatalf("%s task should not be ready", status)
			}
			if Blocked(schema, resolver, task).Blocked {
				t.Fatalf("%s task should not be blocked", status)
			}
		}
	})
}

func engineSchema() *Schema {
	return &Schema{
		Relations: map[string]RelationDef{
			"depends_on": {Blocking: true, From: []string{"*"}, To: []string{"*"}},
			"relates":    {From: []string{"*"}, To: []string{"*"}},
		},
		Types: map[string]TypeDef{
			"task": {
				Pipeline: []string{"todo", "in_progress", "review", "parked", "done", "scrapped"},
				Terminal: []string{"done", "scrapped"},
				Working:  []string{"in_progress"},
				Parked:   []string{"parked"},
				Transitions: map[string]Transition{
					"todo -> in_progress": {From: "todo", To: "in_progress"},
					"in_progress -> done": {From: "in_progress", To: "done"},
					"* -> scrapped":       {From: "*", To: "scrapped", Kind: TransitionSetback},
				},
			},
			"doc": {
				Pipeline: []string{"draft", "closed"},
				Terminal: []string{"closed"},
				Working:  []string{"draft"},
				Transitions: map[string]Transition{
					"draft -> closed": {From: "draft", To: "closed"},
				},
			},
		},
	}
}

func relationStatusGuard(relation string, statuses []string) Guard {
	return Guard{
		Kind:    GuardRelationStatus,
		Params:  map[string]any{"relation": relation, "in": statuses},
		Message: "blocked message",
		Hint:    "blocked hint",
	}
}

func relationExistsGuard(relation string) Guard {
	return Guard{
		Kind:    GuardRelationExists,
		Params:  map[string]any{"relation": relation},
		Message: "blocked message",
		Hint:    "blocked hint",
	}
}

func fieldEqualsGuard(field string, value any) Guard {
	return Guard{
		Kind:    GuardField,
		Params:  map[string]any{"field": field, "equals": value},
		Message: "blocked message",
		Hint:    "blocked hint",
	}
}

func fieldInGuard(field string, values []any) Guard {
	return Guard{
		Kind:    GuardField,
		Params:  map[string]any{"field": field, "in": values},
		Message: "blocked message",
		Hint:    "blocked hint",
	}
}

func childrenStatusGuard(statuses []string) Guard {
	return Guard{
		Kind:    GuardChildrenStatus,
		Params:  map[string]any{"in": statuses},
		Message: "blocked message",
		Hint:    "blocked hint",
	}
}

func parentStatusGuard(statuses []string) Guard {
	return Guard{
		Kind:    GuardParentStatus,
		Params:  map[string]any{"in": statuses},
		Message: "blocked message",
		Hint:    "blocked hint",
	}
}

func sectionGuard(section string) Guard {
	return Guard{
		Kind:    GuardSectionNonempty,
		Params:  map[string]any{"section": section},
		Message: "blocked message",
		Hint:    "blocked hint",
	}
}

func hasEngineViolation(violations []Violation, kind string) bool {
	for _, violation := range violations {
		if violation.Kind == kind {
			return true
		}
	}
	return false
}

func transitionList(statuses []TransitionStatus) []string {
	out := make([]string, 0, len(statuses))
	for _, status := range statuses {
		out = append(out, transitionKey(status.Transition.From, status.Transition.To))
	}
	return out
}

type engineResolver struct {
	tasks map[string]engineTask
}

func (r engineResolver) Get(id string) (TaskView, bool) {
	task, ok := r.tasks[id]
	if !ok {
		return nil, false
	}
	return task, true
}

type engineTask struct {
	id        string
	typ       string
	status    string
	relations map[string][]string
	children  []string
	parent    string
	hasParent bool
	fields    map[string]any
	sections  map[string]bool
	archived  bool
}

func (t engineTask) Status() string { return t.status }
func (t engineTask) Type() string   { return t.typ }

func (t engineTask) Relations(name string) []string {
	return append([]string(nil), t.relations[name]...)
}

func (t engineTask) Children() []string {
	return append([]string(nil), t.children...)
}

func (t engineTask) Parent() (string, bool) {
	return t.parent, t.hasParent
}

func (t engineTask) Field(name string) (any, bool) {
	value, ok := t.fields[name]
	return value, ok
}

func (t engineTask) SectionNonEmpty(name string) bool {
	return t.sections[name]
}

func (t engineTask) Archived() bool {
	return t.archived
}
