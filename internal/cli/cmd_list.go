package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var opts listOptions
		cmd := app.command("list", "List tasks", cobra.NoArgs, func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runList(opts)
		})
		cmd.Flags().VarP(&opts.Types, "type", "t", "task type")
		cmd.Flags().VarP(&opts.Statuses, "status", "s", "task status")
		cmd.Flags().VarP(&opts.Labels, "label", "l", "task label")
		cmd.Flags().Var(&opts.Parents, "parent", "parent id")
		cmd.Flags().Var(&opts.Assignees, "assignee", "assignee")
		cmd.Flags().BoolVar(&opts.Archived, "archived", false, "show archived tasks")
		cmd.Flags().IntVar(&opts.Limit, "limit", 0, "maximum number of tasks")
		return cmd
	})
}

type listOptions struct {
	Types     stringList
	Statuses  stringList
	Labels    stringList
	Parents   stringList
	Assignees stringList
	Archived  bool
	Limit     int
}

type ListResult struct {
	Tasks []TaskSummary `json:"tasks"`
}

type TaskSummary struct {
	ID          string   `json:"id"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Status      string   `json:"status"`
	Priority    string   `json:"priority,omitempty"`
	Parent      string   `json:"parent,omitempty"`
	Assignee    string   `json:"assignee,omitempty"`
	Labels      []string `json:"labels,omitempty"`
	File        string   `json:"file"`
	PendingDeps int      `json:"pending_deps,omitempty"`
}

func (r ListResult) HumanText(Colorizer) string {
	if len(r.Tasks) == 0 {
		return "no tasks"
	}
	lines := make([]string, 0, len(r.Tasks))
	for _, t := range r.Tasks {
		lines = append(lines, fmt.Sprintf("%s %-10s %-12s %s", t.ID, t.Type, t.Status, t.Title))
	}
	return strings.Join(lines, "\n")
}

func (a *App) runList(opts listOptions) (ListResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return ListResult{}, err
	}
	archived := opts.Archived
	tasks := state.Index.Query(index.Query{
		Types:     opts.Types,
		Statuses:  opts.Statuses,
		Labels:    opts.Labels,
		Parents:   opts.Parents,
		Assignees: opts.Assignees,
		Archived:  &archived,
		Limit:     opts.Limit,
	})
	result := ListResult{Tasks: make([]TaskSummary, 0, len(tasks))}
	for _, t := range tasks {
		result.Tasks = append(result.Tasks, summarizeTask(t))
	}
	return result, nil
}

func summarizeTask(t *task.Task) TaskSummary {
	return TaskSummary{
		ID:       t.ID,
		Type:     t.Type,
		Title:    t.Title,
		Status:   t.Status,
		Priority: t.Priority,
		Parent:   t.Parent,
		Assignee: t.Assignee,
		Labels:   append([]string(nil), t.Labels...),
		File:     t.File,
	}
}
