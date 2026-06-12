package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/task"
)

func TestFullPipelineProperty(t *testing.T) {
	t.Parallel()

	for seed := 0; seed < 500; seed++ {
		root := t.TempDir()
		writePipelinePropertyProject(t, root, seed)
		app := New(Options{
			WorkDir: root,
			Env:     func(string) string { return "" },
			GitUser: func(context.Context) (string, error) { return "", fmt.Errorf("no git user") },
			Now:     func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		})
		state, err := app.loadProject()
		if err != nil {
			t.Fatalf("seed %d loadProject: %v", seed, err)
		}
		resolver := state.Index.Resolver()
		for _, candidate := range state.Index.All() {
			view, ok := resolver.Get(candidate.ID)
			if !ok {
				t.Fatalf("seed %d resolver missing %s", seed, candidate.ID)
			}
			for _, available := range schema.AvailableTransitions(state.Schema, resolver, view) {
				violations := schema.Evaluate(state.Schema, resolver, view, available.Transition)
				if available.Passing != (len(violations) == 0) {
					t.Fatalf("seed %d %s transition %#v passing=%v violations=%#v", seed, candidate.ID, available.Transition, available.Passing, violations)
				}
				if !available.Passing {
					continue
				}
				result := app.moveOne(state, candidate.ID, available.Transition.To, "", false, mutationOptions{})
				if !result.OK {
					t.Fatalf("seed %d passing transition rejected by move logic: %#v", seed, result)
				}
			}
		}
	}
}

func writePipelinePropertyProject(t *testing.T, root string, seed int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".sya", "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".sya", "schema.yml"), []byte(pipelinePropertySchemaYAML()), 0o644); err != nil {
		t.Fatal(err)
	}
	count := 3 + seed%5
	for i := 0; i < count; i++ {
		status := []string{"todo", "doing", "review", "parked", "done"}[(seed+i)%5]
		relations := map[string][]string{}
		if i > 0 && (seed+i)%2 == 0 {
			relations["depends_on"] = []string{fmt.Sprintf("p%05d", i-1)}
		}
		taskObj := &task.Task{
			ID:            fmt.Sprintf("p%05d", i),
			Type:          "task",
			Title:         fmt.Sprintf("Property %d/%d", seed, i),
			Status:        status,
			Priority:      []string{"low", "normal", "high", "critical"}[(seed+i)%4],
			Relations:     relations,
			Fields:        map[string]any{},
			Created:       time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC),
			SchemaVersion: 1,
			Body: task.Body{Sections: []task.Section{
				{Name: "Description", Raw: []byte("## Description\nGenerated.\n")},
			}},
		}
		taskObj.Body.Raw = []byte("## Description\nGenerated.\n")
		data, err := task.Serialize(taskObj)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ".sya", "tasks", taskObj.ID+".md"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func pipelinePropertySchemaYAML() string {
	return `
schema_version: 1
defaults: {type: task}
relations:
  depends_on:
    reverse: blocks
    blocking: true
types:
  task:
    pipeline: [todo, doing, review, parked, done, scrapped]
    terminal: [done, scrapped]
    working: [doing, review]
    parked: [parked]
    transitions:
      todo -> doing: {}
      doing -> review:
        ignore_blocking: [depends_on]
      review -> done: {}
      review -> doing:
        kind: setback
        ignore_blocking: [depends_on]
      "* -> scrapped": {}
`
}
