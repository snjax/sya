package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/snjax/sya/internal/index"
	"github.com/snjax/sya/internal/query"
	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/snjax/sya/internal/task"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var opts queryOptions
		cmd := app.command("query <expr>", "Query tasks", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			opts.Expr = args[0]
			return app.runQuery(opts)
		})
		cmd.Long = `Query tasks with boolean expressions.

Grammar:
  expr        := or
  or          := and ("or" and)*
  and         := unary ("and" unary)*
  unary       := "not" unary | primary
  primary     := predicate | "(" expr ")"
  predicate   := key op value | key in (value,...) | ready | blocked | archived | terminal | working | parked | dead_end | rel.<name>

Keys:
  id type status priority title assignee parent label field.<name> rel.<relation> age

Operators:
  = != ~ > >= < <= in(...)

Values:
  barewords, quoted strings, durations with h/d/w for age.

Examples:
  sya query 'type=feature and not terminal and (age>7d or blocked)'
  sya query 'rel.depends_on and priority>=high'
`
		cmd.Flags().IntVar(&opts.Limit, "limit", 0, "maximum number of tasks")
		cmd.Flags().BoolVar(&opts.Archived, "archived", false, "include archived tasks")
		return cmd
	})
}

type queryOptions struct {
	Expr     string
	Limit    int
	Archived bool
}

func (a *App) runQuery(opts queryOptions) (ListResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return ListResult{}, err
	}
	predicate, expr, err := query.Compile(opts.Expr, query.Options{Now: a.now().UTC()})
	if err != nil {
		return ListResult{}, syaerr.Usage{Message: err.Error()}
	}
	archivedFilter := (*bool)(nil)
	if !opts.Archived && !expr.References("archived") {
		active := false
		archivedFilter = &active
	}
	tasks := state.Index.Query(index.Query{Archived: archivedFilter})
	result := ListResult{Tasks: make([]TaskSummary, 0)}
	for _, t := range tasks {
		if !predicate(newQueryTask(state, t)) {
			continue
		}
		result.Tasks = append(result.Tasks, summarizeTask(t))
		if opts.Limit > 0 && len(result.Tasks) >= opts.Limit {
			break
		}
	}
	return result, nil
}

type queryTask struct {
	state   *projectState
	task    *task.Task
	ready   bool
	blocked schema.BlockedStatus
}

func newQueryTask(state *projectState, t *task.Task) queryTask {
	resolver := state.Index.Resolver()
	view, ok := resolver.Get(t.ID)
	var ready bool
	var blocked schema.BlockedStatus
	if ok {
		ready = schema.Ready(state.Schema, resolver, view)
		blocked = schema.Blocked(state.Schema, resolver, view)
	}
	return queryTask{state: state, task: t, ready: ready, blocked: blocked}
}

func (q queryTask) ID() string       { return q.task.ID }
func (q queryTask) Type() string     { return q.task.Type }
func (q queryTask) Status() string   { return q.task.Status }
func (q queryTask) Priority() string { return q.task.Priority }
func (q queryTask) Title() string    { return q.task.Title }
func (q queryTask) Assignee() string { return q.task.Assignee }
func (q queryTask) Parent() string   { return q.task.Parent }
func (q queryTask) Labels() []string { return append([]string(nil), q.task.Labels...) }
func (q queryTask) Created() time.Time {
	return q.task.Created
}
func (q queryTask) Field(name string) (any, bool) {
	value, ok := q.task.Fields[name]
	return value, ok
}
func (q queryTask) Relation(name string) []string {
	return q.state.Index.Related(q.task.ID, name)
}
func (q queryTask) Archived() bool { return q.task.Archived }
func (q queryTask) Terminal() bool {
	typeDef, ok := q.state.Schema.Types[q.task.Type]
	return ok && stringIn(typeDef.Terminal, q.task.Status)
}
func (q queryTask) Working() bool {
	typeDef, ok := q.state.Schema.Types[q.task.Type]
	return ok && stringIn(typeDef.Working, q.task.Status)
}
func (q queryTask) Parked() bool {
	typeDef, ok := q.state.Schema.Types[q.task.Type]
	return ok && stringIn(typeDef.Parked, q.task.Status)
}
func (q queryTask) Ready() bool   { return q.ready }
func (q queryTask) Blocked() bool { return q.blocked.Blocked }
func (q queryTask) DeadEnd() bool { return q.blocked.DeadEnd }

func formatSummaryRows(tasks []TaskSummary, empty string) string {
	if len(tasks) == 0 {
		return empty
	}
	lines := make([]string, 0, len(tasks))
	for _, t := range tasks {
		lines = append(lines, fmt.Sprintf("%s %-10s %-12s %s", t.ID, t.Type, t.Status, t.Title))
	}
	return strings.Join(lines, "\n")
}
