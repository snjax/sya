package cli

import (
	"context"

	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var opts updateOptions
		cmd := app.command("update <id>", "Update task metadata", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			opts.ID = args[0]
			return app.withProjectMutationLock(func() (any, error) {
				return app.runUpdate(opts)
			})
		})
		cmd.Flags().StringVar(&opts.Title, "title", "", "new title")
		cmd.Flags().StringVar(&opts.Priority, "priority", "", "new priority")
		cmd.Flags().StringVar(&opts.Assignee, "assignee", "", "new assignee")
		cmd.Flags().StringVar(&opts.Parent, "parent", "", "parent id or none")
		cmd.Flags().Var(&opts.Fields, "field", "field key=value")
		return cmd
	})
}

type updateOptions struct {
	ID       string
	Title    string
	Priority string
	Assignee string
	Parent   string
	Fields   stringList
}

func (a *App) runUpdate(opts updateOptions) (MutationResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return MutationResult{}, err
	}
	t, err := state.Index.Resolve(opts.ID)
	if err != nil {
		return MutationResult{}, err
	}
	typeDef := state.Schema.Types[t.Type]
	if opts.Title != "" {
		t.Title = opts.Title
	}
	if opts.Priority != "" {
		t.Priority = opts.Priority
	}
	if opts.Assignee != "" {
		t.Assignee = opts.Assignee
	}
	if opts.Parent != "" {
		if opts.Parent == "none" {
			t.Parent = ""
		} else {
			parent, err := validateParent(state.Index, state.Schema, t.Type, opts.Parent)
			if err != nil {
				return MutationResult{}, err
			}
			t.Parent = parent
		}
	}
	for _, raw := range opts.Fields {
		key, value, err := parseKeyValue(raw)
		if err != nil {
			return MutationResult{}, err
		}
		field, ok := declaredField(typeDef, key)
		if !ok {
			return MutationResult{}, syaerr.Usage{Message: "field is not declared for type: " + key}
		}
		parsed, err := parseFieldValue(field, value)
		if err != nil {
			return MutationResult{}, err
		}
		if t.Fields == nil {
			t.Fields = make(map[string]any)
		}
		t.Fields[key] = parsed
	}
	if err := writeTask(state.Project.Root, t); err != nil {
		return MutationResult{}, err
	}
	return MutationResult{ID: t.ID, File: t.File, Status: t.Status, OK: true}, nil
}
