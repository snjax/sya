package index

import (
	"os"
	"testing"
	"time"

	"github.com/snjax/sya/internal/schema"
)

func TestSchemaEngineOverLoadedIndex(t *testing.T) {
	t.Parallel()

	t.Run("blocking dependency chain", func(t *testing.T) {
		t.Parallel()
		idx := loadEngineFixture(t, map[string]string{
			"a.md": engineTaskDoc("a00001", "todo", false, map[string][]string{"depends_on": {"b00001"}}),
			"b.md": engineTaskDoc("b00001", "todo", false, nil),
		})
		resolver := idx.Resolver()
		taskView := mustTaskView(t, resolver, "a00001")
		if schema.Ready(engineIntegrationSchema(), resolver, taskView) {
			t.Fatal("task with open blocking dependency should not be ready")
		}
		blocked := schema.Blocked(engineIntegrationSchema(), resolver, taskView)
		if !blocked.Blocked || len(blocked.Transitions) == 0 {
			t.Fatalf("blocked = %#v, want transition reasons", blocked)
		}
		if !hasIndexEngineViolation(blocked.Transitions[0].Violations, "blocking_relation") {
			t.Fatalf("violations = %#v, want blocking_relation", blocked.Transitions[0].Violations)
		}
	})

	t.Run("ignore blocking backtrack", func(t *testing.T) {
		t.Parallel()
		idx := loadEngineFixture(t, map[string]string{
			"a.md": engineTaskDoc("a00001", "doing", false, map[string][]string{"depends_on": {"b00001"}}),
			"b.md": engineTaskDoc("b00001", "todo", false, nil),
		})
		resolver := idx.Resolver()
		taskView := mustTaskView(t, resolver, "a00001")
		available := schema.AvailableTransitions(engineIntegrationSchema(), resolver, taskView)
		var found bool
		for _, transition := range available {
			if transition.Transition.To != "review" {
				continue
			}
			found = true
			if !transition.Passing {
				t.Fatalf("ignore_blocking transition should pass: %#v", transition)
			}
		}
		if !found {
			t.Fatalf("available transitions = %#v, missing doing -> review", available)
		}
	})

	t.Run("archived target counts terminal", func(t *testing.T) {
		t.Parallel()
		idx := loadEngineFixture(t, map[string]string{
			"a.md": engineTaskDoc("a00001", "todo", false, map[string][]string{"depends_on": {"b00001"}}),
			"b.md": engineTaskDoc("b00001", "todo", true, nil),
		})
		resolver := idx.Resolver()
		taskView := mustTaskView(t, resolver, "a00001")
		if !schema.Ready(engineIntegrationSchema(), resolver, taskView) {
			t.Fatal("archived dependency target should satisfy implicit terminal guard")
		}
	})

	t.Run("parked excluded", func(t *testing.T) {
		t.Parallel()
		idx := loadEngineFixture(t, map[string]string{
			"a.md": engineTaskDoc("a00001", "parked", false, nil),
		})
		resolver := idx.Resolver()
		taskView := mustTaskView(t, resolver, "a00001")
		if schema.Ready(engineIntegrationSchema(), resolver, taskView) {
			t.Fatal("parked task should not be ready")
		}
		if schema.Blocked(engineIntegrationSchema(), resolver, taskView).Blocked {
			t.Fatal("parked task should not be blocked")
		}
	})
}

func loadEngineFixture(t testing.TB, files map[string]string) *Index {
	t.Helper()
	root := t.TempDir()
	for name, contents := range files {
		writeTaskFile(t, root, name, contents)
	}
	idx, err := Load(os.DirFS(root), ".sya", engineIntegrationSchema())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return idx
}

func engineIntegrationSchema() *schema.Schema {
	return &schema.Schema{
		Relations: map[string]schema.RelationDef{
			"depends_on": {Reverse: "blocks", Blocking: true, From: []string{"*"}, To: []string{"*"}},
		},
		Types: map[string]schema.TypeDef{
			"task": {
				Pipeline: []string{"todo", "doing", "review", "parked", "done", "scrapped"},
				Terminal: []string{"done", "scrapped"},
				Working:  []string{"doing", "review"},
				Parked:   []string{"parked"},
				Transitions: map[string]schema.Transition{
					"todo -> doing": {From: "todo", To: "doing"},
					"doing -> done": {From: "doing", To: "done"},
					"doing -> review": {
						From:           "doing",
						To:             "review",
						IgnoreBlocking: []string{"depends_on"},
					},
					"review -> doing": {From: "review", To: "doing"},
					"* -> scrapped":   {From: "*", To: "scrapped", Kind: schema.TransitionSetback},
				},
			},
		},
	}
}

func engineTaskDoc(id, status string, archived bool, relations map[string][]string) string {
	return taskDoc(taskFields{
		ID:        id,
		Type:      "task",
		Title:     id,
		Status:    status,
		Priority:  "normal",
		Relations: relations,
		Created:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Archived:  archived,
	})
}

func mustTaskView(t testing.TB, resolver schema.Resolver, id string) schema.TaskView {
	t.Helper()
	taskView, ok := resolver.Get(id)
	if !ok {
		t.Fatalf("resolver missing task %s", id)
	}
	return taskView
}

func hasIndexEngineViolation(violations []schema.Violation, kind string) bool {
	for _, violation := range violations {
		if violation.Kind == kind {
			return true
		}
	}
	return false
}
