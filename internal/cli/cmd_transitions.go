package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		return app.command("transitions <id>", "Show available transitions", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runTransitions(args[0])
		})
	})
}

type TransitionsResult struct {
	Task        string             `json:"task"`
	Status      string             `json:"status"`
	Transitions []TransitionStatus `json:"transitions"`
}

type TransitionStatus struct {
	To          string             `json:"to"`
	Kind        string             `json:"kind,omitempty"`
	Description string             `json:"description,omitempty"`
	Passing     bool               `json:"passing"`
	Violations  []syaerr.Violation `json:"violations,omitempty"`
}

func (r TransitionsResult) HumanText(Colorizer) string {
	if len(r.Transitions) == 0 {
		return "no transitions"
	}
	lines := make([]string, 0, len(r.Transitions))
	for _, transition := range r.Transitions {
		status := "blocked"
		if transition.Passing {
			status = "ok"
		}
		lines = append(lines, fmt.Sprintf("%s -> %s [%s]", r.Status, transition.To, status))
	}
	return strings.Join(lines, "\n")
}

func (a *App) runTransitions(id string) (TransitionsResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return TransitionsResult{}, err
	}
	t, err := state.Index.Resolve(id)
	if err != nil {
		return TransitionsResult{}, err
	}
	typeDef, ok := state.Schema.Types[t.Type]
	if !ok {
		return TransitionsResult{}, syaerr.SchemaInvalid{Message: "unknown task type: " + t.Type}
	}
	transitions, err := schema.ExpandTransitions(typeDef)
	if err != nil {
		return TransitionsResult{}, err
	}
	result := TransitionsResult{Task: t.ID, Status: t.Status}
	for _, transition := range transitions {
		if transition.From != t.Status {
			continue
		}
		violations := evaluateTransition(state, t, typeDef, transition)
		result.Transitions = append(result.Transitions, TransitionStatus{
			To:          transition.To,
			Kind:        string(transition.Kind),
			Description: transition.Description,
			Passing:     len(violations) == 0,
			Violations:  violations,
		})
	}
	return result, nil
}

func evaluateTransition(state *projectState, t *task.Task, typeDef schema.TypeDef, transition schema.Transition) []syaerr.Violation {
	var violations []syaerr.Violation
	for _, guard := range transition.Guards {
		violations = append(violations, evaluateGuard(state, t, guard)...)
	}
	if targetIsWorkingOrTerminal(typeDef, transition.To) {
		for relation, relationDef := range state.Schema.Relations {
			if !relationDef.Blocking || stringIn(transition.IgnoreBlocking, relation) {
				continue
			}
			offending := nonTerminalRelated(state, t.ID, relation)
			if len(offending) > 0 {
				violations = append(violations, syaerr.Violation{
					Kind:      "blocking_relation",
					Relation:  relation,
					Message:   "blocking relation targets are not terminal",
					Offending: offending,
				})
			}
		}
	}
	return violations
}

func evaluateGuard(state *projectState, t *task.Task, guard schema.Guard) []syaerr.Violation {
	switch guard.Kind {
	case schema.GuardField:
		field := stringParam(guard.Params, "field")
		expected, ok := guard.Params["equals"]
		if !ok {
			return nil
		}
		if fmt.Sprint(t.Fields[field]) != fmt.Sprint(expected) {
			return []syaerr.Violation{{Kind: string(guard.Kind), Field: field, Message: guard.Message, Hint: guard.Hint}}
		}
	case schema.GuardSectionNonempty:
		section := stringParam(guard.Params, "section")
		if sectionEmpty(t, section) {
			return []syaerr.Violation{{Kind: string(guard.Kind), Section: section, File: t.File, Message: guard.Message, Hint: guard.Hint}}
		}
	case schema.GuardRelationExists:
		relation := stringParam(guard.Params, "relation")
		if len(state.Index.Related(t.ID, relation)) == 0 {
			return []syaerr.Violation{{Kind: string(guard.Kind), Relation: relation, Message: guard.Message, Hint: guard.Hint}}
		}
	case schema.GuardRelationStatus:
		relation := stringParam(guard.Params, "relation")
		allowed := stringSliceParam(guard.Params, "in")
		offending := relatedNotInStatuses(state, state.Index.Related(t.ID, relation), allowed)
		if len(offending) > 0 {
			return []syaerr.Violation{{Kind: string(guard.Kind), Relation: relation, Message: guard.Message, Hint: guard.Hint, Offending: offending}}
		}
	case schema.GuardChildrenStatus:
		allowed := stringSliceParam(guard.Params, "in")
		offending := relatedNotInStatuses(state, state.Index.Children(t.ID), allowed)
		if len(offending) > 0 {
			return []syaerr.Violation{{Kind: string(guard.Kind), Message: guard.Message, Hint: guard.Hint, Offending: offending}}
		}
	case schema.GuardParentStatus:
		allowed := stringSliceParam(guard.Params, "in")
		parent, ok := state.Index.Parent(t.ID)
		if !ok {
			return nil
		}
		offending := relatedNotInStatuses(state, []string{parent}, allowed)
		if len(offending) > 0 {
			return []syaerr.Violation{{Kind: string(guard.Kind), Message: guard.Message, Hint: guard.Hint, Offending: offending}}
		}
	}
	return nil
}

func targetIsWorkingOrTerminal(typeDef schema.TypeDef, status string) bool {
	return stringIn(typeDef.Working, status) || stringIn(typeDef.Terminal, status)
}

func nonTerminalRelated(state *projectState, id, relation string) []syaerr.Candidate {
	ids := state.Index.Related(id, relation)
	return relatedNotInStatuses(state, ids, []string{"terminal"})
}

func relatedNotInStatuses(state *projectState, ids []string, allowed []string) []syaerr.Candidate {
	var offending []syaerr.Candidate
	for _, id := range ids {
		related, err := state.Index.Resolve(id)
		if err != nil {
			offending = append(offending, syaerr.Candidate{ID: id})
			continue
		}
		if statusAllowed(state.Schema, related, allowed) {
			continue
		}
		offending = append(offending, candidateFromTask(related))
	}
	return offending
}

func statusAllowed(sch *schema.Schema, t *task.Task, allowed []string) bool {
	for _, status := range allowed {
		if status == t.Status {
			return true
		}
		if status == "terminal" {
			typeDef := sch.Types[t.Type]
			if stringIn(typeDef.Terminal, t.Status) {
				return true
			}
		}
	}
	return false
}

func candidateFromTask(t *task.Task) syaerr.Candidate {
	return syaerr.Candidate{ID: t.ID, Title: t.Title, Type: t.Type, Status: t.Status, File: t.File}
}

func stringParam(params map[string]any, key string) string {
	value, _ := params[key].(string)
	return value
}

func stringSliceParam(params map[string]any, key string) []string {
	switch value := params[key].(type) {
	case []string:
		return value
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			out = append(out, fmt.Sprint(item))
		}
		return out
	default:
		return nil
	}
}

func sectionEmpty(t *task.Task, name string) bool {
	for _, section := range t.Body.Sections {
		if section.Name == name {
			return strings.TrimSpace(sectionText(section)) == ""
		}
	}
	return true
}
