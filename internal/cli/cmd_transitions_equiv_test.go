package cli

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/snjax/sya/internal/schema"
)

func TestTransitionsCommandMatchesEngineRandomProjects(t *testing.T) {
	t.Parallel()

	for seed := int64(0); seed < 200; seed++ {
		seed := seed
		t.Run("seed", func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			buildTransitionEquivalenceProject(t, root, seed)
			app := transitionTestApp(root)
			state, err := app.loadProject()
			if err != nil {
				t.Fatalf("loadProject: %v", err)
			}
			ids := []string{"f00001", "d00001", "e00001"}
			id := ids[rand.New(rand.NewSource(seed)).Intn(len(ids))]
			got, err := app.runTransitions(id)
			if err != nil {
				t.Fatalf("runTransitions(%s): %v", id, err)
			}
			expected := expectedTransitionsFromEngine(t, state, id)
			gotJSON := mustJSON(t, got)
			expectedJSON := mustJSON(t, expected)
			if string(gotJSON) != string(expectedJSON) {
				t.Fatalf("transitions JSON mismatch for %s\nwant: %s\n got: %s", id, expectedJSON, gotJSON)
			}
		})
	}
}

func buildTransitionEquivalenceProject(t *testing.T, root string, seed int64) {
	t.Helper()
	initProject(t, root)
	createSeedTask(t, root, "e00001", "Epic", "-t", "epic")
	createSeedTask(t, root, "d00001", "Dependency")
	createSeedTask(t, root, "f00001", "Feature", "-t", "feature", "--parent", "e00001", "--depends-on", "d00001")

	if seed%2 == 0 {
		mustRun(t, root, nil, []string{"move", "d00001", "in_progress"})
	}
	if seed%3 == 0 {
		if seed%2 != 0 {
			mustRun(t, root, nil, []string{"move", "d00001", "in_progress"})
		}
		mustRun(t, root, nil, []string{"close", "d00001"})
	}
	if seed%5 != 0 {
		mustRun(t, root, nil, []string{"move", "f00001", "spec"})
	}
	if seed%7 == 0 {
		mustRun(t, root, nil, []string{"update", "f00001", "--field", "spec_approved=true"})
		mustRun(t, root, strings.NewReader("Design\n"), []string{"edit", "f00001", "--section", "Design", "--file", "-"})
	}
	if seed%11 == 0 {
		mustRun(t, root, nil, []string{"move", "e00001", "active"})
	}
}

func transitionTestApp(root string) *App {
	return New(Options{
		Version: "test",
		WorkDir: root,
		Env: func(string) string {
			return ""
		},
		GitUser: func(context.Context) (string, error) {
			return "", errors.New("git user unset")
		},
		Now: func() time.Time {
			return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		},
	})
}

func expectedTransitionsFromEngine(t *testing.T, state *projectState, id string) TransitionsResult {
	t.Helper()
	task, err := state.Index.Resolve(id)
	if err != nil {
		t.Fatalf("Resolve(%s): %v", id, err)
	}
	view, ok := state.Index.Resolver().Get(task.ID)
	if !ok {
		t.Fatalf("resolver missing %s", task.ID)
	}
	expected := TransitionsResult{Task: task.ID, Status: task.Status}
	for _, status := range schema.AvailableTransitions(state.Schema, state.Index.Resolver(), view) {
		expected.Transitions = append(expected.Transitions, TransitionStatus{
			To:          status.Transition.To,
			Kind:        string(status.Transition.Kind),
			Description: status.Transition.Description,
			Passing:     status.Passing,
			Violations:  convertViolations(state, status.Violations),
		})
	}
	return expected
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return data
}
