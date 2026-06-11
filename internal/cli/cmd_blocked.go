package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var opts queueOptions
		cmd := app.command("blocked", "List blocked tasks", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runBlocked(opts)
		})
		addQueueFlags(cmd, &opts)
		return cmd
	})
}

type BlockedResult struct {
	Tasks []BlockedTask `json:"tasks"`
}

type BlockedTask struct {
	Task        TaskSummary         `json:"task"`
	DeadEnd     bool                `json:"dead_end,omitempty"`
	Transitions []BlockedTransition `json:"transitions,omitempty"`
}

type BlockedTransition struct {
	From       string            `json:"from"`
	To         string            `json:"to"`
	Kind       string            `json:"kind,omitempty"`
	Violations []EngineViolation `json:"violations,omitempty"`
}

type EngineViolation struct {
	Kind      string          `json:"kind"`
	Message   string          `json:"message"`
	Hint      string          `json:"hint,omitempty"`
	Relation  string          `json:"relation,omitempty"`
	Field     string          `json:"field,omitempty"`
	Section   string          `json:"section,omitempty"`
	Offending []EngineTaskRef `json:"offending,omitempty"`
}

type EngineTaskRef struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Status   string `json:"status,omitempty"`
	Archived bool   `json:"archived,omitempty"`
}

func (r BlockedResult) HumanText(Colorizer) string {
	if len(r.Tasks) == 0 {
		return "no blocked tasks"
	}
	var b strings.Builder
	for i, t := range r.Tasks {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s %-10s %-12s %s", t.Task.ID, t.Task.Type, t.Task.Status, t.Task.Title)
		if t.DeadEnd {
			b.WriteString("\n  dead-end: no candidate transitions")
			continue
		}
		for _, transition := range t.Transitions {
			fmt.Fprintf(&b, "\n  %s -> %s", transition.From, transition.To)
			for _, violation := range transition.Violations {
				fmt.Fprintf(&b, "\n    - %s: %s", violation.Kind, violation.Message)
				if len(violation.Offending) > 0 {
					var ids []string
					for _, offending := range violation.Offending {
						ids = append(ids, offending.ID)
					}
					fmt.Fprintf(&b, " (%s)", strings.Join(ids, ", "))
				}
			}
		}
	}
	return b.String()
}

func (a *App) runBlocked(opts queueOptions) (BlockedResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return BlockedResult{}, err
	}
	resolver := state.Index.Resolver()
	tasks := filteredQueueTasks(state.Index, opts)
	result := BlockedResult{Tasks: make([]BlockedTask, 0, len(tasks))}
	for _, t := range tasks {
		view, ok := resolver.Get(t.ID)
		if !ok {
			continue
		}
		blocked := schema.Blocked(state.Schema, resolver, view)
		if !blocked.Blocked {
			continue
		}
		result.Tasks = append(result.Tasks, blockedTask(t, blocked))
		if opts.Limit > 0 && len(result.Tasks) >= opts.Limit {
			break
		}
	}
	return result, nil
}

func blockedTask(t *task.Task, blocked schema.BlockedStatus) BlockedTask {
	out := BlockedTask{Task: summarizeTask(t), DeadEnd: blocked.DeadEnd}
	for _, transition := range blocked.Transitions {
		if transition.Passing {
			continue
		}
		out.Transitions = append(out.Transitions, BlockedTransition{
			From:       transition.Transition.From,
			To:         transition.Transition.To,
			Kind:       string(transition.Transition.Kind),
			Violations: convertEngineViolations(transition.Violations),
		})
	}
	return out
}

func convertEngineViolations(violations []schema.Violation) []EngineViolation {
	out := make([]EngineViolation, 0, len(violations))
	for _, violation := range violations {
		out = append(out, EngineViolation{
			Kind:      violation.Kind,
			Message:   violation.Message,
			Hint:      violation.Hint,
			Relation:  violation.Relation,
			Field:     violation.Field,
			Section:   violation.Section,
			Offending: convertEngineTaskRefs(violation.Offending),
		})
	}
	return out
}

func convertEngineTaskRefs(refs []schema.TaskRef) []EngineTaskRef {
	out := make([]EngineTaskRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, EngineTaskRef{
			ID:       ref.ID,
			Type:     ref.Type,
			Status:   ref.Status,
			Archived: ref.Archived,
		})
	}
	return out
}
