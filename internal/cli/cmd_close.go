package cli

import (
	"context"

	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var to string
		var reason string
		cmd := app.command("close <id>...", "Close tasks", cobra.MinimumNArgs(1), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			return app.withProjectMutationLock(func() (any, error) {
				return app.runClose(args, to, reason)
			})
		})
		cmd.Flags().StringVar(&to, "to", "", "explicit terminal status")
		cmd.Flags().StringVar(&reason, "reason", "", "close reason")
		return cmd
	})
}

func (a *App) runClose(ids []string, explicitTo, reason string) (any, error) {
	state, err := a.loadProject()
	if err != nil {
		return nil, err
	}
	results := MutationResults{Results: make([]MutationResult, 0, len(ids))}
	hadError := false
	for _, id := range ids {
		result := a.closeOne(state, id, explicitTo, reason)
		if !result.OK {
			hadError = true
		}
		results.Results = append(results.Results, result)
	}
	if hadError {
		if len(results.Results) == 1 && results.Results[0].Err != nil {
			return nil, results.Results[0].Err
		}
		return nil, partialError{data: results, code: syaerr.ExitTransitionRejected}
	}
	return results, nil
}

func (a *App) closeOne(state *projectState, id, explicitTo, reason string) MutationResult {
	t, err := state.Index.Resolve(id)
	if err != nil {
		payload := syaerr.Payload(err)
		return MutationResult{ID: id, OK: false, Error: &payload, Err: err}
	}
	typeDef := state.Schema.Types[t.Type]
	targets := typeDef.Terminal
	if explicitTo != "" {
		if !stringIn(typeDef.Terminal, explicitTo) {
			err := syaerr.Usage{Message: "close --to must be a terminal status"}
			payload := syaerr.Payload(err)
			return MutationResult{ID: t.ID, File: t.File, OK: false, Error: &payload, Err: err}
		}
		targets = []string{explicitTo}
	}
	for _, target := range targets {
		transition, ok, err := transitionForStatus(state.Schema, t, target)
		if err != nil {
			payload := syaerr.Payload(err)
			return MutationResult{ID: t.ID, File: t.File, OK: false, Error: &payload, Err: err}
		}
		if !ok {
			continue
		}
		if violations := checkTransition(state, t, transition); len(violations) > 0 {
			err := transitionError(state, t, transition, violations)
			return a.transitionDenied(state, t, transition.To, err)
		}
		from := t.Status
		if err := moveTask(state, state.Project.Root, t, transition, a.Actor(), a.now(), reason, true); err != nil {
			payload := syaerr.Payload(err)
			return MutationResult{ID: t.ID, File: t.File, OK: false, Error: &payload, Err: err}
		}
		return a.transitionOK(state, t, from, target, true)
	}
	err = syaerr.TransitionNotAllowed{Task: t.ID, TaskType: t.Type, From: t.Status, To: "terminal", Allowed: allowedOptions(state.Schema, state.Index.Resolver(), t)}
	return a.transitionDenied(state, t, "terminal", err)
}
