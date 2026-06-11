package schema

import (
	"math/rand"
	"testing"
)

func TestEnginePropertiesOverSmallGraphs(t *testing.T) {
	t.Parallel()

	for seed := int64(0); seed < 256; seed++ {
		seed := seed
		t.Run("seed", func(t *testing.T) {
			t.Parallel()
			rng := rand.New(rand.NewSource(seed))
			schema := enginePropertySchema()
			resolver := randomEngineResolver(rng)

			for _, task := range resolver.tasks {
				ready := Ready(schema, resolver, task)
				blocked := Blocked(schema, resolver, task).Blocked
				active := !stringSetContains(schema.Types[task.typ].Terminal, task.status) &&
					!stringSetContains(schema.Types[task.typ].Parked, task.status)
				if active && ready == blocked {
					t.Fatalf("seed %d task %#v ready=%v blocked=%v, want exact partition", seed, task, ready, blocked)
				}
				if !active && (ready || blocked) {
					t.Fatalf("seed %d inactive task %#v ready=%v blocked=%v, want neither", seed, task, ready, blocked)
				}

				for _, status := range AvailableTransitions(schema, resolver, task) {
					assertDeclaredViolationRelations(t, schema, status.Violations)
					assertNoArchivedOffending(t, status.Violations)
				}
				for _, status := range Blocked(schema, resolver, task).Transitions {
					assertDeclaredViolationRelations(t, schema, status.Violations)
					assertNoArchivedOffending(t, status.Violations)
				}

				archivedResolver := resolver.withArchivedTargets(task.id, "depends_on")
				violations := Evaluate(schema, archivedResolver, archivedResolver.tasks[task.id], Transition{
					From: task.status,
					To:   "done",
					Guards: []Guard{
						relationStatusGuard("depends_on", []string{"terminal"}),
					},
				})
				if hasEngineViolation(violations, string(GuardRelationStatus)) || hasEngineViolation(violations, "blocking_relation") {
					t.Fatalf("seed %d archived targets should satisfy terminal guards: %#v", seed, violations)
				}
			}
		})
	}
}

func enginePropertySchema() *Schema {
	return &Schema{
		Relations: map[string]RelationDef{
			"depends_on": {Blocking: true, From: []string{"*"}, To: []string{"*"}},
			"relates":    {From: []string{"*"}, To: []string{"*"}},
		},
		Types: map[string]TypeDef{
			"task": {
				Pipeline: []string{"todo", "doing", "parked", "done", "scrapped"},
				Terminal: []string{"done", "scrapped"},
				Working:  []string{"doing"},
				Parked:   []string{"parked"},
				Transitions: map[string]Transition{
					"todo -> doing": {From: "todo", To: "doing"},
					"doing -> done": {
						From: "doing",
						To:   "done",
						Guards: []Guard{
							relationStatusGuard("depends_on", []string{"terminal"}),
						},
					},
					"* -> scrapped": {From: "*", To: "scrapped", Kind: TransitionSetback},
				},
			},
		},
	}
}

func randomEngineResolver(rng *rand.Rand) engineResolver {
	statuses := []string{"todo", "doing", "parked", "done", "scrapped"}
	count := 2 + rng.Intn(6)
	tasks := make(map[string]engineTask, count)
	for index := range count {
		id := string(rune('a' + index))
		tasks[id] = engineTask{
			id:       id,
			typ:      "task",
			status:   statuses[rng.Intn(len(statuses))],
			archived: rng.Intn(5) == 0,
		}
	}
	ids := make([]string, 0, len(tasks))
	for id := range tasks {
		ids = append(ids, id)
	}
	for id, task := range tasks {
		relations := make(map[string][]string)
		for _, target := range ids {
			if target == id || rng.Intn(3) != 0 {
				continue
			}
			relations["depends_on"] = append(relations["depends_on"], target)
		}
		task.relations = relations
		tasks[id] = task
	}
	return engineResolver{tasks: tasks}
}

func (r engineResolver) withArchivedTargets(sourceID, relation string) engineResolver {
	tasks := make(map[string]engineTask, len(r.tasks))
	for id, task := range r.tasks {
		tasks[id] = task
	}
	source := tasks[sourceID]
	for _, targetID := range source.relations[relation] {
		target := tasks[targetID]
		target.archived = true
		target.status = "todo"
		tasks[targetID] = target
	}
	return engineResolver{tasks: tasks}
}

func assertDeclaredViolationRelations(t *testing.T, schema *Schema, violations []Violation) {
	t.Helper()
	for _, violation := range violations {
		if violation.Relation == "" {
			continue
		}
		if _, ok := schema.Relations[violation.Relation]; !ok {
			t.Fatalf("violation references undeclared relation %q: %#v", violation.Relation, violation)
		}
	}
}

func assertNoArchivedOffending(t *testing.T, violations []Violation) {
	t.Helper()
	for _, violation := range violations {
		for _, offending := range violation.Offending {
			if offending.Archived {
				t.Fatalf("archived task should count terminal and not be offending: %#v", violation)
			}
		}
	}
}
