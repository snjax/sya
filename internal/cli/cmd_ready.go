package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var opts queueOptions
		cmd := app.command("ready", "List ready tasks", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runReady(opts)
		})
		addQueueFlags(cmd, &opts)
		return cmd
	})
}

type queueOptions struct {
	Types     stringList
	Labels    stringList
	Parents   stringList
	Assignees stringList
	Limit     int
}

type ReadyResult struct {
	Tasks []TaskSummary `json:"tasks"`
}

func (r ReadyResult) HumanText(Colorizer) string {
	if len(r.Tasks) == 0 {
		return "no ready tasks"
	}
	lines := make([]string, 0, len(r.Tasks))
	for _, t := range r.Tasks {
		line := fmt.Sprintf("%s %-10s %-12s %s", t.ID, t.Type, t.Status, t.Title)
		if t.PendingDeps > 0 {
			line += fmt.Sprintf(" [deps: %d open]", t.PendingDeps)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func addQueueFlags(cmd *cobra.Command, opts *queueOptions) {
	cmd.Flags().VarP(&opts.Types, "type", "t", "task type")
	cmd.Flags().VarP(&opts.Labels, "label", "l", "task label")
	cmd.Flags().Var(&opts.Parents, "parent", "parent id")
	cmd.Flags().Var(&opts.Assignees, "assignee", "assignee")
	cmd.Flags().IntVar(&opts.Limit, "limit", 0, "maximum number of tasks")
}

func (a *App) runReady(opts queueOptions) (ReadyResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return ReadyResult{}, err
	}
	resolver := state.Index.Resolver()
	tasks := filteredQueueTasks(state.Index, opts)
	result := ReadyResult{Tasks: make([]TaskSummary, 0, len(tasks))}
	for _, t := range tasks {
		view, ok := resolver.Get(t.ID)
		if !ok || !schema.Ready(state.Schema, resolver, view) {
			continue
		}
		summary := summarizeTask(t)
		summary.PendingDeps = pendingBlockingDeps(state, t)
		result.Tasks = append(result.Tasks, summary)
		if opts.Limit > 0 && len(result.Tasks) >= opts.Limit {
			break
		}
	}
	return result, nil
}

func pendingBlockingDeps(state *projectState, t *task.Task) int {
	if state == nil || state.Schema == nil || state.Index == nil || t == nil {
		return 0
	}
	resolver := state.Index.Resolver()
	count := 0
	for relationName, relation := range state.Schema.Relations {
		if !relation.Blocking {
			continue
		}
		for _, targetID := range t.Relations[relationName] {
			target, ok := resolver.Get(targetID)
			if !ok || taskViewTerminal(state.Schema, target) {
				continue
			}
			count++
		}
	}
	return count
}

func taskViewTerminal(sch *schema.Schema, view schema.TaskView) bool {
	if sch == nil || view == nil {
		return false
	}
	typeDef, ok := sch.Types[view.Type()]
	if !ok {
		return false
	}
	return view.Archived() || stringIn(typeDef.Terminal, view.Status())
}

func filteredQueueTasks(idx *index.Index, opts queueOptions) []*task.Task {
	archived := false
	return idx.Query(index.Query{
		Types:     opts.Types,
		Labels:    opts.Labels,
		Parents:   opts.Parents,
		Assignees: opts.Assignees,
		Archived:  &archived,
	})
}
