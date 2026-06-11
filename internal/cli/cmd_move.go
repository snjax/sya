package cli

import (
	"context"

	"github.com/snjax/sya/internal/syaerr"
	"github.com/spf13/cobra"
)

func init() {
	registerCommand(func(app *App) *cobra.Command {
		var explain bool
		cmd := app.command("move <id>... <status>", "Move task status", cobra.MinimumNArgs(2), func(ctx context.Context, cmd *cobra.Command, args []string) (any, error) {
			status := args[len(args)-1]
			return app.runMove(args[:len(args)-1], status, explain)
		})
		cmd.Flags().BoolVar(&explain, "explain", false, "check transition without writing")
		return cmd
	})
}

func (a *App) runMove(ids []string, status string, explain bool) (any, error) {
	state, err := a.loadProject()
	if err != nil {
		return nil, err
	}
	results := MutationResults{Results: make([]MutationResult, 0, len(ids))}
	hadError := false
	for _, id := range ids {
		result := a.moveOne(state, id, status, "", !explain)
		if !result.OK {
			hadError = true
		}
		results.Results = append(results.Results, result)
		if result.OK && !explain {
			state, _ = a.loadProject()
		}
	}
	if hadError {
		return nil, partialError{data: results, code: syaerr.ExitTransitionRejected}
	}
	return results, nil
}

func (a *App) moveOne(state *projectState, id, status, reason string, write bool) MutationResult {
	t, err := state.Index.Resolve(id)
	if err != nil {
		payload := syaerr.Payload(err)
		return MutationResult{ID: id, OK: false, Error: &payload}
	}
	transition, ok, err := transitionForStatus(state.Schema, t, status)
	if err != nil {
		payload := syaerr.Payload(err)
		return MutationResult{ID: t.ID, File: t.File, OK: false, Error: &payload}
	}
	if !ok {
		err := syaerr.TransitionNotAllowed{
			Task:    t.ID,
			From:    t.Status,
			To:      status,
			Allowed: allowedOptions(state.Schema, state.Index.Resolver(), t),
		}
		payload := syaerr.Payload(err)
		return MutationResult{ID: t.ID, File: t.File, OK: false, Error: &payload}
	}
	if violations := checkTransition(state, t, transition); len(violations) > 0 {
		payload := syaerr.Payload(transitionError(state, t, transition, violations))
		return MutationResult{ID: t.ID, File: t.File, OK: false, Error: &payload}
	}
	from := t.Status
	if err := moveTask(state, state.Project.Root, t, transition, a.Actor(), a.now(), reason, write); err != nil {
		payload := syaerr.Payload(err)
		return MutationResult{ID: t.ID, File: t.File, OK: false, Error: &payload}
	}
	return MutationResult{ID: t.ID, File: t.File, From: from, To: transition.To, Status: transition.To, OK: true}
}
