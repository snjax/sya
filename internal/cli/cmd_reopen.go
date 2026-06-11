package cli

import (
	"context"

	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var to string
		cmd := app.command("reopen <id>", "Reopen a terminal task", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runReopen(args[0], to)
		})
		cmd.Flags().StringVar(&to, "to", "", "non-terminal status")
		return cmd
	})
}

func (a *App) runReopen(id, to string) (MutationResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return MutationResult{}, err
	}
	t, err := state.Index.Resolve(id)
	if err != nil {
		return MutationResult{}, err
	}
	typeDef := state.Schema.Types[t.Type]
	if !stringIn(typeDef.Terminal, t.Status) {
		return MutationResult{}, syaerr.Usage{Message: "task is not terminal"}
	}
	if to == "" {
		if len(typeDef.Pipeline) == 0 {
			return MutationResult{}, syaerr.SchemaInvalid{Message: "type pipeline is empty"}
		}
		to = typeDef.Pipeline[0]
	}
	if stringIn(typeDef.Terminal, to) || !stringIn(typeDef.Pipeline, to) {
		return MutationResult{}, syaerr.Usage{Message: "reopen target must be non-terminal status in pipeline"}
	}
	from := t.Status
	t.Status = to
	appendTransitionLog(t, a.now(), a.Actor(), from, to, string(schema.TransitionSetback), "reopened")
	if err := writeTask(state.Project.Root, t); err != nil {
		return MutationResult{}, err
	}
	return MutationResult{ID: t.ID, File: t.File, From: from, To: to, Status: to, OK: true}, nil
}
