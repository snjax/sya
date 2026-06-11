package cli

import (
	"context"

	"github.com/snjax/sya/internal/schema"
	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var steal bool
		cmd := app.command("claim <id>", "Claim a task", cobra.ExactArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.runClaim(args[0], steal)
		})
		cmd.Flags().BoolVar(&steal, "steal", false, "steal claim from current assignee")
		return cmd
	})
}

func (a *App) runClaim(id string, steal bool) (MutationResult, error) {
	state, err := a.loadProject()
	if err != nil {
		return MutationResult{}, err
	}
	t, err := state.Index.Resolve(id)
	if err != nil {
		return MutationResult{}, err
	}
	actor := a.Actor()
	if t.Assignee != "" && t.Assignee != actor && !steal {
		err := syaerr.AlreadyClaimed{Task: t.ID, Assignee: t.Assignee}
		return a.transitionDenied(state, t, "working", err), err
	}
	typeDef := state.Schema.Types[t.Type]
	if len(typeDef.Working) == 0 {
		return MutationResult{}, syaerr.Usage{Message: "task type has no working statuses"}
	}
	view, ok := state.Index.Resolver().Get(t.ID)
	if !ok {
		return MutationResult{}, syaerr.NotFound{ID: t.ID}
	}
	statuses := schema.AvailableTransitions(state.Schema, state.Index.Resolver(), view)
	for _, status := range statuses {
		if !stringIn(typeDef.Working, status.Transition.To) || !status.Passing {
			continue
		}
		from := t.Status
		t.Assignee = actor
		if err := moveTask(state, state.Project.Root, t, status.Transition, actor, a.now(), "", true); err != nil {
			return MutationResult{}, err
		}
		return a.transitionOK(state, t, from, status.Transition.To, true), nil
	}
	err = syaerr.TransitionBlocked{
		Task:         t.ID,
		Transition:   syaerr.TransitionRef{From: t.Status, To: "working"},
		Violations:   []syaerr.Violation{{Kind: "claim", Message: "no reachable working transition with passing guards"}},
		Alternatives: passingAlternatives(state.Schema, state.Index.Resolver(), t, ""),
	}
	return a.transitionDenied(state, t, "working", err), err
}
